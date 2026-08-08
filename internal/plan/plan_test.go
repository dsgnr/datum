// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/observe"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/providertest"
	"github.com/dsgnr/datum/internal/resolve"
	"github.com/dsgnr/datum/internal/state"
)

// resource builds a manifest entry from a flat field map.
func resource(typeName, name string, fields map[string]string) resolve.Resource {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return resolve.Resource{
		Ref:      document.Reference{Type: typeName, Name: name},
		Desired:  desired,
		Position: document.Position{File: "base/resources.yaml"},
	}
}

func ref(typeName, name string) document.Reference {
	return document.Reference{Type: typeName, Name: name}
}

// build resolves a manifest into a plan against a pretend host.
func build(t *testing.T, resources []resolve.Resource, providers provider.Set) Plan {
	t.Helper()
	m := resolve.Manifest{Host: "web-001", Revision: "8b91f20", Resources: resources}
	g, err := graph.Build(m)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	observed, err := observe.Host(context.Background(), g, providers, "/repo")
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	return Build(m, g, observed)
}

func stepFor(t *testing.T, p Plan, want string) Step {
	t.Helper()
	for _, step := range p.Steps {
		if step.Ref.String() == want {
			return step
		}
	}
	t.Fatalf("%s is not in the plan", want)
	return Step{}
}

func TestConvergedResourceHasNothingToDo(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", map[string]string{"version": "1.24.0-2"})

	p := build(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"state": "present", "version": "1.24.0-2"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Package[nginx]")
	if step.Action != state.None {
		t.Errorf("action = %s, want none", step.Action)
	}
	if p.Changes() {
		t.Error("a converged plan should not report changes")
	}
}

func TestMissingResourceIsCreated(t *testing.T) {
	host := providertest.New("fake", "Package")

	p := build(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"state": "present"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Package[nginx]")
	if step.Action != state.Create {
		t.Errorf("action = %s, want create", step.Action)
	}
	if step.Provider != "fake" {
		t.Errorf("provider = %q", step.Provider)
	}
}

func TestPresentResourceDeclaredAbsentIsRemoved(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", nil)

	p := build(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"state": "absent"}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "Package[nginx]").Action; got != state.Remove {
		t.Errorf("action = %s, want remove", got)
	}
}

func TestAbsentAndAlreadyGoneIsConverged(t *testing.T) {
	host := providertest.New("fake", "Package")

	p := build(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"state": "absent"}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "Package[nginx]").Action; got != state.None {
		t.Errorf("action = %s, want none", got)
	}
}

