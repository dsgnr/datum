// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsgnr/datum/internal/agent"
	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/reconcile"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/schedule"
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
	repo := fs.String("repo", "", "use a local checkout instead of fetching, until fetching exists")
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

	// Nothing fetches from a remote yet, so the checkout has to be named. Said here
	// rather than assumed, because silently reconciling whatever is in the current
	// directory would be worse than refusing.
	if *repo == "" {
		e.errorf("--repo is required, because fetching from %s is not implemented yet\n", cfg.Source.URL)
		return exitError
	}

	a, err := agent.New(agent.Options{
		Config: cfg,
		Pass:   agentPass(e, cfg, *repo),
		Log:    e.err,
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

// agentPass adapts the existing pass pipeline to what the agent schedules.
//
// Failures are classified here rather than in the agent, because which failures are
// upstream depends on what the pass did and the agent only needs to know how long to
// wait afterwards.
func agentPass(e *env, cfg config.Config, repo string) agent.Pass {
	return func(ctx context.Context) (report.Report, schedule.Failure, error) {
		passMode, err := parseMode(cfg.Reconciliation.Mode)
		if err != nil {
			return report.Report{}, schedule.Local, err
		}

		startedAt := time.Now()
		host := cfg.Host
		f := hostFlags{host: &host, repo: &repo}

		p, code := runPass(ctx, e, f)
		if code != exitOK {
			// Resolution covers the repository, the manifest and validation, so a
			// failure here is upstream and the schedule backs off.
			return report.Report{}, schedule.Upstream, errPassFailed
		}

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
		if _, err := report.Write(cfg.State, r); err != nil {
			e.errorf("the pass finished but its report could not be written: %v\n", err)
		}
		return r, failureFor(result), nil
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

// errPassFailed stands in for a pass that already reported its own error. runPass
// prints what went wrong, so repeating it would say the same thing twice.
var errPassFailed = passError{}

type passError struct{}

func (passError) Error() string { return "the pass could not resolve this host" }
