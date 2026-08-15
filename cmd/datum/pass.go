// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/dsgnr/datum/internal/capability"
	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/observe"
	"github.com/dsgnr/datum/internal/plan"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/resolve"
	"github.com/dsgnr/datum/internal/state"
)

// hostFlags are shared by the commands that read a host.
type hostFlags struct {
	host *string
	repo *string
}

func addHostFlags(fs *flag.FlagSet) hostFlags {
	return hostFlags{
		host: fs.String("host", "", "resolve for a named host instead of the local one"),
		repo: fs.String("repo", ".", "use a local checkout"),
	}
}

// pass is what the read-only half produces. Observe, diff and plan are the same
// work stopping at different points.
type pass struct {
	manifest resolve.Manifest
	graph    *graph.Graph
	observed observe.State
	plan     plan.Plan

	// Carried so that reconcile can apply against the same providers and checkout
	// the plan was built from.
	providers provider.Set
	repoRoot  string
}

// runPass resolves, observes and plans, changing nothing.
func runPass(e *env, f hostFlags) (pass, int) {
	if *f.host == "" {
		e.errorf("--host is required, because nothing yet works out which host this machine is\n")
		return pass{}, exitError
	}

	result, revision, err := loadRepo(*f.repo)
	if err != nil {
		e.errorf("%v\n", err)
		return pass{}, exitError
	}

	manifest, err := resolve.Host(result.Set, *f.host, revision)
	if err != nil {
		e.errorf("%v\n", err)
		return pass{}, exitError
	}

	g, err := graph.Build(manifest)
	if err != nil {
		e.errorf("%v\n", err)
		return pass{}, exitError
	}

	providers, err := capability.Detect()
	if err != nil {
		e.errorf("%v\n", err)
		return pass{}, exitError
	}

	observed, err := observe.Host(context.Background(), g, providers, result.Root)
	if err != nil {
		e.errorf("%v\n", err)
		return pass{}, exitError
	}

	return pass{
		manifest:  manifest,
		graph:     g,
		observed:  observed,
		plan:      plan.Build(manifest, g, observed),
		providers: providers,
		repoRoot:  result.Root,
	}, exitOK
}

func init() {
	register(command{
		name:    "observe",
		summary: "Read the current state of every resource in a host's manifest",
		run:     runObserve,
	})
	register(command{
		name:    "diff",
		summary: "Compare desired against observed state",
		run:     runDiff,
	})
	register(command{
		name:    "plan",
		summary: "Show the ordered actions that would reconcile a host",
		run:     runPlan,
	})
}

func runObserve(e *env, args []string) int {
	fs := newFlagSet(e, "observe")
	f := addHostFlags(fs)
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}
	p, code := runPass(e, f)
	if code != exitOK {
		return code
	}

	for _, result := range p.observed.Results {
		e.printf("%s\n", result.Ref)
		switch {
		case result.Skipped:
			e.printf("  skipped  %s\n", result.Reason)
		case result.Err != nil:
			e.printf("  error    %v\n", result.Err)
		default:
			printObservation(e.out, result.Observation)
		}
		e.printf("\n")
	}
	return exitOK
}

func printObservation(out io.Writer, o provider.Observation) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  exists\t%t\n", o.Exists)
	if o.Found != "" {
		fmt.Fprintf(w, "  found\t%s\n", o.Found)
	}
	for _, name := range sortedFields(o.Fields) {
		fmt.Fprintf(w, "  %s\t%s\n", name, oneLine(o.Fields[name].Scalar))
	}
	for _, name := range o.Unobservable {
		fmt.Fprintf(w, "  %s\tnot observable by this provider\n", name)
	}
	w.Flush()
}

func sortedFields(fields map[string]document.Value) []string {
	value := document.Value{Kind: document.KindMap, Map: fields}
	return value.Keys()
}

func runDiff(e *env, args []string) int {
	fs := newFlagSet(e, "diff")
	f := addHostFlags(fs)
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}
	p, code := runPass(e, f)
	if code != exitOK {
		return code
	}

	differing := 0
	matching := 0
	for _, step := range p.plan.Steps {
		if step.Action == state.Skip || step.Err != nil {
			continue
		}
		if !step.Action.Changes() {
			matching++
			continue
		}
		differing++

		e.printf("%s\n", step.Ref)
		w := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
		switch step.Action {
		case state.Create:
			fmt.Fprintf(w, "  %s\tis not there\n", step.Target)
		case state.Remove:
			fmt.Fprintf(w, "  %s\tis there and declared absent\n", step.Target)
		}
		for _, field := range step.Fields {
			fmt.Fprintf(w, "  %s\t%s\n", field.Field, fieldChange(field))
		}
		w.Flush()
		e.printf("\n")
	}

	e.printf("%s differ, %d match\n", plural(differing, "resource"), matching)
	// A separate code makes this usable as a drift check without parsing output.
	if differing > 0 {
		return exitDiffers
	}
	return exitOK
}

func fieldChange(d plan.FieldDiff) string {
	out := oneLine(d.Observed) + " -> " + oneLine(d.Desired)
	if d.Missing {
		out = "not set, want " + oneLine(d.Desired)
	}
	// Said on the line instead of in a note, because a difference nothing will act on
	// otherwise looks like a plan that did not work.
	if d.Uncorrectable {
		return out + "  (reported, not corrected)"
	}
	return out
}

func runPlan(e *env, args []string) int {
	fs := newFlagSet(e, "plan")
	f := addHostFlags(fs)
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}
	p, code := runPass(e, f)
	if code != exitOK {
		return code
	}

	printPlan(e, p.plan, p.observed)
	if p.plan.Changes() {
		return exitDiffers
	}
	return exitOK
}

func printPlan(e *env, p plan.Plan, observed observe.State) {
	e.printf("host       %s\n", p.Host)
	e.printf("revision   %s\n", p.Revision)
	e.printf("manifest   %s\n\n", p.Manifest)

	for _, step := range p.Steps {
		w := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
		switch {
		case step.Err != nil:
			fmt.Fprintf(w, "error    %s\t%v\n", step.Ref, step.Err)
		case step.Action == state.Skip:
			fmt.Fprintf(w, "skip     %s\t%s\n", step.Ref, step.Reason)
		case !step.Action.Changes():
			fmt.Fprintf(w, "none     %s\t%s\n", step.Ref, step.Target)
		default:
			fmt.Fprintf(w, "%-8s %s\n", step.Action, step.Ref)
			fmt.Fprintf(w, "         target\t%s\n", step.Target)
			if step.Provider != "" {
				fmt.Fprintf(w, "         provider\t%s\n", step.Provider)
			}
			for _, field := range step.Fields {
				fmt.Fprintf(w, "         %s\t%s\n", field.Field, fieldChange(field))
			}
			if step.Reason != "" {
				fmt.Fprintf(w, "         reason\t%s\n", step.Reason)
			}
		}
		w.Flush()
	}

	counts := p.Counts()
	e.printf("\n%d to create, %d to update, %d to remove, %d to skip, %d unchanged\n",
		counts.Create, counts.Update, counts.Remove, counts.Skip, counts.Unchanged)
	if counts.Failed > 0 {
		e.printf("%s could not be read\n", plural(counts.Failed, "resource"))
	}

	// Said outright instead of left to be inferred from the skips.
	if unsupported := observed.Unsupported(); len(unsupported) > 0 {
		e.printf("\nhost state: %s\n", state.Degraded)
		e.printf("no provider on this host for: %s\n", join(unsupported))
	}
}

func join(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}
