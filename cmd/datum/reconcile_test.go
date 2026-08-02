// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/lock"
)

// stateDir is a state directory at the mode Datum insists on. t.TempDir hands out
// 0755, which a pass refuses.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Applying is Linux-only, because the posix provider will not approximate the safety
// rules it relies on anywhere else.
func requireApply(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("applying needs Linux")
	}
}

func TestReconcileAppliesAndVerifies(t *testing.T) {
	requireApply(t)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "stale.conf"), []byte("left over\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", stateDir(t))
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "outcome    changed") {
		t.Errorf("output should report a changed pass, got:\n%s", got.out)
	}

	data, err := os.ReadFile(filepath.Join(target, "app.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "declared\n" {
		t.Errorf("content = %q, want the declared content", data)
	}
	if _, err := os.Lstat(filepath.Join(target, "stale.conf")); !os.IsNotExist(err) {
		t.Error("the resource declared absent should have been removed")
	}
}

func TestReconcileOnAConvergedHostChangesNothing(t *testing.T) {
	requireApply(t)
	target := t.TempDir()
	dir := hostFleet(t, target)
	state := stateDir(t)

	if got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state); got.code != exitOK {
		t.Fatalf("first pass: code = %d, output:\n%s", got.code, got.all())
	}

	got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "outcome    converged") {
		t.Errorf("a second pass should have nothing to do, got:\n%s", got.out)
	}
}

// Observe mode runs the same observation and the same diff as enforce and applies
// none of it.
func TestReconcileInObserveModeChangesNothing(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "app.conf")
	if err := os.WriteFile(path, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := hostFleet(t, target)

	got := invoke("reconcile", "-host", "web-001", "-repo", dir,
		"-state", stateDir(t), "-mode", "observe")
	if got.code != exitDiffers {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitDiffers, got.all())
	}
	if !strings.Contains(got.out, "outcome    drifted") {
		t.Errorf("output should report drift, got:\n%s", got.out)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "untouched\n" {
		t.Errorf("observe mode changed the file: %q", data)
	}
}

func TestReconcileRejectsAnUnknownMode(t *testing.T) {
	dir := hostFleet(t, t.TempDir())

	got := invoke("reconcile", "-host", "web-001", "-repo", dir,
		"-state", stateDir(t), "-mode", "suggest")
	if got.code != exitError {
		t.Fatalf("code = %d, want %d", got.code, exitError)
	}
	if !strings.Contains(got.err, "unknown mode") {
		t.Errorf("stderr = %q", got.err)
	}
}

func TestReconcileRefusesAWidenedStateDirectory(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := t.TempDir()
	if err := os.Chmod(state, 0o755); err != nil {
		t.Fatal(err)
	}

	got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state)
	if got.code != exitError {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitError, got.all())
	}
	if !strings.Contains(got.err, "state directory") {
		t.Errorf("stderr = %q", got.err)
	}
}

func TestReconcileWritesAReport(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := stateDir(t)

	invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state, "-mode", "observe")

	entries, err := os.ReadDir(filepath.Join(state, "reports"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d reports, want 1", len(entries))
	}

	body, err := os.ReadFile(filepath.Join(state, "reports", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("the report should be valid JSON: %v", err)
	}
	if decoded["host"] != "web-001" {
		t.Errorf("host = %v", decoded["host"])
	}
}

func TestStatusBeforeAnyPass(t *testing.T) {
	got := invoke("status", "-state", stateDir(t))
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "no pass has run on this host yet") {
		t.Errorf("output = %q", got.out)
	}
}

