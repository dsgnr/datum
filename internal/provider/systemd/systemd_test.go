// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

// The property list is the contract with systemctl, so the tests pin the exact call.
func showCall(unitName string) []string {
	return []string{"systemctl", "show",
		"--property=LoadState", "--property=ActiveState", "--property=UnitFileState",
		"--", unitName}
}

func showOutput(load, active, fileState string) string {
	return "LoadState=" + load + "\nActiveState=" + active + "\nUnitFileState=" + fileState + "\n"
}

func request(name string, fields map[string]string) provider.Request {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return provider.Request{
		Ref:     document.Reference{Type: "Service", Name: name},
		Target:  name,
		Desired: desired,
	}
}

func running() map[string]string {
	return map[string]string{"state": "running", "enabled": "true"}
}

func TestObserveRunningEnabledUnit(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", running()))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("the unit should exist")
	}
	if v, _ := got.Value("state"); v.Scalar != "running" {
		t.Errorf("state = %q, want running", v.Scalar)
	}
	if v, _ := got.Value("enabled"); v.Scalar != "true" {
		t.Errorf("enabled = %q, want true", v.Scalar)
	}
}

func TestObserveStoppedDisabledUnit(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "inactive", "disabled"), showCall("nginx")...)

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", running()))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("state"); v.Scalar != "stopped" {
		t.Errorf("state = %q, want stopped", v.Scalar)
	}
	if v, _ := got.Value("enabled"); v.Scalar != "false" {
		t.Errorf("enabled = %q, want false", v.Scalar)
	}
}

// A unit that is active while its main process restarts is running, and one that has
// failed is not. Both come from the unit, not from a process.
func TestObserveDerivesRunningFromTheUnit(t *testing.T) {
	for active, want := range map[string]string{
		"active":       "running",
		"activating":   "stopped",
		"deactivating": "stopped",
		"failed":       "stopped",
		"inactive":     "stopped",
	} {
		runner := runtest.New().Output(showOutput("loaded", active, "enabled"), showCall("nginx")...)
		got, err := NewWith(runner).Observe(context.Background(), request("nginx", running()))
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := got.Value("state"); v.Scalar != want {
			t.Errorf("ActiveState %q gave state %q, want %q", active, v.Scalar, want)
		}
	}
}

func TestObserveMissingUnit(t *testing.T) {
	runner := runtest.New().Output(showOutput("not-found", "inactive", ""), showCall("nginx")...)

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", running()))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("a unit systemd does not know is not there")
	}
}

// A static unit has no install configuration, so systemctl enable is a no-op on it.
// Comparing against it would report drift that could never clear.
func TestObserveStaticUnitCannotReportEnabled(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "static"), showCall("journald")...)

	got, err := NewWith(runner).Observe(context.Background(), request("journald", running()))
	if err != nil {
		t.Fatal(err)
	}
	if got.CanObserve("enabled") {
		t.Error("enabled should be unobservable for a static unit")
	}
	if v, _ := got.Value("unitFileState"); v.Scalar != "static" {
		t.Errorf("the unit file state should be reported, got %q", v.Scalar)
	}
}

func TestObserveMaskedUnit(t *testing.T) {
	runner := runtest.New().Output(showOutput("masked", "inactive", "masked"), showCall("nginx")...)

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", running()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Found, "masked") {
		t.Errorf("Found = %q, want it to say the unit is masked", got.Found)
	}
}

func TestObserveReportsSystemctlFailure(t *testing.T) {
	runner := runtest.New().Fail(1, "Failed to connect to bus\n", showCall("nginx")...)

	if _, err := NewWith(runner).Observe(context.Background(), request("nginx", running())); err == nil {
		t.Fatal("a systemctl that could not answer should be an error, not an absent unit")
	}
}

// Datum does not write units, so a Service for a unit nothing installed is an error
// at apply time. The message names the likely cause, which is a missing requires.
func TestApplyMissingUnitIsAnError(t *testing.T) {
	runner := runtest.New().Output(showOutput("not-found", "inactive", ""), showCall("nginx")...)

	err := NewWith(runner).Apply(context.Background(), request("nginx", running()), state.Create)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("the error should point at the missing dependency, got %v", err)
	}
	if runner.Ran("systemctl", "start", "--", "nginx") {
		t.Error("nothing should have been started")
	}
}

func TestStartsAStoppedService(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "inactive", "enabled"), showCall("nginx")...)

	err := NewWith(runner).Apply(context.Background(), request("nginx", running()), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "start", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
}

