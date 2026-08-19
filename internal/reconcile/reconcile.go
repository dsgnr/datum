// SPDX-License-Identifier: Apache-2.0

// Package reconcile applies a plan and checks that it took.
//
// It is the only place that calls a provider's Apply, and it does so one resource
// at a time in graph order. Nothing here knows what an operating system is.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/plan"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// Mode decides whether a pass is allowed to change the host.
type Mode int

const (
	// Enforce applies the plan.
	Enforce Mode = iota
	// Observe reports what differs and changes nothing, so a difference ends the pass as
	// drift instead of an action.
	Observe
)

func (m Mode) String() string {
	if m == Observe {
		return "observe"
	}
	return "enforce"
}

// Resource is where one resource ended up.
type Resource struct {
	Ref      document.Reference
	Provider string
	Target   string
	// Action is what was intended. A resource can be Failed with an action of
	// update, which is different from having had nothing to do.
	Action state.Action
	State  state.Resource

	// Fields are the names of the fields that differed, without their values.
	Fields []string
	Reason string
	Err    error
}

// Result is one pass.
type Result struct {
	Outcome   state.Outcome
	HostState state.Host
	Resources []Resource
}

// Options are what a pass needs beyond the plan itself.
type Options struct {
	Mode      Mode
	Providers provider.Set
	Graph     *graph.Graph
	RepoRoot  string

	// ActionTimeout bounds one provider action. A package manager waiting on a lock
	// held by an unattended upgrade is the case it exists for, and without it one
	// stuck action consumes the whole pass. Zero leaves an action unbounded, which is
	// what a one-off command wants.
	ActionTimeout time.Duration
}

// actionContext bounds a single action, so a provider that hangs fails that resource,
// not the pass.
func (o Options) actionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if o.ActionTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, o.ActionTimeout)
}

// Apply works through the plan in order.
//
// A failure does not stop the pass. Resources that do not depend on the failure are
// still reconciled, because leaving the rest of a host unmanaged because one package
// would not install makes one fault into many.
func Apply(ctx context.Context, p plan.Plan, opts Options) (Result, error) {
	blocked := map[document.Reference]string{}
	out := Result{Resources: make([]Resource, 0, len(p.Steps))}
	applied := false

	for _, step := range p.Steps {
		if err := ctx.Err(); err != nil {
			return out, err
		}

		r := Resource{
			Ref:      step.Ref,
			Provider: step.Provider,
			Target:   step.Target,
			Action:   step.Action,
			Fields:   fieldNames(step.Fields),
			Reason:   step.Reason,
		}

		because, isBlocked := blocked[step.Ref]

		switch {
		case step.Err != nil:
			// A resource that could not be read cannot be reconciled, and its
			// dependents cannot rely on it either.
			r.State = state.Failed
			r.Err = step.Err
			block(opts.Graph, blocked, step.Ref, "could not be read")

		case step.Action == state.Skip:
			r.State = state.Skipped

		case isBlocked:
			r.State = state.Blocked
			r.Reason = because

		case !step.Action.Changes():
			r.State = state.Converged

		case opts.Mode == Observe:
			r.State = state.Drifted

		default:
			applied = true
			var reason string
			r.State, reason, r.Err = enforce(ctx, step, opts)
			if reason != "" {
				r.Reason = reason
			}
			// Drift nothing can correct does not block what depends on this
			// resource, because the resource is otherwise in the declared state.
			if r.State != state.Converged && r.Err != nil {
				block(opts.Graph, blocked, step.Ref, dependencyReason(step.Ref, r))
			}
		}

		out.Resources = append(out.Resources, r)
	}

	out.HostState = state.HostFrom(states(out.Resources))
	out.Outcome = outcome(out.Resources, applied)
	return out, nil
}

