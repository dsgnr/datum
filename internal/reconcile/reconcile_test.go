// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/observe"
	"github.com/dsgnr/datum/internal/plan"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/providertest"
	"github.com/dsgnr/datum/internal/resolve"
	"github.com/dsgnr/datum/internal/state"
)

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

// run does a whole pass of resolve, observe, plan and apply.
func run(t *testing.T, resources []resolve.Resource, providers provider.Set, mode Mode) Result {
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
	p := plan.Build(m, g, observed)

	result, err := Apply(context.Background(), p, Options{
		Mode:      mode,
		Providers: providers,
		Graph:     g,
		RepoRoot:  "/repo",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return result
}

func resourceFor(t *testing.T, r Result, want string) Resource {
	t.Helper()
	for _, resource := range r.Resources {
		if resource.Ref.String() == want {
			return resource
		}
	}
	t.Fatalf("%s is not in the result", want)
	return Resource{}
}

func TestNothingToDo(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", map[string]string{"version": "1.24.0-2"})

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}, provider.NewSet(host), Enforce)

	if result.Outcome != state.OutcomeConverged {
		t.Errorf("outcome = %s, want converged", result.Outcome)
	}
	if got := resourceFor(t, result, "Package[nginx]").State; got != state.Converged {
		t.Errorf("state = %s, want converged", got)
	}
	if len(host.Applied) != 0 {
		t.Errorf("nothing should have been applied, got %v", host.Order())
	}
}

func TestCreateIsAppliedAndVerified(t *testing.T) {
	host := providertest.New("fake", "Package")

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}, provider.NewSet(host), Enforce)

	if result.Outcome != state.OutcomeChanged {
		t.Errorf("outcome = %s, want changed", result.Outcome)
	}
	entry := resourceFor(t, result, "Package[nginx]")
	if entry.Action != state.Create || entry.State != state.Converged {
		t.Errorf("got action %s state %s, want create and converged", entry.Action, entry.State)
	}
	if target, ok := host.Get("Package", "nginx"); !ok || !target.Exists {
		t.Error("the package should be on the host afterwards")
	}
}

func TestRemoveIsAppliedAndVerified(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "telnet", nil)

	result := run(t, []resolve.Resource{
		resource("Package", "telnet", map[string]string{"state": "absent"}),
	}, provider.NewSet(host), Enforce)

	if result.Outcome != state.OutcomeChanged {
		t.Errorf("outcome = %s, want changed", result.Outcome)
	}
	if got := resourceFor(t, result, "Package[telnet]").State; got != state.Converged {
		t.Errorf("state = %s, want converged", got)
	}
	if _, ok := host.Get("Package", "telnet"); ok {
		t.Error("the package should be gone afterwards")
	}
}

// Actions run in graph order, so a service is not restarted before the file it
// reads has been written.
func TestActionsRunInGraphOrder(t *testing.T) {
	host := providertest.New("fake", "File", "Service")

	conf := resource("File", "nginx-conf", map[string]string{"path": "/etc/nginx/nginx.conf", "mode": "0644"})
	service := resource("Service", "nginx", map[string]string{"state": "running"})
	service.Requires = []document.Reference{conf.Ref}

	run(t, []resolve.Resource{service, conf}, provider.NewSet(host), Enforce)

	order := host.Order()
	if len(order) != 2 || order[0] != "File[nginx-conf]" || order[1] != "Service[nginx]" {
		t.Errorf("order = %v", order)
	}
}

// A provider that reports success without changing anything is the failure to catch,
// which is why verification re-observes instead of trusting the return.
func TestApplyThatDoesNotTakeIsDrift(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", map[string]string{"version": "1.22.0-1"})
	host.Unchanging("Package", "nginx")

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}, provider.NewSet(host), Enforce)

	entry := resourceFor(t, result, "Package[nginx]")
	if entry.State != state.Drifted {
		t.Fatalf("state = %s, want drifted", entry.State)
	}
	if entry.Err == nil || !strings.Contains(entry.Err.Error(), "version") {
		t.Errorf("the error should name the field that did not take, got %v", entry.Err)
	}
	if result.Outcome != state.OutcomeFailed {
		t.Errorf("outcome = %s, want failed", result.Outcome)
	}
}

