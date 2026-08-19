// SPDX-License-Identifier: Apache-2.0

package apt_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/apt"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

// repoFixture is a provider writing into a temporary root, with a fleet checkout
// holding one signing key.
type repoFixture struct {
	provider *apt.Provider
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
	return repoFixture{
		provider: apt.NewIn(runtest.New(), root),
		root:     root,
		repo:     repo,
	}
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

func (f repoFixture) sources(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, "etc/apt/sources.list.d/nodesource.sources"))
	if err != nil {
		t.Fatalf("reading sources: %v", err)
	}
	return string(data)
}

func signed() map[string]document.Value {
	return map[string]document.Value{
		"state":      document.Scalar("present"),
		"id":         document.Scalar("nodesource"),
		"url":        document.Scalar("https://deb.nodesource.com/node_20.x"),
		"suite":      document.Scalar("nodistro"),
		"signingKey": document.Scalar("files/node.asc"),
		"components": {Kind: document.KindList, List: []document.Value{document.Scalar("main")}},
	}
}

func TestApplyWritesADeb822SourceAndItsKeyring(t *testing.T) {
	f := newRepoFixture(t)
	if err := f.provider.Apply(context.Background(), f.request(signed()), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	stanza := f.sources(t)
	for _, want := range []string{
		"Types: deb",
		"URIs: https://deb.nodesource.com/node_20.x",
		"Suites: nodistro",
		"Components: main",
		"Signed-By: " + filepath.Join(f.root, "etc/apt/keyrings/nodesource.asc"),
		"Enabled: yes",
	} {
		if !strings.Contains(stanza, want) {
			t.Errorf("stanza missing %q, got:\n%s", want, stanza)
		}
	}

	// The key has to land on the host, or Signed-By points at nothing.
	key, err := os.ReadFile(filepath.Join(f.root, "etc/apt/keyrings/nodesource.asc"))
	if err != nil {
		t.Fatalf("reading keyring: %v", err)
	}
	if !strings.HasPrefix(string(key), "-----BEGIN PGP") {
		t.Errorf("keyring holds %q", key)
	}
}

// An unsigned source is the one case where apt is told to trust without a key, and
// it only reaches a provider when the manifest said so explicitly.
func TestUnsignedSourceIsTrustedWithoutAKeyring(t *testing.T) {
	f := newRepoFixture(t)
	desired := map[string]document.Value{
		"id":       document.Scalar("internal"),
		"url":      document.Scalar("http://packages.internal/debian"),
		"suite":    document.Scalar("stable"),
		"unsigned": document.Scalar("true"),
	}
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	stanza := f.sources(t)
	if !strings.Contains(stanza, "Trusted: yes") {
		t.Errorf("unsigned source is not trusted:\n%s", stanza)
	}
	if strings.Contains(stanza, "Signed-By") {
		t.Errorf("unsigned source names a key:\n%s", stanza)
	}
	if _, err := os.Stat(filepath.Join(f.root, "etc/apt/keyrings/internal.asc")); !os.IsNotExist(err) {
		t.Errorf("a keyring was written for an unsigned source")
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
	if got, _ := observation.Value("url"); got.Scalar != "https://deb.nodesource.com/node_20.x" {
		t.Errorf("url = %q", got.Scalar)
	}
	if got, _ := observation.Value("enabled"); got.Scalar != "true" {
		t.Errorf("enabled = %q", got.Scalar)
	}

	// Both sides are digests, so the comparison the engine makes is like for like.
	desired, ok := observation.Desired.Lookup("signingKey")
	if !ok || !strings.HasPrefix(desired.Scalar, "sha256:") {
		t.Fatalf("desired signingKey = %#v", desired)
	}
	observed, _ := observation.Value("signingKey")
	if observed.Scalar != desired.Scalar {
		t.Errorf("signingKey observed %q, desired %q", observed.Scalar, desired.Scalar)
	}
}

// A rotated key is the change this type exists to make visible, and it is invisible
// unless the key is compared rather than just its path.
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

func TestADeletedKeyringShowsAsDrift(t *testing.T) {
	f := newRepoFixture(t)
	req := f.request(signed())
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := os.Remove(filepath.Join(f.root, "etc/apt/keyrings/nodesource.asc")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	observation, err := f.provider.Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !observation.Exists {
		t.Fatal("the source itself should still exist")
	}
	if observed, _ := observation.Value("signingKey"); observed.Scalar != "" {
		t.Errorf("signingKey = %q, want empty", observed.Scalar)
	}
}

func TestRemoveTakesOutTheSourceAndTheKeyring(t *testing.T) {
	f := newRepoFixture(t)
	req := f.request(signed())
	if err := f.provider.Apply(context.Background(), req, state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := f.provider.Apply(context.Background(), req, state.Remove); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
	for _, path := range []string{
		"etc/apt/sources.list.d/nodesource.sources",
		"etc/apt/keyrings/nodesource.asc",
	} {
		if _, err := os.Stat(filepath.Join(f.root, path)); !os.IsNotExist(err) {
			t.Errorf("%s survived removal", path)
		}
	}
}

func TestRemovingASourceThatIsNotThereSucceeds(t *testing.T) {
	f := newRepoFixture(t)
	if err := f.provider.Apply(context.Background(), f.request(signed()), state.Remove); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
}

// Dropping a field silently would leave a source that looks applied and is not what
// was asked for.
func TestPriorityIsRefusedRatherThanIgnored(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["priority"] = document.Scalar("600")
	req := f.request(desired)

	if err := f.provider.Apply(context.Background(), req, state.Create); err == nil {
		t.Error("priority was accepted")
	}
	if _, err := f.provider.Observe(context.Background(), req); err == nil {
		t.Error("priority was accepted on observe")
	}
}

func TestAFlatSourceGetsASlashSuite(t *testing.T) {
	f := newRepoFixture(t)
	desired := map[string]document.Value{
		"id":         document.Scalar("flat"),
		"url":        document.Scalar("https://packages.example.com/debian/"),
		"signingKey": document.Scalar("files/node.asc"),
	}
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if stanza := f.sources(t); !strings.Contains(stanza, "Suites: /") {
		t.Errorf("flat source has no suite:\n%s", stanza)
	}
}

func TestADisabledSourceIsWrittenDisabled(t *testing.T) {
	f := newRepoFixture(t)
	desired := signed()
	desired["enabled"] = document.Scalar("false")
	if err := f.provider.Apply(context.Background(), f.request(desired), state.Create); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if stanza := f.sources(t); !strings.Contains(stanza, "Enabled: no") {
		t.Errorf("source is not disabled:\n%s", stanza)
	}
}

// The id becomes a filename, so a traversal in it would write outside the source
// directory.
func TestARepositoryIdIsRefusedWhenItCouldEscapeThePath(t *testing.T) {
	f := newRepoFixture(t)
	for _, id := range []string{"../evil", "a b", "a/b", "a;b"} {
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
