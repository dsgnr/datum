// SPDX-License-Identifier: Apache-2.0

// Package report is the record one pass leaves behind.
//
// `datum status` reads these structures, so the field names appear in the status
// reference and changing them changes an interface.
//
// Field values are omitted. A report records which fields differed, since a plan can
// contain file content.
package report

import (
	"time"

	"github.com/dsgnr/datum/internal/state"
)

// Report is one pass.
type Report struct {
	Host string `json:"host"`

	// RevisionAttempted is the newest revision the pass tried to resolve, whether
	// or not it got there. RevisionApplied is the one whose desired state the host
	// is actually reconciling, and the two diverge when a commit fails to resolve.
	RevisionAttempted string `json:"revisionAttempted"`
	RevisionApplied   string `json:"revisionApplied"`
	LastKnownGood     string `json:"lastKnownGood,omitempty"`
	Manifest          string `json:"manifest,omitempty"`

	// Error is why the attempted revision was not applied.
	Error string `json:"error,omitempty"`

	Outcome   string `json:"outcome"`
	HostState string `json:"hostState"`
	Mode      string `json:"mode,omitempty"`

	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	DurationMS int64     `json:"durationMs"`

	Counts    Counts     `json:"counts"`
	Resources []Resource `json:"resources"`
}

// Resource is where one resource ended up.
type Resource struct {
	Ref      string `json:"ref"`
	Provider string `json:"provider,omitempty"`
	Target   string `json:"target,omitempty"`
	Action   string `json:"action"`
	State    string `json:"state"`

	// Fields are the names of the fields that did not match, without their values.
	Fields []string `json:"fields,omitempty"`
	Reason string   `json:"reason,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// Counts is the per-state breakdown status prints.
type Counts struct {
	Total     int `json:"total"`
	Converged int `json:"converged"`
	Drifted   int `json:"drifted"`
	Failed    int `json:"failed"`
	Blocked   int `json:"blocked"`
	Skipped   int `json:"skipped"`
}

// Tally counts the resources by state.
func Tally(resources []Resource) Counts {
	out := Counts{Total: len(resources)}
	for _, r := range resources {
		switch r.State {
		case state.Converged.String():
			out.Converged++
		case state.Drifted.String():
			out.Drifted++
		case state.Failed.String():
			out.Failed++
		case state.Blocked.String():
			out.Blocked++
		case state.Skipped.String():
			out.Skipped++
		}
	}
	return out
}

// Duration fills in the derived timing fields.
func (r *Report) Duration() {
	r.DurationMS = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
}
