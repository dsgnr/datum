// SPDX-License-Identifier: Apache-2.0

package statedir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")

	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != Mode {
		t.Errorf("mode = %04o, want %04o", info.Mode().Perm(), Mode)
	}
}

func TestEnsureAcceptsAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, Mode); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsAWidenedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Check(dir)
	if err == nil {
		t.Fatal("a world-readable state directory should be refused")
	}
	if !strings.Contains(err.Error(), "0755") {
		t.Errorf("the error should name the mode found, got %v", err)
	}
}

// Refusing rather than correcting, because a silent chmod hides the fact that the
// directory had been readable.
func TestCheckDoesNotCorrectPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	Check(dir)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %04o, want it left alone", info.Mode().Perm())
	}
}

func TestCheckRejectsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, nil, FileMode); err != nil {
		t.Fatal(err)
	}

	if err := Check(path); err == nil {
		t.Fatal("a file standing in for the state directory should be refused")
	}
}

func TestCheckReportsAMissingDirectory(t *testing.T) {
	if err := Check(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing state directory should be reported")
	}
}
