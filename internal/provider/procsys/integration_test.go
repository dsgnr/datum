//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests write to the real /proc/sys and /etc/sysctl.d, so they change kernel
// parameters on the machine that runs them. They are behind a build tag for that
// reason, and `make test-sysctl` runs them in a throwaway privileged container.
//
// A temporary directory is a poor stand-in for procfs, whose files have fixed sizes,
// cannot be created or removed, and reject values the kernel will not take. Those are
// the behaviours worth checking against the real thing.
package procsys

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/state"
)

// A parameter that is safe to change inside a container's own network namespace.
const param = "net.ipv4.ip_forward"

func setup(t *testing.T) *Provider {
	t.Helper()
	if !Detect() {
		t.Skip("/proc/sys is not writable here")
	}
	p := New()
	t.Cleanup(func() {
		os.Remove(p.confPath(param))
	})
	return p
}

func TestIntegrationSetsTheRunningKernelAndPersists(t *testing.T) {
	p := setup(t)
	c := context.Background()

	before, readable, err := p.runningValue(param)
	if err != nil {
		t.Fatal(err)
	}
	if !readable {
		t.Skipf("%s is not present in this kernel", param)
	}
	t.Cleanup(func() { p.write(param, before) })

	// Pick the value the kernel is not currently holding.
	want := "1"
	if before == "1" {
		want = "0"
	}

	if err := p.Apply(c, request(param, present(want)), state.Create); err != nil {
		t.Fatal(err)
	}

	got, _, err := p.runningValue(param)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("running value = %q, want %q", got, want)
	}

	body, err := os.ReadFile(p.confPath(param))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), param+" = "+want) {
		t.Errorf("conf file = %q", body)
	}

	observation, err := p.Observe(c, request(param, present(want)))
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Exists {
		t.Error("the parameter should be managed now")
	}
	if v, _ := observation.Value("persisted"); v.Scalar != "true" {
		t.Errorf("persisted = %q, want true", v.Scalar)
	}
	if v, _ := observation.Value("value"); v.Scalar != want {
		t.Errorf("value = %q, want %q", v.Scalar, want)
	}
}

// procfs files have a fixed size and cannot be replaced by a rename, so writing has to
// truncate in place. A second pass over a converged parameter is where a partial write
// would show up.
func TestIntegrationRepeatedApplyIsClean(t *testing.T) {
	p := setup(t)
	c := context.Background()

	before, readable, _ := p.runningValue(param)
	if !readable {
		t.Skipf("%s is not present in this kernel", param)
	}
	t.Cleanup(func() { p.write(param, before) })

	for i := 0; i < 3; i++ {
		if err := p.Apply(c, request(param, present("1")), state.Create); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}
	got, _, err := p.runningValue(param)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("running value = %q after three passes, want 1", got)
	}
}

// The kernel rejects a value it will not accept at write time, which is the only place
// a bad value can surface.
func TestIntegrationKernelRejectsABadValue(t *testing.T) {
	p := setup(t)

	before, readable, _ := p.runningValue(param)
	if !readable {
		t.Skipf("%s is not present in this kernel", param)
	}
	t.Cleanup(func() { p.write(param, before) })

	err := p.Apply(context.Background(), request(param, present("not-a-number")), state.Create)
	if err == nil {
		t.Fatal("want an error for a value the kernel cannot take")
	}
	if got, _, _ := p.runningValue(param); got != before {
		t.Errorf("the value changed to %q despite the failure", got)
	}
}

// A parameter no kernel has must be reported rather than created, because /proc/sys does
// not let a file be added.
func TestIntegrationUnknownParameter(t *testing.T) {
	p := setup(t)

	got, err := p.Observe(context.Background(), request("net.ipv4.datum_no_such_thing", present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if got.CanObserve("value") {
		t.Error("a parameter that does not exist has no observable value")
	}

	err = p.Apply(context.Background(), request("net.ipv4.datum_no_such_thing", present("1")), state.Create)
	if err == nil {
		t.Fatal("want an error")
	}
}

// Removing takes the file away and leaves the kernel holding whatever it holds.
func TestIntegrationRemoveLeavesTheRunningValue(t *testing.T) {
	p := setup(t)
	c := context.Background()

	before, readable, _ := p.runningValue(param)
	if !readable {
		t.Skipf("%s is not present in this kernel", param)
	}
	t.Cleanup(func() { p.write(param, before) })

	if err := p.Apply(c, request(param, present("1")), state.Create); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(c, request(param, map[string]string{"state": "absent"}), state.Remove); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(p.confPath(param)); !os.IsNotExist(err) {
		t.Error("the file should be gone")
	}
	if got, _, _ := p.runningValue(param); got != "1" {
		t.Errorf("running value = %q, want it left at 1", got)
	}
}

// The provider writes into the real directory, which has to exist and be usable.
func TestIntegrationConfDirectoryIsTheRealOne(t *testing.T) {
	p := setup(t)
	if p.confDir != "/etc/sysctl.d" {
		t.Errorf("confDir = %q", p.confDir)
	}
	if filepath.Dir(p.confPath(param)) != "/etc/sysctl.d" {
		t.Errorf("confPath = %q", p.confPath(param))
	}
}