func TestApplyFailureIsRecorded(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.ApplyErr["Package nginx"] = errors.New("dpkg was interrupted")

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}, provider.NewSet(host), Enforce)

	entry := resourceFor(t, result, "Package[nginx]")
	if entry.State != state.Failed {
		t.Errorf("state = %s, want failed", entry.State)
	}
	if entry.Err == nil || !strings.Contains(entry.Err.Error(), "dpkg") {
		t.Errorf("err = %v", entry.Err)
	}
	if result.Outcome != state.OutcomeFailed || result.HostState != state.HostFailed {
		t.Errorf("got outcome %s host %s", result.Outcome, result.HostState)
	}
	if result.Failures() == nil {
		t.Error("Failures should report the apply error")
	}
}

// A dependent is never attempted against a prerequisite that is not in place.
func TestFailurePropagatesToDependents(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.ApplyErr["File /etc/nginx/nginx.conf"] = errors.New("no space left on device")

	conf := resource("File", "nginx-conf", map[string]string{"path": "/etc/nginx/nginx.conf", "mode": "0644"})
	service := resource("Service", "nginx", map[string]string{"state": "running"})
	service.Requires = []document.Reference{conf.Ref}

	result := run(t, []resolve.Resource{service, conf}, provider.NewSet(host), Enforce)

	if got := resourceFor(t, result, "Service[nginx]").State; got != state.Blocked {
		t.Errorf("state = %s, want blocked", got)
	}
	if reason := resourceFor(t, result, "Service[nginx]").Reason; !strings.Contains(reason, "File[nginx-conf]") {
		t.Errorf("the reason should name the failure, got %q", reason)
	}
	for _, call := range host.Applied {
		if call.Ref == ref("Service", "nginx") {
			t.Error("a blocked resource should not have been applied")
		}
	}
}

// Blocking is transitive, or a chain of three would attempt the last one.
func TestBlockingIsTransitive(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.ApplyErr["File /etc/app/base.conf"] = errors.New("read-only filesystem")

	base := resource("File", "base", map[string]string{"path": "/etc/app/base.conf", "mode": "0644"})
	extra := resource("File", "extra", map[string]string{"path": "/etc/app/extra.conf", "mode": "0644"})
	extra.Requires = []document.Reference{base.Ref}
	service := resource("Service", "app", map[string]string{"state": "running"})
	service.Requires = []document.Reference{extra.Ref}

	result := run(t, []resolve.Resource{service, extra, base}, provider.NewSet(host), Enforce)

	for _, name := range []string{"File[extra]", "Service[app]"} {
		if got := resourceFor(t, result, name).State; got != state.Blocked {
			t.Errorf("%s state = %s, want blocked", name, got)
		}
	}
}

// One fault should not leave the rest of a host unmanaged.
func TestUnrelatedResourcesStillReconcile(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.ApplyErr["Package nginx"] = errors.New("dpkg was interrupted")

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
		resource("Package", "curl", map[string]string{"version": "8.5.0-2"}),
	}, provider.NewSet(host), Enforce)

	if got := resourceFor(t, result, "Package[curl]").State; got != state.Converged {
		t.Errorf("curl state = %s, want converged", got)
	}
}

// A resource that could not be read cannot be reconciled, and neither can anything
// waiting on it.
func TestUnreadableResourceFailsAndBlocks(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.ObserveErr["File /etc/nginx/nginx.conf"] = errors.New("permission denied")

	conf := resource("File", "nginx-conf", map[string]string{"path": "/etc/nginx/nginx.conf", "mode": "0644"})
	service := resource("Service", "nginx", map[string]string{"state": "running"})
	service.Requires = []document.Reference{conf.Ref}

	result := run(t, []resolve.Resource{service, conf}, provider.NewSet(host), Enforce)

	if got := resourceFor(t, result, "File[nginx-conf]").State; got != state.Failed {
		t.Errorf("state = %s, want failed", got)
	}
	if got := resourceFor(t, result, "Service[nginx]").State; got != state.Blocked {
		t.Errorf("state = %s, want blocked", got)
	}
}

// No provider for a type is a coverage gap rather than a failure, and it makes the
// host degraded.
func TestUnsupportedTypeIsSkipped(t *testing.T) {
	host := providertest.New("fake", "Package")

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
		resource("Service", "nginx", map[string]string{"state": "running"}),
	}, provider.NewSet(host), Enforce)

	if got := resourceFor(t, result, "Service[nginx]").State; got != state.Skipped {
		t.Errorf("state = %s, want skipped", got)
	}
	if result.HostState != state.Degraded {
		t.Errorf("host state = %s, want degraded", result.HostState)
	}
}