func TestStopsARunningService(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	err := NewWith(runner).Apply(context.Background(),
		request("nginx", map[string]string{"state": "stopped", "enabled": "true"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "stop", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
}

// Running and enabled are independent, so each is brought into line on its own.
func TestEnablesWithoutStarting(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "disabled"), showCall("nginx")...)

	err := NewWith(runner).Apply(context.Background(), request("nginx", running()), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "enable", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
	if runner.Ran("systemctl", "start", "--", "nginx") {
		t.Error("an already running service should not have been started again")
	}
}

func TestDisablesARunningServiceThatShouldNotStartAtBoot(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	err := NewWith(runner).Apply(context.Background(),
		request("nginx", map[string]string{"state": "running", "enabled": "false"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "disable", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
	if runner.Ran("systemctl", "stop", "--", "nginx") {
		t.Error("disabling is not stopping")
	}
}

// A service that is already running and enabled still restarts when its
// configuration changes, which is the entire purpose of restartOn.
func TestRestartOnTriggerRestartsAConvergedService(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	req := request("nginx", running())
	req.Trigger = provider.Restart

	if err := NewWith(runner).Apply(context.Background(), req, state.Update); err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "restart", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
}

func TestReloadOnTriggerReloads(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	req := request("nginx", running())
	req.Trigger = provider.Reload

	if err := NewWith(runner).Apply(context.Background(), req, state.Update); err != nil {
		t.Fatal(err)
	}
	if !runner.Ran("systemctl", "reload", "--", "nginx") {
		t.Errorf("calls = %v", runner.Calls())
	}
	if runner.Ran("systemctl", "restart", "--", "nginx") {
		t.Error("a reload must not become a restart")
	}
}

// Substituting a restart for a reload turns a stated requirement into an outage, so
// a unit that cannot reload fails the action.
func TestReloadFailureIsNotRetriedAsARestart(t *testing.T) {
	runner := runtest.New().
		Output(showOutput("loaded", "active", "enabled"), showCall("cron")...).
		Fail(3, "Job type reload is not applicable for unit cron.service.\n",
			"systemctl", "reload", "--", "cron")

	req := request("cron", running())
	req.Trigger = provider.Reload

	err := NewWith(runner).Apply(context.Background(), req, state.Update)
	if err == nil {
		t.Fatal("want an error when the unit cannot reload")
	}
	if !strings.Contains(err.Error(), "not applicable") {
		t.Errorf("err = %v", err)
	}
	if runner.Ran("systemctl", "restart", "--", "cron") {
		t.Error("the reload must not have fallen back to a restart")
	}
}

// Starting a stopped unit satisfies a trigger too, and reloading something that is
// not running would fail for the wrong reason.
func TestTriggerOnAStoppedServiceStartsIt(t *testing.T) {
	for _, trigger := range []provider.Trigger{provider.Restart, provider.Reload} {
		runner := runtest.New().Output(showOutput("loaded", "inactive", "enabled"), showCall("nginx")...)

		req := request("nginx", running())
		req.Trigger = trigger

		if err := NewWith(runner).Apply(context.Background(), req, state.Update); err != nil {
			t.Fatal(err)
		}
		if !runner.Ran("systemctl", "start", "--", "nginx") {
			t.Errorf("%s on a stopped unit: calls = %v", trigger, runner.Calls())
		}
	}
}

// A stopped service with a trigger is not started, because a trigger says to pick up a
// change, not to run something that should not be running.
func TestTriggerDoesNotStartAServiceDeclaredStopped(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "inactive", "enabled"), showCall("nginx")...)

	req := request("nginx", map[string]string{"state": "stopped", "enabled": "true"})
	req.Trigger = provider.Restart

	if err := NewWith(runner).Apply(context.Background(), req, state.Update); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"start", "restart", "reload"} {
		if runner.Ran("systemctl", verb, "--", "nginx") {
			t.Errorf("a service declared stopped should not have been %sed", verb)
		}
	}
}

func TestConvergedServiceIsLeftAlone(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "enabled"), showCall("nginx")...)

	if err := NewWith(runner).Apply(context.Background(), request("nginx", running()), state.Update); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"start", "stop", "restart", "reload", "enable", "disable"} {
		if runner.Ran("systemctl", verb, "--", "nginx") {
			t.Errorf("a converged service should not have been %sed", verb)
		}
	}
}

// A static unit cannot be enabled, so nothing is attempted rather than failing every
// pass on a command systemd treats as a no-op.
func TestStaticUnitIsNotEnabled(t *testing.T) {
	runner := runtest.New().Output(showOutput("loaded", "active", "static"), showCall("journald")...)

	if err := NewWith(runner).Apply(context.Background(), request("journald", running()), state.Update); err != nil {
		t.Fatal(err)
	}
	if runner.Ran("systemctl", "enable", "--", "journald") {
		t.Error("a static unit should not have been enabled")
	}
}

// A Service does not own its unit, so there is nothing for a remove to mean.
func TestRemoveIsRefused(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(), request("nginx", running()), state.Remove)
	if err == nil {
		t.Fatal("want an error")
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

func TestNoActionRunsNothing(t *testing.T) {
	runner := runtest.New()

	for _, action := range []state.Action{state.None, state.Skip} {
		if err := NewWith(runner).Apply(context.Background(), request("nginx", running()), action); err != nil {
			t.Fatal(err)
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

func TestRejectsNamesThatAreNotUnitNames(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, name := range []string{
		"",
		"--now",
		"nginx; systemctl poweroff",
		"../../etc/systemd/system/evil.service",
		"/etc/systemd/system/evil.service",
	} {
		if _, err := p.Observe(context.Background(), request(name, nil)); err == nil {
			t.Errorf("Observe accepted %q", name)
		}
		if err := p.Apply(context.Background(), request(name, running()), state.Update); err == nil {
			t.Errorf("Apply accepted %q", name)
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("a rejected name should not reach systemctl, got %v", calls)
	}
}

func TestAcceptsRealUnitNames(t *testing.T) {
	for _, name := range []string{"nginx", "nginx.service", "getty@tty1.service", "systemd-journald.service"} {
		if err := validUnit(name); err != nil {
			t.Errorf("validUnit(%q) = %v", name, err)
		}
	}
}

func TestTypesAndName(t *testing.T) {
	p := NewWith(runtest.New())
	if p.Name() != "systemd" {
		t.Errorf("Name = %q", p.Name())
	}
	if types := p.Types(); len(types) != 1 || types[0] != "Service" {
		t.Errorf("Types = %v", types)
	}
}

func TestSatisfiesTheProviderInterface(t *testing.T) {
	var _ provider.Provider = NewWith(runtest.New())
}