func TestChangedFieldIsAnUpdate(t *testing.T) {
	host := providertest.New("fake", "File")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{
		"mode": "0644", "owner": "root",
	})

	p := build(t, []resolve.Resource{
		resource("File", "nginx-config", map[string]string{
			"path": "/etc/nginx/nginx.conf", "mode": "0600", "owner": "root",
		}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "File[nginx-config]")
	if step.Action != state.Update {
		t.Fatalf("action = %s, want update", step.Action)
	}
	if len(step.Fields) != 1 {
		t.Fatalf("fields = %v, want just mode", step.Fields)
	}
	if got := step.Fields[0].String(); got != "mode 0644 -> 0600" {
		t.Errorf("diff = %q", got)
	}
}

// Only declared fields are compared. A resource that says nothing about ownership
// is not drifted by ownership, however it looks on the host.
func TestUndeclaredFieldsAreNotDrift(t *testing.T) {
	host := providertest.New("fake", "File")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{
		"mode": "0600", "owner": "nobody", "group": "nogroup",
	})

	p := build(t, []resolve.Resource{
		resource("File", "nginx-config", map[string]string{
			"path": "/etc/nginx/nginx.conf", "mode": "0600",
		}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "File[nginx-config]").Action; got != state.None {
		t.Errorf("action = %s, want none", got)
	}
}

// A field the provider cannot read is not compared, because a difference that
// cannot be measured is not drift.
func TestUnobservableFieldsAreNotCompared(t *testing.T) {
	host := providertest.New("fake", "File")
	host.Set("File", "/etc/nginx/nginx.conf", providertest.Target{
		Exists:       true,
		Fields:       map[string]string{"mode": "0600"},
		Unobservable: []string{"content"},
	})

	p := build(t, []resolve.Resource{
		resource("File", "nginx-config", map[string]string{
			"path": "/etc/nginx/nginx.conf", "mode": "0600", "content": "anything",
		}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "File[nginx-config]").Action; got != state.None {
		t.Errorf("action = %s, want none", got)
	}
}

// Fields that tell a provider how to act have nothing on the host to compare
// against, so they never produce a diff.
func TestProviderDirectivesAreNotCompared(t *testing.T) {
	host := providertest.New("fake", "File")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{"mode": "0600"})

	p := build(t, []resolve.Resource{
		resource("File", "nginx-config", map[string]string{
			"path": "/etc/nginx/nginx.conf", "mode": "0600",
			"source": "files/nginx.conf", "validate": "nginx",
		}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "File[nginx-config]").Action; got != state.None {
		t.Errorf("action = %s, want none. fields: %v", got, stepFor(t, p, "File[nginx-config]").Fields)
	}
}

func TestNoProviderMeansSkipped(t *testing.T) {
	// A host whose capability set covers File and not Service.
	host := providertest.New("fake", "File")

	p := build(t, []resolve.Resource{
		resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"}),
		resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Service[nginx]")
	if step.Action != state.Skip {
		t.Errorf("action = %s, want skip", step.Action)
	}
	if !strings.Contains(step.Reason, "no provider for Service") {
		t.Errorf("reason = %q", step.Reason)
	}
}

// A plan lists every resource, including the ones with nothing to do, because a
// plan of only changes cannot say whether a resource was considered.
func TestPlanIncludesUnchangedResources(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "curl", nil)
	host.SetFields("Package", "nginx", nil)

	p := build(t, []resolve.Resource{
		resource("Package", "curl", map[string]string{"state": "present"}),
		resource("Package", "nginx", map[string]string{"state": "present"}),
		resource("Package", "git", map[string]string{"state": "present"}),
	}, provider.NewSet(host))

	if len(p.Steps) != 3 {
		t.Fatalf("plan has %d steps, want 3", len(p.Steps))
	}
	counts := p.Counts()
	if counts.Create != 1 || counts.Unchanged != 2 {
		t.Errorf("counts = %+v", counts)
	}
}

// A service whose configuration changed is updated even though its own fields already
// match, and the action is an update, not a new kind of action.
func TestReloadOnTriggersAnUpdate(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{"mode": "0644"})
	host.SetFields("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})

	file := resource("File", "nginx-config", map[string]string{
		"path": "/etc/nginx/nginx.conf", "mode": "0600",
	})
	service := resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})
	service.ReloadOn = []document.Reference{ref("File", "nginx-config")}

	p := build(t, []resolve.Resource{file, service}, provider.NewSet(host))

	step := stepFor(t, p, "Service[nginx]")
	if step.Action != state.Update {
		t.Fatalf("action = %s, want update", step.Action)
	}
	if !strings.Contains(step.Reason, "reloadOn matched") {
		t.Errorf("reason = %q", step.Reason)
	}
	if len(step.Triggers) != 1 || step.Triggers[0] != ref("File", "nginx-config") {
		t.Errorf("triggers = %v", step.Triggers)
	}
}

func TestATriggerThatDidNotFireLeavesTheServiceAlone(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{"mode": "0600"})
	host.SetFields("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})

	file := resource("File", "nginx-config", map[string]string{
		"path": "/etc/nginx/nginx.conf", "mode": "0600",
	})
	service := resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})
	service.RestartOn = []document.Reference{ref("File", "nginx-config")}

	p := build(t, []resolve.Resource{file, service}, provider.NewSet(host))

	if got := stepFor(t, p, "Service[nginx]").Action; got != state.None {
		t.Errorf("action = %s, want none", got)
	}
}

// Several files changing in one pass reload the service once, because the plan holds
// one action per resource rather than one per trigger.
func TestSeveralTriggersProduceOneUpdate(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	for _, path := range []string{"/etc/nginx/a.conf", "/etc/nginx/b.conf"} {
		host.SetFields("File", path, map[string]string{"mode": "0644"})
	}
	host.SetFields("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})

	a := resource("File", "a", map[string]string{"path": "/etc/nginx/a.conf", "mode": "0600"})
	b := resource("File", "b", map[string]string{"path": "/etc/nginx/b.conf", "mode": "0600"})
	service := resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})
	service.ReloadOn = []document.Reference{ref("File", "a"), ref("File", "b")}

	p := build(t, []resolve.Resource{a, b, service}, provider.NewSet(host))

	updates := 0
	for _, step := range p.Steps {
		if step.Ref == ref("Service", "nginx") && step.Action == state.Update {
			updates++
		}
	}
	if updates != 1 {
		t.Errorf("the service was updated %d times, want once", updates)
	}
	step := stepFor(t, p, "Service[nginx]")
	if !strings.Contains(step.Reason, "2 resources changed") {
		t.Errorf("reason = %q", step.Reason)
	}
}

// requires orders without triggering, so a package changing does not restart a
// service that merely requires it.
func TestRequiresDoesNotTrigger(t *testing.T) {
	host := providertest.New("fake", "Package", "Service")
	host.SetFields("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})

	pkg := resource("Package", "nginx", map[string]string{"state": "present"})
	service := resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})
	service.Requires = []document.Reference{ref("Package", "nginx")}

	p := build(t, []resolve.Resource{pkg, service}, provider.NewSet(host))

	if got := stepFor(t, p, "Package[nginx]").Action; got != state.Create {
		t.Fatalf("the package should be created, got %s", got)
	}
	if got := stepFor(t, p, "Service[nginx]").Action; got != state.None {
		t.Errorf("the service should be left alone, got %s", got)
	}
}

