// SPDX-License-Identifier: Apache-2.0

// Package provider is the boundary between Datum and an operating system.
//
// A provider is the only thing that reads or changes a machine. Everything above
// it is portable, which is ADR-0002 and is also why the engine is testable without
// a host.
package provider

import (
	"context"
	"sort"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/state"
)

// Trigger is the operation a dependency's change asks for.
//
// restartOn and reloadOn differ only in which Trigger the provider receives. A provider
// whose target cannot reload fails the action and does not substitute a restart.
type Trigger int

const (
	// NoTrigger means nothing downstream asked for anything.
	NoTrigger Trigger = iota
	Restart
	Reload
)

func (t Trigger) String() string {
	switch t {
	case Restart:
		return "restart"
	case Reload:
		return "reload"
	default:
		return "none"
	}
}

// Request is one resource handed to a provider, and all it gets. No view of the
// manifest, no fleet configuration and no layer provenance, because none of that should
// change what it does to a target.
type Request struct {
	Ref document.Reference
	// Target is what this resource manages, such as a path or a package name.
	Target string
	// Desired is the merged desired state.
	Desired document.Value
	// LayerDir is where relative paths such as a File source resolve from.
	LayerDir string
	// RepoRoot is the checkout, needed to read a content source.
	RepoRoot string
	// Trigger is set when a dependency changed in this pass and this resource
	// reacts to it. Observe never sees one.
	Trigger Trigger
}

// Field reads a scalar from the desired state.
func (r Request) Field(name string) (string, bool) {
	value, ok := r.Desired.Lookup(name)
	if !ok || value.Kind != document.KindScalar {
		return "", false
	}
	return value.Scalar, true
}

// FieldOr reads a scalar, or returns fallback when the document did not set it.
func (r Request) FieldOr(name, fallback string) string {
	if value, ok := r.Field(name); ok {
		return value
	}
	return fallback
}

// Present reports whether the resource is declared to exist. No state field means
// present.
func (r Request) Present() bool {
	return r.FieldOr("state", "present") != "absent"
}

// Observation is what a provider found at a target.
type Observation struct {
	// Exists is false for a path holding something of the wrong kind, with Found
	// saying what was there.
	Exists bool
	// Found lets a plan say the target is occupied instead of reporting a mismatch on
	// every field.
	Found string
	// Fields are the values read back, keyed the same way desired state is.
	Fields map[string]document.Value
	// Unobservable fields are not compared. A difference that cannot be measured is
	// not drift.
	Unobservable []string

	// Desired is the provider's own view, for values that cannot be compared as
	// declared. A File's content is a repository path on one side and bytes on the
	// other, so the provider digests both. Empty when fields compare directly.
	Desired document.Value
}

// DesiredOr returns the provider's normalised desired state, or what was declared.
func (o Observation) DesiredOr(declared document.Value) document.Value {
	if o.Desired.Kind == document.KindMap && len(o.Desired.Map) > 0 {
		return o.Desired
	}
	return declared
}

// Value reads one observed field.
func (o Observation) Value(name string) (document.Value, bool) {
	value, ok := o.Fields[name]
	return value, ok
}

// CanObserve reports whether a field was readable.
func (o Observation) CanObserve(name string) bool {
	for _, unobservable := range o.Unobservable {
		if unobservable == name {
			return false
		}
	}
	return true
}

// Provider implements one or more resource types on one class of system.
type Provider interface {
	// Name appears in plans and reports, so somebody can tell which code acted.
	Name() string

	// Types lists the resource types this provider satisfies.
	Types() []string

	// Observe must not change anything. Planning, drift reporting and verification
	// all depend on that.
	Observe(ctx context.Context, req Request) (Observation, error)

	// Apply carries out one action. It is called in plan order, one at a time.
	Apply(ctx context.Context, req Request, action state.Action) error
}

// Set maps resource types to the providers that satisfy them here. It is what a
// distribution reduces to, so nothing downstream asks what the machine runs.
type Set struct {
	byType map[string]Provider
}

func NewSet(providers ...Provider) Set {
	s := Set{byType: map[string]Provider{}}
	for _, p := range providers {
		for _, typeName := range p.Types() {
			s.byType[typeName] = p
		}
	}
	return s
}

// For returns the provider for a type, and false when nothing here handles it, which
// makes the resource skipped rather than failed.
func (s Set) For(typeName string) (Provider, bool) {
	p, ok := s.byType[typeName]
	return p, ok
}

// Supported lists the resource types this host can reconcile, sorted.
func (s Set) Supported() []string {
	out := make([]string, 0, len(s.byType))
	for typeName := range s.byType {
		out = append(out, typeName)
	}
	sort.Strings(out)
	return out
}

// Missing lists the wanted types with no provider here. Any of them makes the host
// degraded.
func (s Set) Missing(wanted []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, typeName := range wanted {
		if _, ok := s.byType[typeName]; ok || seen[typeName] {
			continue
		}
		seen[typeName] = true
		out = append(out, typeName)
	}
	sort.Strings(out)
	return out
}
