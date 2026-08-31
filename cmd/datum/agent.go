// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsgnr/datum/internal/agent"
	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/git"
	"github.com/dsgnr/datum/internal/metrics"
	"github.com/dsgnr/datum/internal/reconcile"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/schedule"
	"github.com/dsgnr/datum/internal/source"
)

func init() {
	register(command{
		name:    "agent",
		summary: "Run as a service, reconciling on the configured interval",
		run:     runAgent,
	})
}

func runAgent(e *env, args []string) int {
	fs := newFlagSet(e, "agent")
	configPath := fs.String("config", config.Path, "agent configuration file")
	repo := fs.String("repo", "", "reconcile a local checkout instead of fetching from source.url")
	prefix := fs.String("prefix", "", "fleet directory within the repository, for a monorepo")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	cfg, warnings, err := config.Load(*configPath)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	for _, warning := range warnings {
		// Warned, not refused, so a typo is visible without stranding a host that a newer
		// agent's key was added to.
		e.errorf("%s: %s\n", *configPath, warning)
	}

	// Built before the agent, because the pass records refusals into it and the agent
	// serves it.
	registry := metrics.NewRegistry(cfg)

	pass, err := buildPass(e, cfg, registry, *repo, *prefix)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	a, err := agent.New(agent.Options{
		Config:   cfg,
		Pass:     pass,
		Log:      e.err,
		Registry: registry,
	})
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	// SIGTERM is how a service manager asks a process to stop, and SIGINT is what a
	// terminal sends. Both stop scheduling and end an in-flight pass, which leaves a
	// partially applied pass that the next one observes and continues from.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := a.Run(ctx); err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	return exitOK
}

// buildPass chooses between fetching and reading a checkout that is already there.
//
// Naming a local checkout stays supported for trying Datum on one machine and for hosts
// without git installed.
func buildPass(e *env, cfg config.Config, registry *metrics.Registry, repo, prefix string) (agent.Pass, error) {
	if repo != "" {
		// A local checkout carries no signature to verify, so a host configured to require
		// one is refused rather than applying an unverified tree.
		if cfg.Trust.Require != config.RequireNone {
			return nil, staticError("trust.require is " + string(cfg.Trust.Require) +
				" and nothing verifies a checkout named with --repo, so either drop --repo or set trust.require: none")
		}
		e.errorf("reconciling the checkout at %s, so nothing is fetched and nothing is verified\n", repo)
		return localPass(e, cfg, repo), nil
	}
	if cfg.Source.URL == "" {
		return nil, errNoSource
	}
	if !git.Available() {
		return nil, errNoGit
	}
	return fetchingPass(e, cfg, registry, prefix), nil
}

type staticError string

func (e staticError) Error() string { return string(e) }

const (
	errNoSource staticError = "source.url is required, or name a checkout with --repo"
	errNoGit    staticError = "git is not installed, so nothing can be fetched, and --repo is the only option"

	// errPassFailed stands in for a pass that already reported its own error. runPass
	// prints what went wrong, so repeating it would say the same thing twice.
	errPassFailed     staticError = "the pass could not resolve this host"
	errFallbackFailed staticError = "the accepted revision no longer resolves, so there is nothing to fall back to"
)

// fetchingPass obtains a revision and reconciles it.
func fetchingPass(e *env, cfg config.Config, registry *metrics.Registry, prefix string) agent.Pass {
	src := source.New(cfg, prefix)

	return func(ctx context.Context) (report.Report, schedule.Failure, error) {
		selection, err := src.Select(ctx)
		if err != nil {
			countRefusal(registry, err)
			// Nothing to reconcile, which is the one case a source problem leaves a
			// host unmanaged, and only on a host that was never managed.
			return report.Report{}, schedule.Upstream, err
		}

		if selection.FellBack {
			countRefusal(registry, selection.Cause)
			e.errorf("keeping %s because the newer revision was refused: %s\n",
				shortRev(selection.Accepted), selection.Why)
		}
		registry.SetTrust(trustedSigners(cfg), !selection.FirstContact)

		p, code := runPassIn(ctx, e, cfg.Host, selection.Fleet)
		if code != exitOK {
			// Resolution covers parsing, the graph and validation, so a failure here is the
			// repository, not the host.
			if selection.FellBack {
				return report.Report{}, schedule.Upstream, errFallbackFailed
			}
			return report.Report{}, schedule.Upstream, errPassFailed
		}

		// Advanced after resolution and validation and before apply, since a revision whose
		// apply failed on one resource is still accepted. Pinning a host with one
		// persistently failing resource at an old revision would mean the fix for that
		// resource could never reach it.
		if !selection.FellBack && selection.Revision != selection.Accepted {
			if err := src.Accept(selection.Revision); err != nil {
				e.errorf("the revision resolved but could not be recorded: %v\n", err)
			}
		}
		return applyAndReport(ctx, e, cfg, p, selection)
	}
}

