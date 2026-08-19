// SPDX-License-Identifier: Apache-2.0

// Package metrics renders and serves the agent's metrics.
//
// A scrape returns what the last pass recorded. It does not trigger a pass, read the
// host, or touch the repository. An endpoint is scraped far more often than a host is
// reconciled, and one that observed on demand would turn a monitoring system into
// fleet-wide load.
//
// Every time-related metric is an absolute timestamp and none reports an age. An
// elapsed-time gauge is only correct while something keeps recomputing it, so a wedged
// agent would keep serving a value that stopped being true.
package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/state"
)

// hostStates and resourceStates are listed so that a state which is not current is
// exported as zero instead of going missing. A series that disappears cannot be
// graphed.
var hostStates = []string{"converged", "drifted", "degraded", "failed", "awaiting-reboot", "unknown"}

var resourceStates = []string{"converged", "drifted", "failed", "blocked", "skipped"}

// Registry holds what the agent has seen. Counters live here because they have to
// survive between passes, and a report only describes the last one.
type Registry struct {
	mu sync.Mutex

	cfg config.Config

	// offset is the host's schedule position, exported so a fleet can see when a
	// host is due without computing a hash by hand.
	offset time.Duration

	latest    *report.Report
	lastError string

	passes              map[string]int
	consecutiveFailures int
	revisionsRefused    map[string]int
	resourcesRefused    map[string]int

	lastAttempt time.Time
	lastSuccess time.Time
	duration    time.Duration

	rebootRequired bool
	signers        []string
	baseline       bool
}

func NewRegistry(cfg config.Config) *Registry {
	return &Registry{
		cfg:              cfg,
		passes:           map[string]int{},
		revisionsRefused: map[string]int{},
		resourcesRefused: map[string]int{},
	}
}

// SetOffset records the schedule offset for this host.
func (r *Registry) SetOffset(offset time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.offset = offset
}

// SetTrust records what the agent trusts, which is the part of the configuration a
// fleet cannot otherwise see without reading every host's file.
func (r *Registry) SetTrust(signers []string, baselinePresent bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signers = append([]string(nil), signers...)
	r.baseline = baselinePresent
}

// RecordPass takes the result of a completed pass.
func (r *Registry) RecordPass(rep report.Report, failures int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	copied := rep
	r.latest = &copied
	r.lastError = rep.Error
	r.consecutiveFailures = failures

	r.lastAttempt = rep.StartedAt
	r.duration = time.Duration(rep.DurationMS) * time.Millisecond

	// The outcome names in the catalogue are the pass outcomes, so a report carrying
	// anything else is a bug, not something to paper over here.
	r.passes[rep.Outcome]++
	if rep.Outcome != state.OutcomeFailed.String() {
		r.lastSuccess = rep.FinishedAt
	}
	r.rebootRequired = rep.HostState == "awaiting-reboot"
}

// RecordAttempt notes a pass that started, so a pass which never produced a report
// still moves the attempt timestamp. Without it a host whose passes die early looks
// like one that stopped trying.
func (r *Registry) RecordAttempt(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAttempt = at
}

// RecordFailure records a pass that failed before it could produce a report, which is
// what an upstream failure looks like.
func (r *Registry) RecordFailure(reason string, failures int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.passes[state.OutcomeFailed.String()]++
	r.consecutiveFailures = failures
	r.lastError = reason
}

// RefuseRevision counts a revision turned away by a trust control.
func (r *Registry) RefuseRevision(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revisionsRefused[reason]++
}

// RefuseResource counts a resource turned away by a safety control.
func (r *Registry) RefuseResource(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resourcesRefused[reason]++
}

// Render writes the exposition format. It is the same text for the listener and for
// the textfile, so the two cannot disagree.
func (r *Registry) Render() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder
	w := &writer{b: &b}

	w.gauge("datum_pass_last_attempt_timestamp_seconds",
		"Unix time of the last pass attempt", timestamp(r.lastAttempt))
	w.gauge("datum_pass_last_success_timestamp_seconds",
		"Unix time of the last pass that did not fail", timestamp(r.lastSuccess))
	w.gauge("datum_pass_duration_seconds",
		"Duration of the last pass", r.duration.Seconds())

	w.help("datum_passes_total", "counter", "Passes by outcome")
	for _, outcome := range state.Outcomes() {
		w.sample("datum_passes_total", labels{{"outcome", outcome.String()}},
			float64(r.passes[outcome.String()]))
	}

	w.help("datum_host_state", "gauge", "Current host state, 1 for the active state")
	current := ""
	if r.latest != nil {
		current = r.latest.HostState
	}
	for _, name := range hostStates {
		w.sample("datum_host_state", labels{{"state", name}}, boolValue(name == current))
	}

	w.gauge("datum_reboot_required",
		"Whether the host is waiting for a reboot", boolValue(r.rebootRequired))

	w.help("datum_mode", "gauge", "Active reconciliation mode, 1 for the active mode")
	for _, mode := range []string{"enforce", "observe"} {
		w.sample("datum_mode", labels{{"mode", mode}}, boolValue(r.mode() == mode))
	}

	counts := report.Counts{}
	if r.latest != nil {
		counts = r.latest.Counts
	}
	w.gauge("datum_resources_total", "Resources in the effective manifest", float64(counts.Total))
	w.help("datum_resources", "gauge", "Resources by state after the last pass")
	for _, name := range resourceStates {
		w.sample("datum_resources", labels{{"state", name}}, float64(countFor(counts, name)))
	}

	r.renderRevisions(w)
	r.renderTrust(w)

	w.gauge("datum_pass_consecutive_failures",
		"Consecutive failed passes", float64(r.consecutiveFailures))
	w.gauge("datum_schedule_offset_seconds",
		"This host's offset within the reconciliation interval", r.offset.Seconds())

	return b.String()
}