// enforce applies one step and then reads the target again.
//
// Re-observe instead of trusting what the provider returned. An apply that reported
// success and did not take is the failure to catch, and a provider cannot be the judge
// of its own work.
func enforce(ctx context.Context, step plan.Step, opts Options) (state.Resource, string, error) {
	p, ok := opts.Providers.For(step.Ref.Type)
	if !ok {
		// The plan marks unsupported types as skipped, so reaching here means the
		// provider set changed underneath the pass.
		return state.Failed, "", fmt.Errorf("no provider for %s", step.Ref.Type)
	}
	node, ok := opts.Graph.Nodes[step.Ref]
	if !ok {
		return state.Failed, "", fmt.Errorf("%s is not in the graph", step.Ref)
	}
	req := provider.Request{
		Ref:      step.Ref,
		Target:   node.Target,
		Desired:  node.Resource.Desired,
		LayerDir: node.Resource.LayerDir,
		RepoRoot: opts.RepoRoot,
		Trigger:  step.Trigger,
	}

	applyCtx, cancelApply := opts.actionContext(ctx)
	err := p.Apply(applyCtx, req, step.Action)
	cancelApply()
	if err != nil {
		// Said plainly, because "signal: killed" on its own reads like a crash rather than a
		// bound the agent imposed.
		if applyCtx.Err() != nil && ctx.Err() == nil {
			return state.Failed, "", fmt.Errorf("%s did not finish within the action timeout of %s",
				step.Action, opts.ActionTimeout)
		}
		return state.Failed, "", err
	}

	// Verification observes without the trigger, because a restart is something the pass
	// asked for, not a property of the target to read back.
	verifyReq := req
	verifyReq.Trigger = provider.NoTrigger
	verifyCtx, cancelVerify := opts.actionContext(ctx)
	observation, err := p.Observe(verifyCtx, verifyReq)
	cancelVerify()
	if err != nil {
		return state.Failed, "", fmt.Errorf("verifying %s: %w", step.Ref, err)
	}
	return verify(step, verifyReq, observation)
}

// verify decides whether the target now holds what was declared.
//
// A difference the provider declares uncorrectable is not a failed apply. It is drift
// that gets reported every pass and never acted on, which is the documented behaviour
// for a uid that does not match.
func verify(step plan.Step, req provider.Request, observation provider.Observation) (state.Resource, string, error) {
	if req.Present() != observation.Exists {
		return state.Drifted, "", verificationError(step, "the target is still "+existence(observation.Exists))
	}
	if !observation.Exists {
		// Declared absent and gone is as verified as it gets.
		return state.Converged, "", nil
	}

	diff := plan.Compare(step.Ref, observation.DesiredOr(req.Desired), observation)
	if failed := diff.Correctable(); len(failed) > 0 {
		return state.Drifted, "", verificationError(step, "these fields did not take: "+list(fieldNames(failed)))
	}
	if left := diff.Uncorrectable(); len(left) > 0 {
		return state.Drifted, list(fieldNames(left)) + " differs and is not corrected", nil
	}
	return state.Converged, "", nil
}

func verificationError(step plan.Step, detail string) error {
	return fmt.Errorf("%s reported success on %s but %s", step.Provider, step.Ref, detail)
}

func existence(exists bool) string {
	if exists {
		return "there"
	}
	return "absent"
}

// block marks everything downstream of a failure, transitively, so a dependent is
// never attempted against a prerequisite that is not in place.
func block(g *graph.Graph, blocked map[document.Reference]string, ref document.Reference, reason string) {
	if g == nil {
		return
	}
	queue := []document.Reference{ref}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range g.Dependents(current) {
			if _, already := blocked[edge.To]; already {
				continue
			}
			blocked[edge.To] = reason
			queue = append(queue, edge.To)
		}
	}
}

func dependencyReason(ref document.Reference, r Resource) string {
	if r.State == state.Drifted {
		return ref.String() + " did not verify"
	}
	return ref.String() + " failed"
}

// outcome distinguishes a pass that had nothing to do from one that did something.
//
// Drift is reported instead of failed in two cases, an observe-mode pass that was asked
// not to act and a difference the provider declares uncorrectable, which no number of
// passes will change. Drift left after an apply that claimed success is a failure, and
// that is the one carrying an error.
func outcome(resources []Resource, applied bool) state.Outcome {
	drifted := false
	for _, r := range resources {
		switch {
		case r.State == state.Failed, r.State == state.Blocked:
			return state.OutcomeFailed
		case r.State == state.Drifted && r.Err != nil:
			return state.OutcomeFailed
		case r.State == state.Drifted:
			drifted = true
		}
	}
	switch {
	case drifted:
		return state.OutcomeDrifted
	case applied:
		return state.OutcomeChanged
	default:
		return state.OutcomeConverged
	}
}

func states(resources []Resource) []state.Resource {
	out := make([]state.Resource, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.State)
	}
	return out
}

func fieldNames(fields []plan.FieldDiff) []string {
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.Field)
	}
	return out
}

func list(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}

// Failures collects the errors from a result, for a caller that wants one error.
func (r Result) Failures() error {
	var errs []error
	for _, resource := range r.Resources {
		if resource.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", resource.Ref, resource.Err))
		}
	}
	return errors.Join(errs...)
}
