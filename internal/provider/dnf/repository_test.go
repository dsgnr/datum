// SPDX-License-Identifier: Apache-2.0

package dnf_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/dnf"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

type repoFixture struct {
	provider *dnf.Provider
	root     string
	repo     string
}

func newRepoFixture(t *testing.T) repoFixture {
	t.Helper()
	root := t.TempDir()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "layers", "web", "files"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "layers", "web", "files", "node.asc"),
		[]byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return repoFixture{provider: dnf.NewIn(runtest.New(), root), root: root, repo: repo}
}

func (f repoFixture) request(desired map[string]document.Value) provider.Request {
	return provider.Request{
		Ref:      document.Reference{Type: "Repository", Name: "nodesource"},
		Target:   "nodesource",
		Desired:  document.Value{Kind: document.KindMap, Map: desired},
		LayerDir: "layers/web",
		RepoRoot: f.repo,
	}
}

func (f repoFixture) repoFile(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, "etc/yum.repos.d/nodesource.repo"))
	if err != nil {
		t.Fatalf("reading repo file: %v", err)
	}
	return string(data)
}

func signed() map[string]document.Value {
	return map[string]document.Value{
		"state":      document.Scalar("present"),
		"id":         document.Scalar("nodesource"),
		"url":        document.Scalar("https://rpm.nodesource.com/pub_20.x"),
		"signingKey": document.Scalar("files/node.asc"),
	}
}

func TestApplyWritesARepoFileAndItsKey(t *testing.T) {
	f := newRepoFixture(t)
	if err := f.provider.Apply(context.Background(), f.request(signed()), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	text := f.repoFile(t)
	for _, want := range []string{
		"[nodesource]",
		"name=nodesource",
		"baseurl=https://rpm.nodesource.com/pub_20.x",
		"enabled=1",
		"gpgcheck=1",
		"gpgkey=file://" + filepath.Join(f.root, "etc/pki/rpm-gpg/nodesource.asc"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("repo file missing %q, got:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(f.root, "etc/pki/rpm-gpg/nodesource.asc")); err != nil {
		t.Errorf("key was not written: %v", err)
	}
}

func TestUnsignedSourceTurnsOffGpgCheck(t *testing.T) {
	f := newRepoFixture(t)
	desired := map[string]document.Value{
		"id":       document.Scalar("internal"),
		"url":      document.Scalar("http://packages.internal/fedora"),
		"unsigned": document.Scalar("true"),
	}
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	text := f.repoFile(t)
	if !strings.Contains(text, "gpgcheck=0") {
		t.Errorf("unsigned source checks signatures:\n%s", text)
	}
	if strings.Contains(text, "gpgkey") {
		t.Errorf("unsigned source names a key:\n%s", text)
	}
}

// dnf has a native priority field, so unlike apt it does not have to refuse one.
func TestPriorityIsWrittenThrough(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["priority"] = document.Scalar("10")
	req := f.request(desired)

	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if text := f.repoFile(t); !strings.Contains(text, "priority=10") {
		t.Errorf("priority missing:\n%s", text)
	}

	observation, err := f.provider.Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if got, _ := observation.Value("priority"); got.Scalar != "10" {
		t.Errorf("observed priority = %q", got.Scalar)
	}
}

// suite and components describe an apt source. Silently dropping them would apply a
// source that is not the one the manifest asked for.
func TestAptOnlyFieldsAreRefused(t *testing.T) {
	f := newRepoFixture(t)

	withSuite := signed()
	withSuite["suite"] = document.Scalar("nodistro")
	withComponents := signed()
	withComponents["components"] = document.Value{
		Kind: document.KindList,
		List: []document.Value{document.Scalar("main")},
	}

	for name, desired := range map[string]map[string]document.Value{
		"suite":      withSuite,
		"components": withComponents,
	} {
		req := f.request(desired)
		if err := f.provider.Apply(context.Background(), req, state.Create); err == nil {
			t.Errorf("%s was accepted on apply", name)
		}
		if _, err := f.provider.Observe(context.Background(), req); err == nil {
			t.Errorf("%s was accepted on observe", name)
		}
	}
}

// An empty list is not somebody asking for components, so it should not be an error.
func TestAnEmptyComponentsListIsNotAnError(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["components"] = document.Value{Kind: document.KindList}
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func TestObserveReportsAnAbsentSource(t *testing.T) {
	f := newRepoFixture(t)
	observation, err := f.provider.Observe(context.Background(), f.request(signed()))
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if observation.Exists {
		t.Fatal("a source that was never written exists")
	}
}

func TestObserveReadsBackWhatWasApplied(t *testing.T) {
	f := newRepoFixture(t)
	req := f.request(signed())
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	observation, err := f.provider.Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !observation.Exists {
		t.Fatal("applied source does not exist")
	}
	if got, _ := observation.Value("url"); got.Scalar != "https://rpm.nodesource.com/pub_20.x" {
		t.Errorf("url = %q", got.Scalar)
	}
	desired, ok := observation.Desired.Lookup("signingKey")
	if !ok || !strings.HasPrefix(desired.Scalar, "sha256:") {
		t.Fatalf("desired signingKey = %#v", desired)
	}
	if observed, _ := observation.Value("signingKey"); observed.Scalar != desired.Scalar {
		t.Errorf("signingKey observed %q, desired %q", observed.Scalar, desired.Scalar)
	}
}

func TestARotatedKeyShowsAsDrift(t *testing.T) {
	f := newRepoFixture(t)
	req := f.request(signed())
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.repo, "layers", "web", "files", "node.asc"),
		[]byte("a different key\n"), 0o644); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	observation, err := f.provider.Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	desired, _ := observation.Desired.Lookup("signingKey")
	observed, _ := observation.Value("signingKey")
	if desired.Scalar == observed.Scalar {
		t.Error("a rotated key did not show as drift")
	}
}

func TestADisabledSourceIsWrittenDisabled(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["enabled"] = document.Scalar("false")
	req := f.request(desired)
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if text := f.repoFile(t); !strings.Contains(text, "enabled=0") {
		t.Errorf("source is not disabled:\n%s", text)
	}
	observation, err := f.provider.Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if got, _ := observation.Value("enabled"); got.Scalar != "false" {
		t.Errorf("observed enabled = %q", got.Scalar)
	}
}

func TestRemoveTakesOutTheRepoFileAndTheKey(t *testing.T) {
	f := newRepoFixture(t)
	req := f.request(signed())
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := f.provider.Apply(context.Background(), req, state.Remove); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
	for _, path := range []string{
		"etc/yum.repos.d/nodesource.repo",
		"etc/pki/rpm-gpg/nodesource.asc",
	} {
		if _, err := os.Stat(filepath.Join(f.root, path)); !os.IsNotExist(err) {
			t.Errorf("%s survived removal", path)
		}
	}
}

// The id is both a filename and an ini section header.
func TestARepositoryIdIsRefusedWhenItCouldEscapeThePath(t *testing.T) {
	f := newRepoFixture(t)
	for _, id := range []string{"../evil", "a b", "a/b", "a]b"} {
		req := f.request(signed())
		req.Target = id
		if err := f.provider.Apply(context.Background(), req, state.Create); err == nil {
			t.Errorf("id %q was accepted", id)
		}
	}
}

func TestASigningKeyOutsideTheRepositoryIsRefused(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["signingKey"] = document.Scalar("../../../../etc/shadow")
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err == nil {
		t.Error("a key outside the repository was accepted")
	}
}