func TestPlanFollowsGraphOrder(t *testing.T) {
	host := providertest.New("fake", "Package", "File", "Service")

	pkg := resource("Package", "nginx", map[string]string{"state": "present"})
	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	file.Requires = []document.Reference{ref("Package", "nginx")}
	service := resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"})
	service.Requires = []document.Reference{ref("Package", "nginx")}
	service.RestartOn = []document.Reference{ref("File", "nginx-config")}

	p := build(t, []resolve.Resource{service, file, pkg}, provider.NewSet(host))

	var order []string
	for _, step := range p.Steps {
		order = append(order, step.Ref.String())
	}
	want := "Package[nginx],File[nginx-config],Service[nginx]"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

func TestObservationFailureIsReportedAndNotPlanned(t *testing.T) {
	host := providertest.New("fake", "File")
	host.ObserveErr["File /etc/shadow"] = errRead

	p := build(t, []resolve.Resource{
		resource("File", "shadow", map[string]string{"path": "/etc/shadow"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "File[shadow]")
	if step.Err == nil {
		t.Fatal("the read failure should be on the step")
	}
	if step.Action != state.None {
		t.Errorf("action = %s, want none for something that could not be read", step.Action)
	}
	if p.Counts().Failed != 1 {
		t.Errorf("counts = %+v", p.Counts())
	}
}

// A path holding something of the wrong kind is reported as occupied, not as a mismatch
// on every field.
func TestOccupiedTargetIsReported(t *testing.T) {
	host := providertest.New("fake", "File")
	host.Set("File", "/etc/nginx", providertest.Target{Exists: false, Found: "a directory"})

	p := build(t, []resolve.Resource{
		resource("File", "conf", map[string]string{"path": "/etc/nginx", "mode": "0600"}),
	}, provider.NewSet(host))

	if got := stepFor(t, p, "File[conf]").Action; got != state.Create {
		t.Errorf("action = %s, want create", got)
	}
}

func TestCountsCoverEveryAction(t *testing.T) {
	host := providertest.New("fake", "Package", "File")
	host.SetFields("Package", "curl", nil)
	host.SetFields("Package", "old", nil)
	host.SetFields("File", "/etc/f", map[string]string{"mode": "0644"})

	p := build(t, []resolve.Resource{
		resource("Package", "curl", map[string]string{"state": "present"}),
		resource("Package", "new", map[string]string{"state": "present"}),
		resource("Package", "old", map[string]string{"state": "absent"}),
		resource("File", "f", map[string]string{"path": "/etc/f", "mode": "0600"}),
		resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"}),
	}, provider.NewSet(host))

	got := p.Counts()
	want := Counts{Create: 1, Update: 1, Remove: 1, Skip: 1, Unchanged: 1}
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
}

var errRead = &readError{}

type readError struct{}

func (e *readError) Error() string { return "permission denied" }

// state is two different fields wearing one name. On a Service it says running or
// stopped, which is a property of a unit that is already there. Leaving it out of the
// comparison made a service somebody had stopped by hand look converged.
func TestStoppedServiceIsDrift(t *testing.T) {
	host := providertest.New("fake", "Service")
	host.SetFields("Service", "nginx", map[string]string{
		"state":   "stopped",
		"enabled": "true",
	})

	p := build(t, []resolve.Resource{
		resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Service[nginx]")
	if step.Action != state.Update {
		t.Fatalf("action = %s, want update", step.Action)
	}
	if len(step.Fields) != 1 || step.Fields[0].Field != "state" {
		t.Errorf("fields = %v, want the state field", step.Fields)
	}
}

// On the types where state means present or absent, existence already answers the
// question and comparing the field as well would report drift twice.
func TestPresentStateIsNotComparedAsAField(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", map[string]string{"version": "1.24.0-2"})

	p := build(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"state": "present", "version": "1.24.0-2"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Package[nginx]")
	if step.Action != state.None {
		t.Errorf("action = %s, want none. fields = %v", step.Action, step.Fields)
	}
}

// A service that is running but should not start at boot is drift on one field only.
func TestServiceEnabledDriftIsReported(t *testing.T) {
	host := providertest.New("fake", "Service")
	host.SetFields("Service", "nginx", map[string]string{
		"state":   "running",
		"enabled": "false",
	})

	p := build(t, []resolve.Resource{
		resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"}),
	}, provider.NewSet(host))

	step := stepFor(t, p, "Service[nginx]")
	if len(step.Fields) != 1 || step.Fields[0].Field != "enabled" {
		t.Errorf("fields = %v, want just enabled", step.Fields)
	}
}
