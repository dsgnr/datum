//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the real systemctl against a booted systemd, so they start, stop,
// enable and disable units on the machine that runs them. They are behind a build tag
// for that reason, and `make test-systemd` runs them in a throwaway container with
// systemd as PID 1.
package systemd

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// cron has no ExecReload, and nginx has one. The difference is the point of two of
// these tests.
const (
	cron  = "cron.service"
	nginx = "nginx.service"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return c
}

func setup(t *testing.T) *Provider {
	t.Helper()
	if !Detect() {
		t.Skip("systemd is not the init system here")
	}
	return New()
}

func systemctl(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("systemctl %v: %v", args, err)
	}
	return strings.TrimSpace(string(out)), code
}

// activeEnterTimestamp changes when a unit is restarted and does not when it is
// reloaded, which is how a fallback from reload to restart would be caught.
func activeEnterTimestamp(t *testing.T, unitName string) string {
	t.Helper()
	out, _ := systemctl(t, "show", "--property=ActiveEnterTimestampMonotonic", "--value", unitName)
	return out
}

func TestIntegrationObserveKnownUnit(t *testing.T) {
	p := setup(t)
	systemctl(t, "start", cron)

	got, err := p.Observe(ctx(t), request(cron, running()))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("cron should be a known unit")
	}
	if v, _ := got.Value("state"); v.Scalar != "running" {
		t.Errorf("state = %q, want running", v.Scalar)
	}
	if v, _ := got.Value("loadState"); v.Scalar != "loaded" {
		t.Errorf("loadState = %q", v.Scalar)
	}
}

func TestIntegrationObserveMissingUnit(t *testing.T) {
	p := setup(t)

	got, err := p.Observe(ctx(t), request("datum-no-such.service", running()))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("a unit that does not exist should not be reported as existing")
	}
}

func TestIntegrationStartAndStop(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	t.Cleanup(func() { systemctl(t, "stop", nginx) })

	systemctl(t, "stop", nginx)
	if err := p.Apply(c, request(nginx, running()), state.Update); err != nil {
		t.Fatal(err)
	}
	got, err := p.Observe(c, request(nginx, running()))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("state"); v.Scalar != "running" {
		t.Fatalf("state = %q after start, want running", v.Scalar)
	}

	stopped := map[string]string{"state": "stopped", "enabled": "true"}
	if err := p.Apply(c, request(nginx, stopped), state.Update); err != nil {
		t.Fatal(err)
	}
	got, err = p.Observe(c, request(nginx, stopped))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("state"); v.Scalar != "stopped" {
		t.Errorf("state = %q after stop, want stopped", v.Scalar)
	}
}

// Running and enabled are independent, so enabling must not start anything.
func TestIntegrationEnableAndDisable(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	t.Cleanup(func() { systemctl(t, "stop", nginx) })

	systemctl(t, "disable", nginx)
	systemctl(t, "stop", nginx)

	enabledStopped := map[string]string{"state": "stopped", "enabled": "true"}
	if err := p.Apply(c, request(nginx, enabledStopped), state.Update); err != nil {
		t.Fatal(err)
	}
	got, err := p.Observe(c, request(nginx, enabledStopped))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("enabled"); v.Scalar != "true" {
		t.Errorf("enabled = %q, want true", v.Scalar)
	}
	if v, _ := got.Value("state"); v.Scalar != "stopped" {
		t.Errorf("enabling started the service, state = %q", v.Scalar)
	}

	disabledStopped := map[string]string{"state": "stopped", "enabled": "false"}
	if err := p.Apply(c, request(nginx, disabledStopped), state.Update); err != nil {
		t.Fatal(err)
	}
	got, err = p.Observe(c, request(nginx, disabledStopped))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("enabled"); v.Scalar != "false" {
		t.Errorf("enabled = %q, want false", v.Scalar)
	}
}

// A service that is already running and enabled still restarts when something it
// listens to changes, which is the entire purpose of restartOn.
func TestIntegrationRestartTrigger(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	t.Cleanup(func() { systemctl(t, "stop", nginx) })

	systemctl(t, "start", nginx)
	before := activeEnterTimestamp(t, nginx)

	req := request(nginx, running())
	req.Trigger = provider.Restart
	if err := p.Apply(c, req, state.Update); err != nil {
		t.Fatal(err)
	}

	if after := activeEnterTimestamp(t, nginx); after == before {
		t.Errorf("the unit was not restarted, timestamp stayed %s", before)
	}
	got, err := p.Observe(c, request(nginx, running()))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("state"); v.Scalar != "running" {
		t.Errorf("state = %q after restart, want running", v.Scalar)
	}
}

// A reload keeps the same process, so the unit's active timestamp does not move. That
// is what distinguishes it from a restart.
func TestIntegrationReloadTrigger(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	t.Cleanup(func() { systemctl(t, "stop", nginx) })

	systemctl(t, "start", nginx)
	before := activeEnterTimestamp(t, nginx)

	req := request(nginx, running())
	req.Trigger = provider.Reload
	if err := p.Apply(c, req, state.Update); err != nil {
		t.Fatal(err)
	}

	if after := activeEnterTimestamp(t, nginx); after != before {
		t.Errorf("a reload restarted the unit, timestamp moved from %s to %s", before, after)
	}
}

// Substituting a restart for a reload turns a stated requirement into an outage, so a
// unit with no ExecReload has to fail and leave the service alone.
func TestIntegrationReloadOnAUnitThatCannotReload(t *testing.T) {
	p := setup(t)
	systemctl(t, "start", cron)
	before := activeEnterTimestamp(t, cron)

	req := request(cron, running())
	req.Trigger = provider.Reload
	err := p.Apply(ctx(t), req, state.Update)
	if err == nil {
		t.Fatal("want an error, because cron has no ExecReload")
	}
	if after := activeEnterTimestamp(t, cron); after != before {
		t.Errorf("the failed reload restarted the unit anyway, %s to %s", before, after)
	}
}

// Datum does not write units, so this is an error, not an attempt.
func TestIntegrationApplyMissingUnit(t *testing.T) {
	p := setup(t)

	err := p.Apply(ctx(t), request("datum-no-such.service", running()), state.Create)
	if err == nil {
		t.Fatal("want an error for a unit nothing installed")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("the error should point at the missing dependency, got %v", err)
	}
}

// A static unit has no install configuration, so whether it is enabled is not a
// question with an answer and must not be reported as drift.
func TestIntegrationStaticUnit(t *testing.T) {
	p := setup(t)

	got, err := p.Observe(ctx(t), request("systemd-journald.service", running()))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("unitFileState"); v.Scalar != "static" {
		t.Skipf("systemd-journald is %q here rather than static", v.Scalar)
	}
	if got.CanObserve("enabled") {
		t.Error("enabled should be unobservable for a static unit")
	}
}

// A second apply over a converged service changes nothing, which is what makes a pass
// safe to repeat.
func TestIntegrationRepeatedApplyChangesNothing(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	t.Cleanup(func() { systemctl(t, "stop", nginx) })

	if err := p.Apply(c, request(nginx, running()), state.Update); err != nil {
		t.Fatal(err)
	}
	before := activeEnterTimestamp(t, nginx)

	if err := p.Apply(c, request(nginx, running()), state.Update); err != nil {
		t.Fatal(err)
	}
	if after := activeEnterTimestamp(t, nginx); after != before {
		t.Errorf("a repeated apply restarted the unit, %s to %s", before, after)
	}
}
