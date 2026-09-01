// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/dsgnr/datum/internal/anchor"
	"github.com/dsgnr/datum/internal/lock"
	"github.com/dsgnr/datum/internal/reconcile"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/state"
)

// defaultStateDir matches the agent configuration reference.
const defaultStateDir = "/var/lib/datum"

func init() {
	register(command{
		name:    "reconcile",
		summary: "Bring a host to its desired state and verify the result",
		run:     runReconcile,
	})
}

func runReconcile(e *env, args []string) int {
	fs := newFlagSet(e, "reconcile")
	f := addHostFlags(fs)
	stateDir := fs.String("state", defaultStateDir, "directory for the pass lock and reports")
	mode := fs.String("mode", "enforce", "enforce applies the plan, observe only reports what differs")
	wait := fs.Bool("wait", false, "wait for another pass to finish instead of exiting")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	passMode, err := parseMode(*mode)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	// The lock is taken before anything is read, so two passes cannot observe the
	// same host while one of them is partway through changing it.
	held, err := lock.Acquire(*stateDir, *wait)
	if err != nil {
		e.errorf("%v\n", err)
		if errors.Is(err, lock.ErrHeld) {
			return exitLockHeld
		}
		return exitError
	}
	defer held.Release()

	startedAt := time.Now()
	p, code := runPass(context.Background(), e, f)
	if code != exitOK {
		return code
	}

	result, err := reconcile.Apply(context.Background(), p.plan, reconcile.Options{
		Mode:      passMode,
		Providers: p.providers,
		Graph:     p.graph,
		RepoRoot:  p.repoRoot,
		Protected: anchor.Default(),
	})
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	r := buildReport(p, result, passMode, startedAt)
	if _, err := report.Write(*stateDir, r); err != nil {
		// The pass itself may well have succeeded, so this is reported rather than turned
		// into a failure of the reconciliation.
		e.errorf("the pass finished but its report could not be written: %v\n", err)
	}

	printResult(e, r, result)
	return codeFor(result.Outcome)
}

func parseMode(name string) (reconcile.Mode, error) {
	switch name {
	case "enforce":
		return reconcile.Enforce, nil
	case "observe":
		return reconcile.Observe, nil
	default:
		return reconcile.Enforce, fmt.Errorf("unknown mode %q, want enforce or observe", name)
	}
}

// codeFor keeps drift and failure apart, so a monitoring check does not have to
// parse output to tell a host that needs correcting from one that needs looking at.
func codeFor(outcome state.Outcome) int {
	switch outcome {
	case state.OutcomeFailed:
		return exitError
	case state.OutcomeDrifted:
		return exitDiffers
	default:
		return exitOK
	}
}

func buildReport(p pass, result reconcile.Result, mode reconcile.Mode, startedAt time.Time) report.Report {
	r := report.Report{
		Host: p.manifest.Host,
		// Nothing here resolves a newer revision and falls back, so the attempted
		// and applied revisions are the same until that exists.
		RevisionAttempted: p.manifest.Revision,
		RevisionApplied:   p.manifest.Revision,
		Manifest:          p.manifest.ShortDigest(),
		Outcome:           result.Outcome.String(),
		HostState:         result.HostState.String(),
		Mode:              mode.String(),
		StartedAt:         startedAt.UTC(),
		FinishedAt:        time.Now().UTC(),
	}
	for _, resource := range result.Resources {
		entry := report.Resource{
			Ref:      resource.Ref.String(),
			Provider: resource.Provider,
			Target:   resource.Target,
			Action:   resource.Action.String(),
			State:    resource.State.String(),
			Fields:   resource.Fields,
			Reason:   resource.Reason,
		}
		if resource.Err != nil {
			entry.Error = resource.Err.Error()
		}
		r.Resources = append(r.Resources, entry)
	}
	r.Counts = report.Tally(r.Resources)
	r.Duration()
	return r
}

func printResult(e *env, r report.Report, result reconcile.Result) {
	e.printf("host       %s\n", r.Host)
	e.printf("revision   %s\n", r.RevisionApplied)
	e.printf("manifest   %s\n", r.Manifest)
	e.printf("mode       %s\n\n", r.Mode)

	w := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
	for _, resource := range result.Resources {
		// Converged resources are the majority on a healthy host, and listing them buries the
		// ones that matter.
		if resource.State == state.Converged && !resource.Action.Changes() {
			continue
		}
		fmt.Fprintf(w, "%-10s %s\t%s\n", resource.State, resource.Ref, detail(resource))
	}
	w.Flush()

	e.printf("\noutcome    %s\n", r.Outcome)
	e.printf("host state %s\n", r.HostState)
	e.printf("resources  %d total, %d converged, %d drifted, %d failed, %d blocked, %d skipped\n",
		r.Counts.Total, r.Counts.Converged, r.Counts.Drifted,
		r.Counts.Failed, r.Counts.Blocked, r.Counts.Skipped)
	e.printf("duration   %dms\n", r.DurationMS)
}

func detail(resource reconcile.Resource) string {
	switch {
	case resource.Err != nil:
		return resource.Err.Error()
	case resource.Reason != "":
		return resource.Reason
	case len(resource.Fields) > 0:
		return resource.Action.String() + " " + join(resource.Fields)
	default:
		return resource.Action.String() + " " + resource.Target
	}
}
