// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// request builds a provider request from a flat field map. The target is taken
// from the path field for the types that use it.
func request(typeName, name string, fields map[string]string) provider.Request {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	target := name
	if path, ok := fields["path"]; ok {
		target = path
	}
	return provider.Request{
		Ref:     document.Reference{Type: typeName, Name: name},
		Target:  target,
		Desired: desired,
	}
}

func observe(t *testing.T, req provider.Request) provider.Observation {
	t.Helper()
	got, err := New().Observe(context.Background(), req)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	return got
}

// requireLinux skips a test that changes the filesystem, because applying is only
// implemented where the safety rules can be followed.
func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("applying is implemented on Linux only, this is %s", runtime.GOOS)
	}
}

func TestObserveMissingFile(t *testing.T) {
	req := request("File", "f", map[string]string{"path": filepath.Join(t.TempDir(), "nothing")})
	got := observe(t, req)
	if got.Exists {
		t.Error("a file that is not there should not exist")
	}
}

func TestObserveExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("hello\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "f", map[string]string{"path": path}))
	if !got.Exists {
		t.Fatal("the file should exist")
	}
	if mode, _ := got.Value("mode"); mode.Scalar != "0640" {
		t.Errorf("mode = %q, want 0640", mode.Scalar)
	}
	if owner, ok := got.Value("owner"); !ok || owner.Scalar == "" {
		t.Errorf("owner = %q, %v", owner.Scalar, ok)
	}
}

// A mode is reported as four octal digits, so a setuid bit is visible rather than
// silently dropped.
func TestObserveReportsSetuid(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "helper")
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	// Go's FileMode keeps setuid in a high bit of its own instead of the octal, so the
	// constant has to be used and not 0o4755.
	if err := os.Chmod(path, 0o755|fs.ModeSetuid); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "helper", map[string]string{"path": path}))
	if mode, _ := got.Value("mode"); mode.Scalar != "04755" {
		t.Errorf("mode = %q, want 04755", mode.Scalar)
	}
}

// A path holding something of the wrong kind is reported as absent with what was found,
// so a plan can say the path is occupied instead of reporting a mismatch on every
// field.
func TestObserveWrongKindAtTheTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "occupied")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "f", map[string]string{"path": path}))
	if got.Exists {
		t.Error("a directory is not the file this resource manages")
	}
	if got.Found != "a directory" {
		t.Errorf("Found = %q", got.Found)
	}
}

// A symbolic link is observed as a link, not as whatever it points at, which is what
// keeps File and Symlink from fighting over one path.
func TestObserveSymlinkIsNotFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	asFile := observe(t, request("File", "f", map[string]string{"path": link}))
	if asFile.Exists {
		t.Error("a link is not a regular file")
	}
	if asFile.Found != "a symbolic link" {
		t.Errorf("Found = %q", asFile.Found)
	}

	asLink := observe(t, request("Symlink", "l", map[string]string{"path": link}))
	if !asLink.Exists {
		t.Fatal("the link should exist as a Symlink")
	}
	if got, _ := asLink.Value("target"); got.Scalar != target {
		t.Errorf("target = %q, want %q", got.Scalar, target)
	}
}

// Content is declared as text or a repository path and read back as bytes, so both
// sides are reduced to a digest. That is what makes them comparable at all.
func TestContentIsComparedAsADigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := request("File", "f", map[string]string{"path": path, "content": "same\n"})
	got := observe(t, req)

	want, ok := got.Desired.Lookup("content")
	if !ok {
		t.Fatal("the provider should normalise desired content to a digest")
	}
	have, ok := got.Value("content")
	if !ok {
		t.Fatal("observed content is missing")
	}
	if want.Scalar != have.Scalar {
		t.Errorf("identical content produced different digests:\n%s\n%s", want.Scalar, have.Scalar)
	}
	if !strings.HasPrefix(have.Scalar, "sha256:") {
		t.Errorf("content digest = %q", have.Scalar)
	}
}

func TestDifferentContentProducesDifferentDigests(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "f", map[string]string{"path": path, "content": "declared\n"}))
	want, _ := got.Desired.Lookup("content")
	have, _ := got.Value("content")
	if want.Scalar == have.Scalar {
		t.Error("different content should produce different digests")
	}
}

