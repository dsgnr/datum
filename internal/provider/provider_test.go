// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/state"
)

// stub is the smallest thing that satisfies the interface, used to build sets.
type stub struct {
	name  string
	types []string
}

func (s stub) Name() string    { return s.name }
func (s stub) Types() []string { return s.types }
func (s stub) Observe(context.Context, Request) (Observation, error) {
	return Observation{}, nil
}
func (s stub) Apply(context.Context, Request, state.Action) error { return nil }

func TestSetMapsTypesToProviders(t *testing.T) {
	set := NewSet(
		stub{name: "apt", types: []string{"Package", "Repository"}},
		stub{name: "posix-file", types: []string{"File", "Directory", "Symlink"}},
	)

	p, ok := set.For("Package")
	if !ok || p.Name() != "apt" {
		t.Errorf("Package resolved to %v, %v", p, ok)
	}
	if _, ok := set.For("Service"); ok {
		t.Error("Service has no provider in this set")
	}

	want := "Directory,File,Package,Repository,Symlink"
	if got := strings.Join(set.Supported(), ","); got != want {
		t.Errorf("Supported = %s, want %s", got, want)
	}
}

func TestSetReportsMissingTypes(t *testing.T) {
	set := NewSet(stub{name: "posix-file", types: []string{"File"}})

	missing := set.Missing([]string{"File", "Service", "Package", "Service"})
	want := "Package,Service"
	if got := strings.Join(missing, ","); got != want {
		t.Errorf("Missing = %s, want %s", got, want)
	}
}

func TestRequestFieldHelpers(t *testing.T) {
	req := Request{Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
		"state": document.Scalar("present"),
		"mode":  document.Scalar("0640"),
	}}}

	if got, ok := req.Field("mode"); !ok || got != "0640" {
		t.Errorf("Field(mode) = %q, %v", got, ok)
	}
	if _, ok := req.Field("owner"); ok {
		t.Error("owner is not set")
	}
	if got := req.FieldOr("owner", "root"); got != "root" {
		t.Errorf("FieldOr = %q", got)
	}
	if !req.Present() {
		t.Error("state is present")
	}
}

// A resource with no state field is present, because the common case is wanting
// something there.
func TestPresentDefaultsToTrue(t *testing.T) {
	empty := Request{Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}}
	if !empty.Present() {
		t.Error("a resource with no state should be present")
	}

	absent := Request{Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
		"state": document.Scalar("absent"),
	}}}
	if absent.Present() {
		t.Error("state: absent should not be present")
	}
}

func TestObservationUnobservableFields(t *testing.T) {
	o := Observation{Exists: true, Unobservable: []string{"content"}}
	if o.CanObserve("content") {
		t.Error("content was reported as unobservable")
	}
	if !o.CanObserve("mode") {
		t.Error("mode should be observable")
	}
}
