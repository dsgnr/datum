//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the real rpm and dnf, so they install and remove packages on the
// machine that runs them. They are behind a build tag for that reason, and `make
// test-dnf` runs them in a throwaway Fedora container.
//
// rpm reports an absent package on stdout, which a mock written from apt's behaviour
// would not reproduce.
package dnf

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

// A small package with nothing awkward in its dependencies.
const pkg = "sl"

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return c
}

func setup(t *testing.T) *Provider {
	t.Helper()
	if _, err := exec.LookPath("dnf"); err != nil {
		t.Skip("dnf is not installed here")
	}
	t.Cleanup(func() {
		exec.Command("dnf", "remove", "--assumeyes", pkg).Run()
	})
	return New()
}

func rpmVersion(t *testing.T) (string, bool) {
	t.Helper()
	out, err := exec.Command("rpm", "--query", "--queryformat=%{VERSION}-%{RELEASE}", pkg).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func TestIntegrationInstallObserveRemove(t *testing.T) {
	p := setup(t)
	c := ctx(t)
	req := request(pkg, map[string]string{"state": "present"})

	exec.Command("dnf", "remove", "--assumeyes", pkg).Run()

	// This is the case a mock written from apt's behaviour gets wrong, because rpm
	// reports it on stdout with a non-zero exit.
	before, err := p.Observe(c, req)
	if err != nil {
		t.Fatalf("observing an absent package should not be an error: %v", err)
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
	version, _ := after.Value("version")
	got, installed := rpmVersion(t)
	if !installed || got != version.Scalar {
		t.Errorf("rpm reports %q installed=%t, the provider reported %q", got, installed, version.Scalar)
	}

	// A second create over a converged package changes nothing.
	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}
	if again, _ := rpmVersion(t); again != version.Scalar {
		t.Errorf("a repeated create moved the version to %q", again)
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

// A package already at the pinned version needs no transaction, which is the path that
// depends on rpm's own version comparison working.
func TestIntegrationPinnedVersion(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	if err := p.Apply(c, request(pkg, map[string]string{"state": "present"}), state.Create); err != nil {
		t.Fatal(err)
	}
	version, ok := rpmVersion(t)
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

// rpm's ordering is the only thing that gets cases like a release candidate right, so
// the comparison is checked against rpm itself rather than assumed.
func TestIntegrationVersionComparison(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.0-1", "2.0-1", -1},
		{"2.0-1", "1.0-1", 1},
		{"1.0-1", "1.0-1", 0},
		// A tilde sorts before the release it is a candidate for, which a string
		// compare gets backwards.
		{"1.0~rc1-1", "1.0-1", -1},
		{"5.02-22.fc41", "5.02-23.fc41", -1},
	} {
		got, err := p.compare(c, tc.a, tc.b)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestIntegrationUnreachableVersionFails(t *testing.T) {
	p := setup(t)

	err := p.Apply(ctx(t), request(pkg, map[string]string{
		"state":   "present",
		"version": "9.9.9-1",
	}), state.Create)
	if err == nil {
		t.Fatal("want an error for a version that does not exist")
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
}

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

// dnf decides whether a repo file is valid, which is the part a mock cannot check. A
// local directory is served so the test needs no network, and `dnf repolist` is asked
// about this source specifically.
func TestARepoFileThisProviderWritesIsOneDnfCanRead(t *testing.T) {
	p := setup(t)
	c := ctx(t)

	served := t.TempDir()
	req := provider.Request{
		Ref:    document.Reference{Type: "Repository", Name: "datum-local"},
		Target: "datum-local",
		Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
			"id":       document.Scalar("datum-local"),
			"url":      document.Scalar("file://" + served),
			"unsigned": document.Scalar("true"),
			"priority": document.Scalar("20"),
		}},
	}
	t.Cleanup(func() {
		_ = p.Apply(context.Background(), req, state.Remove)
	})

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// repolist parses every repo file, so a malformed one shows up here. The source
	// is disabled for the query because an empty directory has no metadata to fetch.
	out, err := exec.CommandContext(c, "dnf", "repolist", "--all").CombinedOutput()
	if err != nil {
		t.Fatalf("dnf repolist failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "datum-local") {
		t.Fatalf("dnf does not list the source:\n%s", out)
	}

	observation, err := p.Observe(c, req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !observation.Exists {
		t.Fatal("the source dnf just listed does not exist")
	}
	if got, _ := observation.Value("priority"); got.Scalar != "20" {
		t.Errorf("observed priority = %q", got.Scalar)
	}

	if err := p.Apply(c, req, state.Remove); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
	after, err := exec.CommandContext(c, "dnf", "repolist", "--all").CombinedOutput()
	if err != nil {
		t.Fatalf("dnf repolist after removal: %v\n%s", err, after)
	}
	if strings.Contains(string(after), "datum-local") {
		t.Errorf("dnf still lists a removed source:\n%s", after)
	}
}

// The key has to land where gpgkey points, or dnf refuses everything from the source
// the first time it tries to verify a package.
func TestASignedSourceLandsItsKeyWhereGpgkeyPoints(t *testing.T) {
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
			"url":        document.Scalar("https://rpm.example.com/fedora"),
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

	text, err := os.ReadFile("/etc/yum.repos.d/datum-signed.repo")
	if err != nil {
		t.Fatalf("reading repo file: %v", err)
	}
	const wantPath = "/etc/pki/rpm-gpg/datum-signed.asc"
	if !strings.Contains(string(text), "gpgkey=file://"+wantPath) {
		t.Fatalf("repo file does not point at the key:\n%s", text)
	}
	onDisk, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("key is not where gpgkey points: %v", err)
	}
	if string(onDisk) != key {
		t.Errorf("key content = %q", onDisk)
	}
}