// Observe mode runs the same observation and the same diff and changes nothing.
func TestObserveModeChangesNothing(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "nginx", map[string]string{"version": "1.22.0-1"})

	result := run(t, []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}, provider.NewSet(host), Observe)

	if len(host.Applied) != 0 {
		t.Errorf("observe mode should apply nothing, got %v", host.Order())
	}
	entry := resourceFor(t, result, "Package[nginx]")
	if entry.State != state.Drifted {
		t.Errorf("state = %s, want drifted", entry.State)
	}
	if entry.Action != state.Update {
		t.Errorf("action = %s, want the update it would have made", entry.Action)
	}
	if entry.Err != nil {
		t.Errorf("reporting drift is not a failure, got %v", entry.Err)
	}
	// Calling this converged would make the word useless.
	if result.Outcome != state.OutcomeDrifted {
		t.Errorf("outcome = %s, want drifted", result.Outcome)
	}
	if result.HostState != state.HostDrifted {
		t.Errorf("host state = %s, want drifted", result.HostState)
	}
}

// Drift in observe mode is expected, so it does not block anything downstream.
func TestObserveModeDoesNotBlockDependents(t *testing.T) {
	host := providertest.New("fake", "File", "Service")

	conf := resource("File", "nginx-conf", map[string]string{"path": "/etc/nginx/nginx.conf", "mode": "0644"})
	service := resource("Service", "nginx", map[string]string{"state": "running"})
	service.Requires = []document.Reference{conf.Ref}

	result := run(t, []resolve.Resource{service, conf}, provider.NewSet(host), Observe)

	if got := resourceFor(t, result, "Service[nginx]").State; got != state.Drifted {
		t.Errorf("state = %s, want drifted", got)
	}
}

// A field's value would put file content on disk, so only its name is carried.
func TestDifferingFieldsAreNamedNotValued(t *testing.T) {
	host := providertest.New("fake", "File")
	host.SetFields("File", "/etc/app/secret", map[string]string{"mode": "0644"})

	result := run(t, []resolve.Resource{
		resource("File", "secret", map[string]string{"path": "/etc/app/secret", "mode": "0600"}),
	}, provider.NewSet(host), Observe)

	entry := resourceFor(t, result, "File[secret]")
	if len(entry.Fields) != 1 || entry.Fields[0] != "mode" {
		t.Errorf("fields = %v, want just the name mode", entry.Fields)
	}
}

// A restart triggered by a file change is applied as an update to the service.
func TestTriggeredRestartIsApplied(t *testing.T) {
	host := providertest.New("fake", "File", "Service")
	host.SetFields("File", "/etc/nginx/nginx.conf", map[string]string{"mode": "0600"})
	host.SetFields("Service", "nginx", map[string]string{"state": "running"})

	conf := resource("File", "nginx-conf", map[string]string{"path": "/etc/nginx/nginx.conf", "mode": "0644"})
	service := resource("Service", "nginx", map[string]string{"state": "running"})
	service.RestartOn = []document.Reference{conf.Ref}

	result := run(t, []resolve.Resource{service, conf}, provider.NewSet(host), Enforce)

	entry := resourceFor(t, result, "Service[nginx]")
	if entry.Action != state.Update {
		t.Errorf("action = %s, want update", entry.Action)
	}
	if entry.State != state.Converged {
		t.Errorf("state = %s, want converged", entry.State)
	}
	if order := host.Order(); len(order) != 2 || order[1] != "Service[nginx]" {
		t.Errorf("order = %v, want the service after the file", order)
	}
}

func TestCancellationStopsThePass(t *testing.T) {
	host := providertest.New("fake", "Package")
	m := resolve.Manifest{Host: "web-001", Resources: []resolve.Resource{
		resource("Package", "nginx", map[string]string{"version": "1.24.0-2"}),
	}}
	g, err := graph.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewSet(host)
	observed, err := observe.Host(context.Background(), g, providers, "/repo")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Apply(ctx, plan.Build(m, g, observed), Options{
		Providers: providers, Graph: g, RepoRoot: "/repo",
	}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if len(host.Applied) != 0 {
		t.Error("a cancelled pass should apply nothing")
	}
}
