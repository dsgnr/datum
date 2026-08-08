// SPDX-License-Identifier: Apache-2.0

// Package plan turns desired and observed state into an ordered set of actions.
//
// A plan is data, not an execution. It is rebuilt from a fresh observation every
// pass and never stored and replayed.
package plan

import (
	"fmt"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/observe"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/resolve"
	"github.com/dsgnr/datum/internal/state"
)

// Step is what the plan intends to do to one resource.
type Step struct {
	Ref      document.Reference
	Action   state.Action
	Provider string
	Target   string

	// Fields are the differences that caused an update.
	Fields []FieldDiff
	// Reason explains an action the field list does not account for, such as a
	// triggered reload or an unsupported type.
	Reason string
	// Err means the resource could not be read, so it cannot be planned.
	Err error

	// Triggers are the resources whose change caused this one to be updated, and
	// Trigger is the operation they ask for.
	Triggers []document.Reference
	Trigger  provider.Trigger
}

// Plan is the ordered set of actions for one pass.
type Plan struct {
	Host     string
	Revision string
	Manifest string
	Steps    []Step
}

// Counts is the summary line a plan prints.
type Counts struct {
	Create    int
	Update    int
	Remove    int
	Skip      int
	Unchanged int
	Failed    int
}

func (p Plan) Counts() Counts {
	var c Counts
	for _, step := range p.Steps {
		switch {
		case step.Err != nil:
			c.Failed++
		case step.Action == state.Create:
			c.Create++
		case step.Action == state.Update:
			c.Update++
		case step.Action == state.Remove:
			c.Remove++
		case step.Action == state.Skip:
			c.Skip++
		default:
			c.Unchanged++
		}
	}
	return c
}

// Changes reports whether the plan would alter the host.
func (p Plan) Changes() bool {
	for _, step := range p.Steps {
		if step.Action.Changes() {
			return true
		}
	}
	return false
}

// Build produces the plan for one pass.
//
// Every resource appears, including the ones with nothing to do, or the plan could
// not say whether a resource was considered.
func Build(m resolve.Manifest, g *graph.Graph, observed observe.State) Plan {
	desired := make(map[document.Reference]document.Value, len(m.Resources))
	targets := make(map[document.Reference]string, len(m.Resources))
	for ref, node := range g.Nodes {
		desired[ref] = node.Resource.Desired
		targets[ref] = node.Target
	}

	diffs := diffAll(observed, desired)

	out := Plan{Host: m.Host, Revision: m.Revision, Manifest: m.ShortDigest()}
	changed := map[document.Reference]bool{}

	// Graph order, so a resource is decided after whatever it depends on and a
	// trigger can be seen.
	for _, ref := range g.Order() {
		result, _ := observed.For(ref)
		step := Step{Ref: ref, Provider: result.Provider, Target: targets[ref]}

		switch {
		case result.Skipped:
			step.Action = state.Skip
			step.Reason = "no provider for " + ref.Type + " on this host"

		case result.Err != nil:
			step.Action = state.None
			step.Err = result.Err

		default:
			step.Action, step.Fields = decide(desired[ref], diffs[ref])
		}

		// A service whose config changed is updated even when its own fields match. An
		// update, not a new action, to keep the set small.
		if triggers := firedTriggers(g, ref, changed); len(triggers) > 0 {
			step.Triggers = triggers
			step.Trigger = triggerOp(g, ref)
			if step.Action == state.None {
				step.Action = state.Update
				step.Reason = triggerReason(g, ref, triggers)
			}
		}

		if step.Action.Changes() {
			changed[ref] = true
		}
		out.Steps = append(out.Steps, step)
	}
	return out
}

// decide picks the action from what was declared and what was found.
func decide(desired document.Value, diff Diff) (state.Action, []FieldDiff) {
	wanted := present(desired)

	switch {
	case wanted && !diff.Exists:
		return state.Create, nil
	case !wanted && diff.Exists:
		return state.Remove, nil
	case !wanted && !diff.Exists:
		// Absent and already gone is the other converged case.
		return state.None, nil
	case diff.Differs():
		return state.Update, diff.Fields
	default:
		return state.None, nil
	}
}

func present(desired document.Value) bool {
	value, ok := desired.Lookup("state")
	if !ok {
		return true
	}
	return value.Scalar != "absent"
}

// firedTriggers returns the dependencies that changed and that this one reacts to.
func firedTriggers(g *graph.Graph, ref document.Reference, changed map[document.Reference]bool) []document.Reference {
	var out []document.Reference
	for _, edge := range g.Dependencies(ref) {
		if edge.Kind.Triggers() && changed[edge.From] {
			out = append(out, edge.From)
		}
	}
	return out
}

// triggerOp is the operation a resource's trigger edges ask for. Declaring both
// restartOn and reloadOn is rejected during resolution, so the first edge decides.
func triggerOp(g *graph.Graph, ref document.Reference) provider.Trigger {
	for _, edge := range g.Dependencies(ref) {
		switch edge.Kind {
		case graph.RestartOn:
			return provider.Restart
		case graph.ReloadOn:
			return provider.Reload
		}
	}
	return provider.NoTrigger
}

// triggerReason describes an update caused by something else changing. Several
// triggers in one pass still produce one update.
func triggerReason(g *graph.Graph, ref document.Reference, triggers []document.Reference) string {
	kind := "restartOn"
	for _, edge := range g.Dependencies(ref) {
		if edge.Kind.Triggers() {
			kind = edge.Kind.String()
			break
		}
	}
	if len(triggers) == 1 {
		return triggers[0].String() + " changed, " + kind + " matched"
	}
	return fmt.Sprintf("%d resources changed, %s matched", len(triggers), kind)
}