// renderRevisions exports three revisions, not one. A single series would have to mean
// either the newest revision tried or the one actually running, and those are different
// questions.
func (r *Registry) renderRevisions(w *writer) {
	attempted, applied, lastGood, manifest := "", "", "", ""
	var finished time.Time
	if r.latest != nil {
		attempted = r.latest.RevisionAttempted
		applied = r.latest.RevisionApplied
		lastGood = r.latest.LastKnownGood
		manifest = r.latest.Manifest
		finished = r.latest.FinishedAt
	}

	w.gauge("datum_revision_attempted_timestamp_seconds",
		"Unix time the attempted revision was read", timestamp(finished))
	if attempted != "" {
		w.help("datum_revision_attempted_info", "gauge", "The revision last attempted")
		w.sample("datum_revision_attempted_info", labels{{"revision", attempted}}, 1)
	}

	w.gauge("datum_revision_applied_timestamp_seconds",
		"Unix time the applied revision was reconciled", timestamp(finished))
	if applied != "" {
		w.help("datum_revision_applied_info", "gauge", "The revision being reconciled")
		w.sample("datum_revision_applied_info",
			labels{{"revision", applied}, {"manifest", manifest}}, 1)
	}

	// Absent until something resolves a newer revision and falls back. Exporting it as
	// equal to applied would claim a control that is not running.
	if lastGood != "" {
		w.gauge("datum_last_known_good_timestamp_seconds",
			"Unix time the last known good revision was accepted", timestamp(finished))
		w.help("datum_last_known_good_info", "gauge", "Newest revision that resolved and validated")
		w.sample("datum_last_known_good_info", labels{{"revision", lastGood}}, 1)
	}
}

// renderTrust exports the controls. A refusal nothing outside the host can see is
// indistinguishable from a control that was never switched on.
func (r *Registry) renderTrust(w *writer) {
	w.help("datum_trust_require", "gauge", "Configured trust.require, 1 for the active mode")
	for _, mode := range []string{"signed-commit", "signed-tag", "none"} {
		w.sample("datum_trust_require", labels{{"mode", mode}},
			boolValue(string(r.cfg.Trust.Require) == mode))
	}

	if len(r.signers) > 0 {
		w.help("datum_trust_signer_info", "gauge", "One series per trusted signing key")
		for _, keyid := range r.signers {
			w.sample("datum_trust_signer_info", labels{{"keyid", keyid}}, 1)
		}
	}
	w.gauge("datum_trust_baseline_present",
		"Whether the host started with a baseline revision", boolValue(r.baseline))

	w.help("datum_revisions_refused_total", "counter", "Revisions refused, by control")
	for _, reason := range []string{"unsigned", "untrusted-signer", "ambiguous-tag", "not-descendant", "unsupported-schema"} {
		w.sample("datum_revisions_refused_total", labels{{"reason", reason}},
			float64(r.revisionsRefused[reason]))
	}
	w.help("datum_resources_refused_total", "counter", "Resources refused, by control")
	for _, reason := range []string{"trust-anchor", "hard-link", "untrusted-path", "unsafe-mode"} {
		w.sample("datum_resources_refused_total", labels{{"reason", reason}},
			float64(r.resourcesRefused[reason]))
	}
}

func (r *Registry) mode() string {
	if r.latest != nil && r.latest.Mode != "" {
		return r.latest.Mode
	}
	return r.cfg.Reconciliation.Mode
}

func countFor(c report.Counts, name string) int {
	switch name {
	case "converged":
		return c.Converged
	case "drifted":
		return c.Drifted
	case "failed":
		return c.Failed
	case "blocked":
		return c.Blocked
	case "skipped":
		return c.Skipped
	}
	return 0
}

// timestamp returns zero for a time that never happened, so a host that has not
// reconciled reports 0 rather than a date in 1970 that looks like a real pass.
func timestamp(at time.Time) float64 {
	if at.IsZero() {
		return 0
	}
	return float64(at.UnixNano()) / float64(time.Second)
}

func boolValue(yes bool) float64 {
	if yes {
		return 1
	}
	return 0
}

type label struct{ name, value string }

type labels []label

type writer struct {
	b    *strings.Builder
	seen map[string]bool
}

// help writes the type metadata once per family, which the format requires to appear
// before any sample of it.
func (w *writer) help(name, kind, help string) {
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	if w.seen[name] {
		return
	}
	w.seen[name] = true
	fmt.Fprintf(w.b, "# HELP %s %s.\n# TYPE %s %s\n", name, help, name, kind)
}

func (w *writer) gauge(name, help string, value float64) {
	w.help(name, "gauge", help)
	w.sample(name, nil, value)
}

func (w *writer) sample(name string, ls labels, value float64) {
	if len(ls) == 0 {
		fmt.Fprintf(w.b, "%s %s\n", name, format(value))
		return
	}
	sort.Slice(ls, func(i, j int) bool { return ls[i].name < ls[j].name })
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		parts = append(parts, l.name+`="`+escape(l.value)+`"`)
	}
	fmt.Fprintf(w.b, "%s{%s} %s\n", name, strings.Join(parts, ","), format(value))
}

func format(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// escape handles the three characters the exposition format reserves in a label value.
// A revision or key id should never contain them, and a malformed label breaks an
// entire scrape, not one series.
func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}
