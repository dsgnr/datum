//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the real dpkg-query and apt-get, so they install and remove
// packages on the machine that runs them. They are behind a build tag for that reason,
// and `make test-apt` runs them in a throwaway Debian container.
package apt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
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

// The value of testing a source against real apt is that apt decides whether the stanza
// is valid. A mock accepts anything written to it, and a deb822 field name that is
// subtly wrong is exactly the mistake that survives a unit test.
//
// A local flat repository instead of a third-party one, so the test needs no network
// and no key that would have to be committed.
func TestASourceThisProviderWritesIsOneAptCanRead(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	// Not t.TempDir, because apt drops privilege to the _apt user to fetch and a
	// per-test temporary directory is mode 0700. The fetch has to actually succeed
	// for this to test more than parsing.
	served := "/tmp/datum-apt-source"
	if err := os.MkdirAll(served, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(served) })
	// A flat repository is a directory holding Packages. An empty index is valid and
	// describes a source with no packages, which is all this needs.
	if err := os.WriteFile(filepath.Join(served, "Packages"), nil, 0o644); err != nil {
		t.Fatalf("write Packages: %v", err)
	}

	req := provider.Request{
		Ref:    document.Reference{Type: "Repository", Name: "datum-local"},
		Target: "datum-local",
		Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
			"id":       document.Scalar("datum-local"),
			"url":      document.Scalar("file:" + served + "/"),
			"unsigned": document.Scalar("true"),
		}},
	}
	t.Cleanup(func() {
		_ = p.Apply(context.Background(), req, state.Remove)
	})

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Restricted to this source, so an unrelated archive being unreachable in the
	// container cannot fail the assertion.
	out, err := exec.CommandContext(c, "apt-get", "update", "-qq",
		"-o", "Dir::Etc::sourcelist=/dev/null",
		"-o", "Dir::Etc::sourceparts=/etc/apt/sources.list.d",
		"-o", "APT::Get::List-Cleanup=0",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("apt-get update rejected the source: %v\n%s", err, out)
	}
	if strings.Contains(strings.ToLower(string(out)), "malformed") {
		t.Fatalf("apt reported a malformed source:\n%s", out)
	}

	observation, err := p.Observe(c, req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !observation.Exists {
		t.Fatal("the source apt just read does not exist")
	}

	if err := p.Apply(c, req, state.Remove); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
	if after, err := p.Observe(c, req); err != nil || after.Exists {
		t.Fatalf("source survived removal (exists=%v, err=%v)", after.Exists, err)
	}
}

// A real keyring has to end up somewhere apt will read it, with the stanza pointing
// at the path it was actually written to.
func TestASignedSourceLandsItsKeyringWhereTheStanzaPoints(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "files"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	key := "-----BEGIN PGP PUBLIC KEY BLOCK-----\nnot a real key\n"
	if err := os.WriteFile(filepath.Join(repo, "files", "k.asc"), []byte(key), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	req := provider.Request{
		Ref:    document.Reference{Type: "Repository", Name: "datum-signed"},
		Target: "datum-signed",
		Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
			"id":         document.Scalar("datum-signed"),
			"url":        document.Scalar("https://packages.example.com/debian"),
			"suite":      document.Scalar("stable"),
			"signingKey": document.Scalar("files/k.asc"),
		}},
		RepoRoot: repo,
	}
	t.Cleanup(func() {
		_ = p.Apply(context.Background(), req, state.Remove)
	})

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	stanza, err := os.ReadFile("/etc/apt/sources.list.d/datum-signed.sources")
	if err != nil {
		t.Fatalf("reading stanza: %v", err)
	}
	const wantPath = "/etc/apt/keyrings/datum-signed.asc"
	if !strings.Contains(string(stanza), "Signed-By: "+wantPath) {
		t.Fatalf("stanza does not point at the keyring:\n%s", stanza)
	}
	onDisk, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("keyring is not where the stanza points: %v", err)
	}
	if string(onDisk) != key {
		t.Errorf("keyring content = %q", onDisk)
	}
}
