//go:build linux

// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

func apply(t *testing.T, req provider.Request, action state.Action) {
	t.Helper()
	if err := New().Apply(context.Background(), req, action); err != nil {
		t.Fatalf("apply %s: %v", action, err)
	}
}

func TestCreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	req := request("File", "f", map[string]string{
		"path": path, "content": "hello\n", "mode": "0640",
	})

	apply(t, req, state.Create)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Errorf("content = %q", data)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640", info.Mode().Perm())
	}
}

// A rename is atomic, so a reader sees either the old content or the new one and
// never a partial write.
func TestUpdateReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := request("File", "f", map[string]string{"path": path, "content": "new\n", "mode": "0600"})
	apply(t, req, state.Update)

	data, _ := os.ReadFile(path)
	if string(data) != "new\n" {
		t.Errorf("content = %q", data)
	}
}

// A resource that declares no content manages metadata only, which is how
// ownership on a file a package created is corrected without taking over what is
// in it.
func TestMetadataOnlyUpdateKeepsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("left alone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	apply(t, request("File", "f", map[string]string{"path": path, "mode": "0600"}), state.Update)

	data, _ := os.ReadFile(path)
	if string(data) != "left alone\n" {
		t.Errorf("content = %q, want it untouched", data)
	}
	info, _ := os.Lstat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
}

// Writing through a link at the managed path is how an unprivileged user redirects
// a root write, so the final component is never followed.
func TestWriteRefusesToFollowALinkAtTheTarget(t *testing.T) {
	dir := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(elsewhere, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "app.conf")
	if err := os.Symlink(elsewhere, path); err != nil {
		t.Fatal(err)
	}

	req := request("File", "f", map[string]string{"path": path, "content": "written\n"})
	if err := New().Apply(context.Background(), req, state.Update); err != nil {
		t.Fatalf("the write should replace the link rather than fail: %v", err)
	}

	// The link is replaced by a regular file, and what it pointed at is untouched.
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("the target should now be a regular file, got %v", info.Mode())
	}
	data, _ := os.ReadFile(elsewhere)
	if string(data) != "original\n" {
		t.Errorf("the link target was written through: %q", data)
	}
}

func TestCreateDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.d")
	apply(t, request("Directory", "d", map[string]string{"path": path, "mode": "0750"}), state.Create)

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("a directory should have been created")
	}
	if info.Mode().Perm() != 0o750 {
		t.Errorf("mode = %o, want 750", info.Mode().Perm())
	}
}

// Missing parents are not created, because a directory nobody described would get
// ownership and a mode nobody chose.
func TestCreateDirectoryDoesNotCreateParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "conf.d")
	req := request("Directory", "d", map[string]string{"path": path})
	if err := New().Apply(context.Background(), req, state.Create); err == nil {
		t.Error("a missing parent should fail rather than be created")
	}
}

func TestCreateSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "link")
	apply(t, request("Symlink", "l", map[string]string{
		"path": path, "target": "/etc/nginx/sites-available/app.conf",
	}), state.Create)

	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/etc/nginx/sites-available/app.conf" {
		t.Errorf("target = %q", got)
	}
}

// Replacing a link is atomic, so a reader sees the old target or the new one and
// never nothing at all.
func TestUpdateSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "link")
	if err := os.Symlink("/old", path); err != nil {
		t.Fatal(err)
	}

	apply(t, request("Symlink", "l", map[string]string{"path": path, "target": "/new"}), state.Update)

	got, _ := os.Readlink(path)
	if got != "/new" {
		t.Errorf("target = %q, want /new", got)
	}
}

// A dangling link is legitimate and sometimes deliberate, such as one into a
// filesystem mounted later.
func TestSymlinkTargetNeedNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "link")
	apply(t, request("Symlink", "l", map[string]string{
		"path": path, "target": "/not/there/yet",
	}), state.Create)

	if _, err := os.Readlink(path); err != nil {
		t.Errorf("the link should have been created: %v", err)
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	apply(t, request("File", "f", map[string]string{"path": path, "state": "absent"}), state.Remove)

	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Error("the file should be gone")
	}
}

// Removing something that is already gone is not an error, because the desired
// state is satisfied either way.
func TestRemoveMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-there")
	apply(t, request("File", "f", map[string]string{"path": path, "state": "absent"}), state.Remove)
}

// Removing a link removes the link and never what it points at, because the link's
// own path is what the resource claims.
func TestRemoveSymlinkLeavesItsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	apply(t, request("Symlink", "l", map[string]string{"path": link, "state": "absent"}), state.Remove)

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("the link should be gone")
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the link target should be untouched: %v", err)
	}
}

// Removal is never recursive, because removing a tree means removing things nobody
// declared.
func TestRemoveDirectoryRefusesWhenNotEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.d")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "undeclared.conf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	req := request("Directory", "d", map[string]string{"path": path, "state": "absent"})
	if err := New().Apply(context.Background(), req, state.Remove); err == nil {
		t.Error("a directory holding undeclared files should not be removed")
	}
}

// Nothing is left behind when a write fails, because the staging file is removed
// and the live one was never touched.
func TestFailedWriteLeavesNoStagingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// An owner that cannot be resolved fails after the staging file is created.
	req := request("File", "f", map[string]string{
		"path": path, "content": "new\n", "owner": "nosuchuserexists",
	})
	if err := New().Apply(context.Background(), req, state.Update); err == nil {
		t.Fatal("an unresolvable owner should fail the write")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "original\n" {
		t.Errorf("the live file was changed: %q", data)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "app.conf" {
			t.Errorf("a staging file was left behind: %s", entry.Name())
		}
	}
}
