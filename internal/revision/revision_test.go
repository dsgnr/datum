// SPDX-License-Identifier: Apache-2.0

package revision_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/revision"
)

const (
	first  = "8b91f2036f4e6b0f5a7c1d2e3f4a5b6c7d8e9f01"
	second = "a41c9d34f5e6b7c8d9e0f1a2b3c4d5e6f7a8b9c0"
)

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

// No recorded revision is first contact, not a failure. A host being provisioned has
// none until something writes one.
func TestAHostWithNoRecordedRevisionIsFirstContact(t *testing.T) {
	got, found, err := revision.Read(stateDir(t))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if found || got != "" {
		t.Errorf("Read = %q, %t", got, found)
	}
}

func TestAdvanceThenRead(t *testing.T) {
	dir := stateDir(t)
	if err := revision.Advance(dir, first); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	got, found, err := revision.Read(dir)
	if err != nil || !found {
		t.Fatalf("Read = %q, %t, %v", got, found, err)
	}
	if got != first {
		t.Errorf("Read = %q, want %q", got, first)
	}

	if err := revision.Advance(dir, second); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	got, _, _ = revision.Read(dir)
	if got != second {
		t.Errorf("after advancing, Read = %q, want %q", got, second)
	}
}

// The file sits in a directory holding plans and the pointer a local attacker would
// rewrite to pin a host at an old signed revision.
func TestTheRecordedRevisionIsNotReadableByOtherUsers(t *testing.T) {
	dir := stateDir(t)
	if err := revision.Advance(dir, first); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	info, err := os.Stat(revision.Path(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %04o, want 0600", perm)
	}
}

func TestClearReportsWhatItForgot(t *testing.T) {
	dir := stateDir(t)
	if err := revision.Advance(dir, first); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	previous, found, err := revision.Clear(dir)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if !found || previous != first {
		t.Errorf("Clear = %q, %t", previous, found)
	}

	// Cleared means back to first contact, so the next signed revision becomes the
	// baseline.
	if _, found, _ := revision.Read(dir); found {
		t.Error("the revision survived being cleared")
	}
}

func TestClearingWhenThereIsNothingToClearSucceeds(t *testing.T) {
	previous, found, err := revision.Clear(stateDir(t))
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if found || previous != "" {
		t.Errorf("Clear = %q, %t", previous, found)
	}
}

// An abbreviation that is unambiguous today can collide as a repository grows, and this
// value decides whether a revision is a downgrade.
func TestAbbreviatedRevisionsAreRefused(t *testing.T) {
	dir := stateDir(t)
	for _, bad := range []string{"8b91f20", "8b91f2036f4e6b0f5a7c1d2e3f4a5b6c7d8e9f0", "", "zzzz"} {
		if err := revision.Advance(dir, bad); err == nil {
			t.Errorf("Advance(%q) was accepted", bad)
		}
	}
}

func TestUppercaseIsRefusedSoComparisonsStayByteForByte(t *testing.T) {
	dir := stateDir(t)
	if err := revision.Advance(dir, strings.ToUpper(first)); err == nil {
		t.Error("an uppercase revision was accepted, so two spellings of one revision could differ")
	}
}

// A truncated pointer has to be loud. Reading it as absent would turn a corrupted file
// into an accepted downgrade.
func TestACorruptedPointerIsRefusedRatherThanTreatedAsAbsent(t *testing.T) {
	dir := stateDir(t)
	if err := os.WriteFile(revision.Path(dir), []byte("8b91f20\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, found, err := revision.Read(dir); err == nil {
		t.Errorf("a truncated revision was accepted as %t", found)
	}
}

func TestTrailingWhitespaceIsTolerated(t *testing.T) {
	dir := stateDir(t)
	if err := os.WriteFile(revision.Path(dir), []byte("  "+first+"  \n\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, found, err := revision.Read(dir)
	if err != nil || !found || got != first {
		t.Errorf("Read = %q, %t, %v", got, found, err)
	}
}

// An empty file is what an interrupted write used to leave behind, and it means the host
// has no baseline rather than that its baseline is the empty string.
func TestAnEmptyFileIsFirstContact(t *testing.T) {
	dir := stateDir(t)
	if err := os.WriteFile(revision.Path(dir), []byte("\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, found, err := revision.Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if found || got != "" {
		t.Errorf("Read = %q, %t", got, found)
	}
}

// Renaming into place is what stops a pass interrupted mid-write leaving a truncated
// pointer for the next one to refuse.
func TestAdvanceLeavesNoTemporaryFileBehind(t *testing.T) {
	dir := stateDir(t)
	if err := revision.Advance(dir, first); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("%s was left behind", entry.Name())
		}
	}
}

func TestAdvanceCreatesTheStateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := revision.Advance(dir, first); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	got, found, err := revision.Read(dir)
	if err != nil || !found || got != first {
		t.Errorf("Read = %q, %t, %v", got, found, err)
	}
}