// localPass reconciles a local checkout, with no fetch and no verification.
func localPass(e *env, cfg config.Config, repo string) agent.Pass {
	return func(ctx context.Context) (report.Report, schedule.Failure, error) {
		p, code := runPassIn(ctx, e, cfg.Host, repo)
		if code != exitOK {
			return report.Report{}, schedule.Upstream, errPassFailed
		}
		return applyAndReport(ctx, e, cfg, p, source.Selection{})
	}
}

// countRefusal records that a control turned a revision away. A refusal nothing outside
// the host can see is indistinguishable from a control that was never switched on.
func countRefusal(registry *metrics.Registry, err error) {
	if err == nil {
		return
	}
	if reason, ok := source.ReasonOf(err); ok {
		registry.RefuseRevision(string(reason))
	}
}

// trustedSigners reports the keys the host is configured to trust, which is the
// question during a key rotation. Reporting the key that signed the last revision
// instead would drop the series on a host that had fallen back.
func trustedSigners(cfg config.Config) []string {
	if cfg.Trust.Require == config.RequireNone {
		return nil
	}
	signers, err := git.Signers(cfg.Trust.Signers)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(signers))
	for _, signer := range signers {
		out = append(out, signer.KeyID)
	}
	return out
}

// runPassIn resolves, observes and plans against one fleet directory.
func runPassIn(ctx context.Context, e *env, host, fleet string) (pass, int) {
	f := hostFlags{host: &host, repo: &fleet}
	return runPass(ctx, e, f)
}

func applyAndReport(ctx context.Context, e *env, cfg config.Config, p pass, selection source.Selection) (report.Report, schedule.Failure, error) {
	passMode, err := parseMode(cfg.Reconciliation.Mode)
	if err != nil {
		return report.Report{}, schedule.Local, err
	}

	startedAt := time.Now()
	result, err := reconcile.Apply(ctx, p.plan, reconcile.Options{
		Mode:          passMode,
		Providers:     p.providers,
		Graph:         p.graph,
		RepoRoot:      p.repoRoot,
		ActionTimeout: time.Duration(cfg.Reconciliation.ActionTimeout),
	})
	if err != nil {
		return report.Report{}, schedule.Local, err
	}

	r := buildReport(p, result, passMode, startedAt)
	describeRevisions(&r, selection)
	if _, err := report.Write(cfg.State, r); err != nil {
		e.errorf("the pass finished but its report could not be written: %v\n", err)
	}
	return r, failureFor(result), nil
}

// describeRevisions records the three revisions a fleet asks about separately.
//
// Attempted is what the host last tried, applied is what it is reconciling, and last
// known good is the newest that resolved. On a healthy host all three are equal, and
// attempted running ahead of applied is the signal that a revision failed to resolve.
func describeRevisions(r *report.Report, selection source.Selection) {
	if selection.Revision == "" {
		// A local checkout, where buildReport has already taken the revision from the
		// working tree and there is no pointer to describe.
		return
	}
	r.RevisionApplied = shortRev(selection.Revision)
	r.LastKnownGood = r.RevisionApplied
	r.RevisionAttempted = r.RevisionApplied

	if selection.FellBack {
		r.RevisionAttempted = shortRev(selection.Candidate)
		r.LastKnownGood = shortRev(selection.Accepted)
		r.Error = selection.Why
	}
}

// failureFor decides what the schedule does next. Anything that reached the host is a
// local failure at worst, because the remote is not the problem.
func failureFor(result reconcile.Result) schedule.Failure {
	if result.Failures() != nil {
		return schedule.Local
	}
	return schedule.None
}

func shortRev(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
