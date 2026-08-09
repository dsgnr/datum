// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostFleet writes a fleet whose resources point at a directory the test controls,
// so observing and planning work against a real filesystem without needing root.
func hostFleet(t *testing.T, target string) string {
	t.Helper()
	return fleet(t, map[string]string{
		"fleet/datum.yaml": "datum: v1alpha1\ntype: Fleet\nname: example\n",
		"fleet/base/layer.yaml": `datum: v1alpha1
type: Layer
name: base
precedence: 0
`,
		"fleet/base/files.yaml": `datum: v1alpha1
type: File
name: app-config
desired:
  path: ` + filepath.Join(target, "app.conf") + `
  mode: "0640"
  content: "declared\n"
---
datum: v1alpha1
type: File
name: stale
desired:
  path: ` + filepath.Join(target, "stale.conf") + `
  state: absent
---
datum: v1alpha1
type: Service
name: unmanaged
desired:
  state: running
  enabled: true
`,
		"fleet/hosts/web-001/host.yaml": `datum: v1alpha1
type: Host
name: web-001
labels:
  role: web
`,
	})
}

func TestObserveReportsWhatIsOnDisk(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "app.conf"), []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("observe", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{
		"File[app-config]", "exists", "true", "mode", "0644",
		"File[stale]", "exists", "false",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// A type with no provider is reported as skipped, not as a failure, because a coverage
// gap is not the same as something going wrong.
func TestObserveSkipsTypesWithNoProvider(t *testing.T) {
	dir := hostFleet(t, t.TempDir())

	got := invoke("observe", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "no Service provider") {
		t.Errorf("output should say the service type is unsupported, got:\n%s", got.out)
	}
}

func TestDiffReportsDriftAndExitsTwo(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "app.conf"), []byte("wrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("diff", "-host", "web-001", "-repo", dir)
	// A separate code for differences is what makes this usable as a drift check
	// without parsing the output.
	if got.code != exitDiffers {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitDiffers, got.all())
	}
	for _, want := range []string{"File[app-config]", "mode", "0644 -> 0640", "content"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

func TestDiffOnAConvergedHostExitsZero(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "app.conf")
	if err := os.WriteFile(path, []byte("declared\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Written with the right mode, because the umask may have reduced it.
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("diff", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, want 0. output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "0 resources differ") {
		t.Errorf("output should report no differences, got:\n%s", got.out)
	}
}

// A digest is abbreviated rather than truncated, because a hash cut off mid-way looks
// like corruption.
func TestDiffAbbreviatesDigests(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "app.conf"), []byte("wrong\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("diff", "-host", "web-001", "-repo", dir)
	if strings.Contains(got.out, "...") {
		t.Errorf("a digest should be shortened rather than cut off, got:\n%s", got.out)
	}
	if !strings.Contains(got.out, "sha256:") {
		t.Errorf("output should show digests, got:\n%s", got.out)
	}
}

func TestPlanShowsActionsAndCounts(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "stale.conf"), []byte("left over\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("plan", "-host", "web-001", "-repo", dir)
	if got.code != exitDiffers {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitDiffers, got.all())
	}
	for _, want := range []string{
		"create   File[app-config]",
		"remove   File[stale]",
		"skip     Service[unmanaged]",
		"provider",
		"posix-file",
		"1 to create, 0 to update, 1 to remove, 1 to skip, 0 unchanged",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// A host that cannot reconcile part of its manifest is degraded, and the output states
// that rather than leaving it to be inferred from the skip count.
func TestPlanReportsADegradedHost(t *testing.T) {
	dir := hostFleet(t, t.TempDir())

	got := invoke("plan", "-host", "web-001", "-repo", dir)
	for _, want := range []string{"host state: degraded", "no provider on this host for: Service"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

func TestPassCommandsNeedAHost(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	for _, name := range []string{"observe", "diff", "plan"} {
		got := invoke(name, "-repo", dir)
		if got.code != exitError || !strings.Contains(got.err, "--host is required") {
			t.Errorf("%s: code = %d stderr = %q", name, got.code, got.err)
		}
	}
}

// Every read-only command is safe to run on a production machine, which is by design.
// Nothing here should write to the target.
func TestReadOnlyCommandsChangeNothing(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "app.conf")
	if err := os.WriteFile(path, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	for _, name := range []string{"observe", "diff", "plan"} {
		invoke(name, "-host", "web-001", "-repo", dir)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "untouched\n" {
		t.Errorf("a read-only command changed the file: %q", data)
	}
	// The absent resource must not have been created either.
	if _, err := os.Lstat(filepath.Join(target, "stale.conf")); !os.IsNotExist(err) {
		t.Error("a read-only command created something")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 1 {
		t.Errorf("the target directory has %d entries, want 1", len(entries))
	}
}