func TestStatusReadsTheLastPass(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := stateDir(t)
	invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state, "-mode", "observe")

	got := invoke("status", "-state", state)
	// This fleet declares a Package, which nothing here supports, so the host is
	// degraded. A coverage gap outranks drift in the host state.
	if got.code != exitOK {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitOK, got.all())
	}
	for _, want := range []string{
		"host", "web-001", "revisionAttempted", "revisionApplied",
		"condition", "degraded", "outcome", "drifted", "resources", "skipped",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// Status is read by other tools as well as by people, so the structured form has to
// carry the same field names.
func TestStatusJSON(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := stateDir(t)
	invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state, "-mode", "observe")

	got := invoke("status", "-state", state, "-output", "json")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got.out), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got.out)
	}
	for _, want := range []string{"revisionAttempted", "revisionApplied", "hostState", "counts", "resources"} {
		if _, ok := decoded[want]; !ok {
			t.Errorf("missing field %q", want)
		}
	}
}

func TestStatusRejectsAnUnknownOutput(t *testing.T) {
	got := invoke("status", "-state", stateDir(t), "-output", "yaml")
	if got.code != exitError || !strings.Contains(got.err, "unknown output") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

// Two passes on one host must not run at once, and the second says so rather than
// blocking without explanation.
func TestReconcileRefusesWhenTheLockIsHeld(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := stateDir(t)

	// An flock belongs to the open file description, so taking it here contends with
	// the command even though both are in this process.
	held, err := lock.Acquire(state, false)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state)
	if got.code != exitLockHeld {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitLockHeld, got.all())
	}
	if !strings.Contains(got.err, "another pass is running") {
		t.Errorf("stderr = %q", got.err)
	}
}

// driftOnlyFleet declares nothing this host cannot reconcile, so its host state is
// about drift alone.
func driftOnlyFleet(t *testing.T, target string) string {
	t.Helper()
	return fleet(t, map[string]string{
		"fleet/datum.yaml":      "datum: v1alpha1\ntype: Fleet\nname: example\n",
		"fleet/base/layer.yaml": "datum: v1alpha1\ntype: Layer\nname: base\nprecedence: 0\n",
		"fleet/base/files.yaml": `datum: v1alpha1
type: File
name: app-config
desired:
  path: ` + filepath.Join(target, "app.conf") + `
  mode: "0640"
  content: "declared\n"
`,
		"fleet/hosts/web-001/host.yaml": "datum: v1alpha1\ntype: Host\nname: web-001\nlabels:\n  role: web\n",
	})
}

// A drifted host gets its own exit code, so a check does not have to parse output.
func TestStatusOnADriftedHostExitsTwo(t *testing.T) {
	target := t.TempDir()
	dir := driftOnlyFleet(t, target)
	state := stateDir(t)

	invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state, "-mode", "observe")

	got := invoke("status", "-state", state)
	if got.code != exitDiffers {
		t.Fatalf("code = %d, want %d. output:\n%s", got.code, exitDiffers, got.all())
	}
	if !strings.Contains(got.out, "condition") || !strings.Contains(got.out, "drifted") {
		t.Errorf("output should report drift, got:\n%s", got.out)
	}
}

// A host whose manifest is fully applied reports converged and exits zero.
func TestStatusOnAConvergedHostExitsZero(t *testing.T) {
	requireApply(t)
	target := t.TempDir()
	dir := driftOnlyFleet(t, target)
	state := stateDir(t)

	if got := invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state); got.code != exitOK {
		t.Fatalf("reconcile: code = %d, output:\n%s", got.code, got.all())
	}

	got := invoke("status", "-state", state)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "converged") {
		t.Errorf("output should report a converged host, got:\n%s", got.out)
	}
}

// Every resource on request, since the summary hides the converged ones.
func TestStatusCanListEveryResource(t *testing.T) {
	dir := hostFleet(t, t.TempDir())
	state := stateDir(t)
	invoke("reconcile", "-host", "web-001", "-repo", dir, "-state", state, "-mode", "observe")

	summary := invoke("status", "-state", state)
	if strings.Contains(summary.out, "converged  File[stale]") {
		t.Errorf("the summary should leave converged resources out, got:\n%s", summary.out)
	}

	full := invoke("status", "-state", state, "-resources")
	if !strings.Contains(full.out, "converged") || !strings.Contains(full.out, "File[stale]") {
		t.Errorf("the full listing should include converged resources, got:\n%s", full.out)
	}
}
