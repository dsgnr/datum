//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the real dpkg-query and apt-get, so they install and remove
// packages on the machine that runs them. They are behind a build tag for that reason,
// and `make test-apt` runs them in a throwaway Debian container.
package apt

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/state"
)

// A small package with nothing awkward in its dependencies, present in Debian and
// unlikely to be installed already.
const pkg = "sl"

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return c
}

func setup(t *testing.T) *Provider {
	t.Helper()
	if !Detect() {
		t.Skip("dpkg-query and apt-get are not both present")
	}
	if out, err := exec.Command("apt-get", "update", "-qq").CombinedOutput(); err != nil {
		t.Skipf("apt-get update failed, so there is no package index: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		exec.Command("apt-get", "remove", "--yes", pkg).Run()
	})
	return New()
}

func installedVersion(t *testing.T) (string, bool) {
	t.Helper()
	out, err := exec.Command("dpkg-query", "--showformat=${db:Status-Status} ${Version}", "--show", pkg).Output()
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "installed" {
		return "", false
	}
	return fields[1], true
}

func TestIntegrationInstallObserveRemove(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	req := request(pkg, map[string]string{"state": "present"})

	// Absent to begin with, which is also the unknown-package path through
	// dpkg-query.
	exec.Command("apt-get", "remove", "--yes", pkg).Run()
	before, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if before.Exists {
		t.Fatalf("%s is installed already, so this test cannot tell what it did", pkg)
	}

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}

	after, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Exists {
		t.Fatal("the package should be installed after create")
	}
	version, ok := after.Value("version")
	if !ok || version.Scalar == "" {
		t.Errorf("the installed version should be reported, got %+v", version)
	}
	if got, installed := installedVersion(t); !installed || got != version.Scalar {
		t.Errorf("dpkg reports %q installed=%t, the provider reported %q", got, installed, version.Scalar)
	}

	// A second create over a converged package changes nothing, which is what makes
	// a pass safe to repeat.
	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}
	if got, _ := installedVersion(t); got != version.Scalar {
		t.Errorf("a repeated create changed the version to %q", got)
	}

	if err := p.Apply(c, req, state.Remove); err != nil {
		t.Fatal(err)
	}
	gone, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if gone.Exists {
		t.Error("the package should be gone after remove")
	}
}

// Pinning a version has to be reachable from either direction, so apt-get is told
// to allow a downgrade.
func TestIntegrationPinnedVersion(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	if err := p.Apply(c, request(pkg, map[string]string{"state": "present"}), state.Create); err != nil {
		t.Fatal(err)
	}
	version, ok := installedVersion(t)
	if !ok {
		t.Fatal("the package should be installed")
	}

	pinned := request(pkg, map[string]string{"state": "present", "version": version})
	if err := p.Apply(c, pinned, state.Update); err != nil {
		t.Fatal(err)
	}
	got, err := p.Observe(c, pinned)
	if err != nil {
		t.Fatal(err)
	}
	reported, _ := got.Value("version")
	if reported.Scalar != version {
		t.Errorf("version = %q, want %q", reported.Scalar, version)
	}
}

// A failure has to be reported and never swallowed, or a pass would call a host
// converged on the strength of a command that did not work.
func TestIntegrationUnreachableVersionFails(t *testing.T) {
	p := setup(t)

	err := p.Apply(ctx(t), request(pkg, map[string]string{
		"state":   "present",
		"version": "9.9.9-1",
	}), state.Create)
	if err == nil {
		t.Fatal("want an error for a version that does not exist")
	}
	if !strings.Contains(err.Error(), "9.9.9-1") {
		t.Errorf("the error should name the version asked for, got %v", err)
	}
}

func TestIntegrationUnknownPackageFails(t *testing.T) {
	p := setup(t)

	err := p.Apply(ctx(t), request("datum-no-such-package", map[string]string{
		"state": "present",
	}), state.Create)
	if err == nil {
		t.Fatal("want an error for a package that does not exist")
	}
	if !strings.Contains(err.Error(), "datum-no-such-package") {
		t.Errorf("the error should name the package, got %v", err)
	}
}

// Observing something that was never installed must not be an error, because that
// is the ordinary case on every host that does not declare the package.
func TestIntegrationObserveUnknownPackage(t *testing.T) {
	p := setup(t)

	got, err := p.Observe(ctx(t), request("datum-no-such-package", nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("a package that does not exist is not installed")
	}
}