// A resource managing metadata only says nothing about content, so there is nothing
// to compare and no digest is produced.
func TestMetadataOnlyResourceDoesNotCompareContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("whatever\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "f", map[string]string{"path": path, "mode": "0644"}))
	if _, ok := got.Value("content"); ok {
		t.Error("content should not be observed when the resource does not declare it")
	}
	if _, ok := got.Desired.Lookup("content"); ok {
		t.Error("no desired content digest should be produced")
	}
}

// A secret is resolved on the host during apply and never enters the manifest, so
// it cannot be compared during a read.
func TestSecretContentIsUnobservable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credential")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := observe(t, request("File", "f", map[string]string{"path": path, "secretRef": "app/password"}))
	if got.CanObserve("content") {
		t.Error("content backed by a secret should be reported as unobservable")
	}
}

func TestContentFromASourceInTheRepository(t *testing.T) {
	repo := t.TempDir()
	layer := filepath.Join(repo, "roles", "web", "files")
	if err := os.MkdirAll(layer, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer, "app.conf"), []byte("from the repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	host := t.TempDir()
	path := filepath.Join(host, "app.conf")
	if err := os.WriteFile(path, []byte("from the repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := request("File", "f", map[string]string{"path": path, "source": "files/app.conf"})
	req.RepoRoot = repo
	req.LayerDir = "roles/web"

	got := observe(t, req)
	want, _ := got.Desired.Lookup("content")
	have, _ := got.Value("content")
	if want.Scalar != have.Scalar {
		t.Error("the file matches its source but the digests differ")
	}
}

// Without confinement, write access to any layer would be read access to every
// file the agent can reach.
func TestSourceCannotEscapeTheRepository(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "roles", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("not yours\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := request("File", "f", map[string]string{
		"path":   filepath.Join(t.TempDir(), "app.conf"),
		"source": "../../../../" + strings.TrimPrefix(outside, "/"),
	})
	req.RepoRoot = repo
	req.LayerDir = "roles/web"

	if _, err := New().Observe(context.Background(), req); err == nil {
		t.Fatal("a source outside the repository should be refused")
	}
}

func TestObserveMissingDirectory(t *testing.T) {
	got := observe(t, request("Directory", "d", map[string]string{
		"path": filepath.Join(t.TempDir(), "nothing"),
	}))
	if got.Exists {
		t.Error("the directory should not exist")
	}
}

func TestProviderIdentity(t *testing.T) {
	p := New()
	if p.Name() != "posix-file" {
		t.Errorf("Name = %q", p.Name())
	}
	want := "File,Directory,Symlink"
	if got := strings.Join(p.Types(), ","); got != want {
		t.Errorf("Types = %s, want %s", got, want)
	}
}

func TestApplyRejectsAnUnknownAction(t *testing.T) {
	req := request("File", "f", map[string]string{"path": filepath.Join(t.TempDir(), "f")})
	if err := New().Apply(context.Background(), req, state.Action(99)); err == nil {
		t.Error("an unexpected action should be refused")
	}
}

func TestApplyDoesNothingForNoneOrSkip(t *testing.T) {
	req := request("File", "f", map[string]string{"path": filepath.Join(t.TempDir(), "f")})
	for _, action := range []state.Action{state.None, state.Skip} {
		if err := New().Apply(context.Background(), req, action); err != nil {
			t.Errorf("Apply(%s) = %v", action, err)
		}
	}
	if _, err := os.Stat(req.Target); !os.IsNotExist(err) {
		t.Error("nothing should have been created")
	}
}

func TestModeText(t *testing.T) {
	cases := map[os.FileMode]string{
		0o644: "0644",
		0o600: "0600",
		0o755: "0755",
	}
	for mode, want := range cases {
		if got := modeText(mode); got != want {
			t.Errorf("modeText(%o) = %q, want %q", mode, got, want)
		}
	}
}

func TestParseMode(t *testing.T) {
	got, err := parseMode("0640")
	if err != nil || got != 0o640 {
		t.Errorf("parseMode(0640) = %o, %v", got, err)
	}
	if _, err := parseMode("nonsense"); err == nil {
		t.Error("a non-octal mode should be refused")
	}
}
