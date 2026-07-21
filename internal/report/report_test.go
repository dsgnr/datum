// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/state"
)

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func at(minute int) time.Time {
	return time.Date(2026, 2, 8, 9, minute, 0, 0, time.UTC)
}

func sample(minute int) Report {
	r := Report{
		Host:              "web-001",
		RevisionAttempted: "8b91f20",
		RevisionApplied:   "8b91f20",
		Outcome:           state.OutcomeChanged.String(),
		HostState:         state.HostConverged.String(),
		StartedAt:         at(minute),
		FinishedAt:        at(minute).Add(200 * time.Millisecond),
	}
	r.Duration()
	return r
}

func TestWriteAndRead(t *testing.T) {
	dir := stateDir(t)

	if _, err := Write(dir, sample(14)); err != nil {
		t.Fatal(err)
	}

	got, ok, err := Latest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the report that was just written should be found")
	}
	if got.Host != "web-001" || got.RevisionApplied != "8b91f20" {
		t.Errorf("read back %+v", got)
	}
	if !got.FinishedAt.Equal(sample(14).FinishedAt) {
		t.Errorf("finishedAt = %v", got.FinishedAt)
	}
	if got.DurationMS != 200 {
		t.Errorf("durationMs = %d, want 200", got.DurationMS)
	}
}

func TestLatestOnAHostThatHasNeverRun(t *testing.T) {
	_, ok, err := Latest(stateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a host with no reports should report none rather than fail")
	}
}

func TestLatestPicksTheNewest(t *testing.T) {
	dir := stateDir(t)
	for _, minute := range []int{14, 44, 29} {
		if _, err := Write(dir, sample(minute)); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := Latest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.StartedAt.Equal(at(44)) {
		t.Errorf("startedAt = %v, want the 09:44 pass", got.StartedAt)
	}
}

func TestHistoryIsNewestFirst(t *testing.T) {
	dir := stateDir(t)
	for _, minute := range []int{14, 29, 44} {
		if _, err := Write(dir, sample(minute)); err != nil {
			t.Fatal(err)
		}
	}

	history, err := History(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("got %d reports, want 3", len(history))
	}
	if !history[0].StartedAt.Equal(at(44)) || !history[2].StartedAt.Equal(at(14)) {
		t.Errorf("order = %v, %v", history[0].StartedAt, history[2].StartedAt)
	}
}

func TestWritePrunesOldReports(t *testing.T) {
	dir := stateDir(t)
	for minute := 0; minute < Keep+5; minute++ {
		if _, err := Write(dir, sample(minute)); err != nil {
			t.Fatal(err)
		}
	}

	history, err := History(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != Keep {
		t.Fatalf("kept %d reports, want %d", len(history), Keep)
	}
	// The newest are the ones retained.
	if !history[0].StartedAt.Equal(at(Keep + 4)) {
		t.Errorf("newest retained = %v", history[0].StartedAt)
	}
}

// A report can name a file whose content should not be read by other local users.
func TestReportFilesAreNotReadableByOthers(t *testing.T) {
	dir := stateDir(t)

	path, err := Write(dir, sample(14))
	if err != nil {
		t.Fatal(err)
	}

	file, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode().Perm() != 0o600 {
		t.Errorf("report mode = %04o, want 0600", file.Mode().Perm())
	}

	reports, err := os.Stat(Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if reports.Mode().Perm() != 0o700 {
		t.Errorf("reports directory mode = %04o, want 0700", reports.Mode().Perm())
	}
}

func TestWriteRefusesAWidenedStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(dir, sample(14)); err == nil {
		t.Fatal("writing into a world-readable state directory should be refused")
	}
}

// The field names are the ones in the status reference, so a rename is a change of
// interface and not a tidy-up.
func TestJSONFieldNames(t *testing.T) {
	r := sample(14)
	r.LastKnownGood = "8b91f20"
	r.Manifest = "sha256:3f2a9c4e"
	r.Resources = []Resource{{
		Ref:    "File[nginx-conf]",
		Action: state.Update.String(),
		State:  state.Converged.String(),
		Fields: []string{"mode"},
	}}
	r.Counts = Tally(r.Resources)

	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"revisionAttempted", "revisionApplied", "lastKnownGood", "manifest",
		"outcome", "hostState", "startedAt", "finishedAt", "durationMs",
		"counts", "resources", "ref", "action", "state", "fields",
	} {
		if !strings.Contains(string(body), `"`+name+`"`) {
			t.Errorf("missing field %q", name)
		}
	}
}

// Field values would put file content on disk, which is what the report is supposed
// to keep off it.
func TestResourcesCarryFieldNamesWithoutValues(t *testing.T) {
	body, err := json.Marshal(Resource{
		Ref:    "File[app-credentials]",
		Fields: []string{"content"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"desired", "observed", "value", "content"} {
		if _, found := decoded[unwanted]; found {
			t.Errorf("a resource entry should not carry %q", unwanted)
		}
	}
}

func TestTally(t *testing.T) {
	counts := Tally([]Resource{
		{State: state.Converged.String()},
		{State: state.Converged.String()},
		{State: state.Drifted.String()},
		{State: state.Failed.String()},
		{State: state.Blocked.String()},
		{State: state.Skipped.String()},
	})
	want := Counts{Total: 6, Converged: 2, Drifted: 1, Failed: 1, Blocked: 1, Skipped: 1}
	if counts != want {
		t.Errorf("got %+v, want %+v", counts, want)
	}
}

// A half-written report must never be read as a whole one, so nothing appears under
// its final name until it is complete.
func TestWriteLeavesNoTemporaryFiles(t *testing.T) {
	dir := stateDir(t)
	if _, err := Write(dir, sample(14)); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".report-") {
			t.Errorf("leftover temporary file %s", filepath.Join(Dir(dir), entry.Name()))
		}
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}
