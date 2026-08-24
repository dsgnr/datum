// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/metrics"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/state"
)

func newRegistry() *metrics.Registry {
	cfg := config.Default()
	cfg.Host = "web-001"
	return metrics.NewRegistry(cfg)
}

func sample(t *testing.T, text, series string) (float64, bool) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, " ")
		if !found || name != series {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatalf("value of %s is %q", series, value)
		}
		return parsed, true
	}
	return 0, false
}

func mustSample(t *testing.T, text, series string) float64 {
	t.Helper()
	value, ok := sample(t, text, series)
	if !ok {
		t.Fatalf("%s is missing from:\n%s", series, text)
	}
	return value
}

func passReport() report.Report {
	started := time.Date(2026, 2, 8, 9, 14, 0, 0, time.UTC)
	return report.Report{
		Host:              "web-001",
		RevisionAttempted: "8b91f20",
		RevisionApplied:   "8b91f20",
		Manifest:          "sha256:3f2a9c4e",
		Outcome:           "converged",
		HostState:         "converged",
		Mode:              "enforce",
		StartedAt:         started,
		FinishedAt:        started.Add(183 * time.Millisecond),
		DurationMS:        183,
		Counts:            report.Counts{Total: 14, Converged: 14},
	}
}

// The catalogue in the documentation is the contract, so every name in it has to be
// exported.
func TestEveryDocumentedMetricIsExported(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)
	r.SetTrust([]string{"SHA256:abc"}, true)
	text := r.Render()

	for _, name := range []string{
		"datum_pass_last_attempt_timestamp_seconds",
		"datum_pass_last_success_timestamp_seconds",
		"datum_pass_duration_seconds",
		"datum_passes_total",
		"datum_host_state",
		"datum_reboot_required",
		"datum_mode",
		"datum_resources_total",
		"datum_resources",
		"datum_revision_attempted_timestamp_seconds",
		"datum_revision_attempted_info",
		"datum_revision_applied_timestamp_seconds",
		"datum_revision_applied_info",
		"datum_trust_require",
		"datum_trust_signer_info",
		"datum_trust_baseline_present",
		"datum_revisions_refused_total",
		"datum_resources_refused_total",
		"datum_pass_consecutive_failures",
	} {
		if !strings.Contains(text, "# TYPE "+name+" ") {
			t.Errorf("%s is not exported", name)
		}
	}
}

// An age would be wrong the moment nothing recomputes it, which is exactly the wedged
// agent these metrics exist to reveal.
func TestNoMetricReportsAnElapsedTime(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)
	for _, line := range strings.Split(r.Render(), "\n") {
		if !strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		name := strings.Fields(line)[2]
		if strings.Contains(name, "seconds_since") || strings.Contains(name, "_age_") {
			t.Errorf("%s reports an age", name)
		}
	}
}

func TestTimestampsAreAbsoluteUnixSeconds(t *testing.T) {
	r := newRegistry()
	rep := passReport()
	r.RecordPass(rep, 0)
	text := r.Render()

	want := float64(rep.StartedAt.Unix())
	if got := mustSample(t, text, "datum_pass_last_attempt_timestamp_seconds"); got != want {
		t.Errorf("attempt timestamp = %v, want %v", got, want)
	}
	if got := mustSample(t, text, "datum_pass_duration_seconds"); got != 0.183 {
		t.Errorf("duration = %v", got)
	}
}

// A host that has never reconciled must not report a pass in 1970.
func TestAnAgentThatHasNotReconciledReportsZeroTimestamps(t *testing.T) {
	text := newRegistry().Render()
	for _, series := range []string{
		"datum_pass_last_attempt_timestamp_seconds",
		"datum_pass_last_success_timestamp_seconds",
	} {
		if got := mustSample(t, text, series); got != 0 {
			t.Errorf("%s = %v, want 0", series, got)
		}
	}
}

// A state that is not current has to be exported as zero. A series that vanishes
// cannot be graphed or alerted on.
func TestEveryHostStateIsExportedIncludingTheInactiveOnes(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)
	text := r.Render()

	if got := mustSample(t, text, `datum_host_state{state="converged"}`); got != 1 {
		t.Errorf("converged = %v, want 1", got)
	}
	for _, other := range []string{"drifted", "degraded", "failed", "awaiting-reboot", "unknown"} {
		series := `datum_host_state{state="` + other + `"}`
		if got := mustSample(t, text, series); got != 0 {
			t.Errorf("%s = %v, want 0", series, got)
		}
	}
}

func TestPassOutcomesAccumulate(t *testing.T) {
	r := newRegistry()
	rep := passReport()
	r.RecordPass(rep, 0)
	r.RecordPass(rep, 0)
	failed := rep
	failed.Outcome = "failed"
	r.RecordPass(failed, 1)

	text := r.Render()
	if got := mustSample(t, text, `datum_passes_total{outcome="converged"}`); got != 2 {
		t.Errorf("converged passes = %v, want 2", got)
	}
	if got := mustSample(t, text, `datum_passes_total{outcome="failed"}`); got != 1 {
		t.Errorf("failed passes = %v, want 1", got)
	}
	if got := mustSample(t, text, "datum_pass_consecutive_failures"); got != 1 {
		t.Errorf("consecutive failures = %v", got)
	}
}

// A failed pass must not move the success timestamp, or staleness detection is blind.
func TestAFailedPassDoesNotMoveTheSuccessTimestamp(t *testing.T) {
	r := newRegistry()
	good := passReport()
	r.RecordPass(good, 0)
	success := mustSample(t, r.Render(), "datum_pass_last_success_timestamp_seconds")

	failed := passReport()
	failed.Outcome = "failed"
	failed.StartedAt = good.StartedAt.Add(time.Hour)
	failed.FinishedAt = failed.StartedAt.Add(time.Second)
	r.RecordPass(failed, 1)

	text := r.Render()
	if got := mustSample(t, text, "datum_pass_last_success_timestamp_seconds"); got != success {
		t.Errorf("success timestamp moved to %v", got)
	}
	if got := mustSample(t, text, "datum_pass_last_attempt_timestamp_seconds"); got == success {
		t.Error("attempt timestamp did not move")
	}
}

func TestRefusalsAreCountedByReason(t *testing.T) {
	r := newRegistry()
	r.RefuseRevision("unsigned")
	r.RefuseRevision("unsigned")
	r.RefuseResource("trust-anchor")
	text := r.Render()

	if got := mustSample(t, text, `datum_revisions_refused_total{reason="unsigned"}`); got != 2 {
		t.Errorf("unsigned refusals = %v", got)
	}
	if got := mustSample(t, text, `datum_resources_refused_total{reason="trust-anchor"}`); got != 1 {
		t.Errorf("trust-anchor refusals = %v", got)
	}
	// Reasons that have not fired are still exported, so a dashboard has the series.
	if got := mustSample(t, text, `datum_revisions_refused_total{reason="not-descendant"}`); got != 0 {
		t.Errorf("not-descendant = %v, want 0", got)
	}
}

// Claiming a last known good revision when nothing computes one would report a control
// that is not running.
func TestLastKnownGoodIsAbsentUntilSomethingSetsIt(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)
	if strings.Contains(r.Render(), "datum_last_known_good_info") {
		t.Error("last known good is exported without being computed")
	}

	withGood := passReport()
	withGood.LastKnownGood = "7ab21f"
	r.RecordPass(withGood, 0)
	if !strings.Contains(r.Render(), `datum_last_known_good_info{revision="7ab21f"}`) {
		t.Error("last known good is not exported once set")
	}
}

func TestTrustModeReportsTheConfiguredValue(t *testing.T) {
	cfg := config.Default()
	cfg.Host = "web-001"
	cfg.Trust.Require = config.RequireNone
	r := metrics.NewRegistry(cfg)

	text := r.Render()
	if got := mustSample(t, text, `datum_trust_require{mode="none"}`); got != 1 {
		t.Errorf("none = %v, want 1", got)
	}
	if got := mustSample(t, text, `datum_trust_require{mode="signed-commit"}`); got != 0 {
		t.Errorf("signed-commit = %v, want 0", got)
	}
}

// The format is strict enough that a stray character breaks a whole scrape, so the
// output is checked line by line rather than trusted.
func TestOutputParsesAsTheExpositionFormat(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 2)
	r.SetTrust([]string{`weird"key`}, true)

	line := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})? -?[0-9.eE+]+$`)
	for _, text := range strings.Split(strings.TrimRight(r.Render(), "\n"), "\n") {
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if !line.MatchString(text) {
			t.Errorf("unparseable line %q", text)
		}
	}
	if !strings.Contains(r.Render(), `keyid="weird\"key"`) {
		t.Error("a quote in a label value was not escaped")
	}
}

func TestServingDoesNotTriggerAPass(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)

	srv, err := metrics.Listen("127.0.0.1:0", r)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	// Scraped twice, because an endpoint that did work on demand would differ.
	first := scrape(t, srv.Address())
	second := scrape(t, srv.Address())
	if first != second {
		t.Error("two scrapes returned different bodies, so something moved")
	}
	if !strings.Contains(first, "datum_pass_duration_seconds") {
		t.Errorf("body does not look like metrics:\n%s", first)
	}
}

func TestTheWrongPathSaysWhereMetricsAre(t *testing.T) {
	r := newRegistry()
	srv, err := metrics.Listen("127.0.0.1:0", r)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	resp, err := http.Get("http://" + srv.Address() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/metrics") {
		t.Errorf("body = %q", body)
	}
}

// A port already in use has to fail at startup rather than leave a service that looks
// healthy and answers nothing.
func TestAnAddressInUseIsAnErrorAtStartup(t *testing.T) {
	r := newRegistry()
	first, err := metrics.Listen("127.0.0.1:0", r)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { first.Close() })

	if _, err := metrics.Listen(first.Address(), r); err == nil {
		t.Error("binding an address already in use succeeded")
	}
}

func TestTextfileCarriesTheSameMetrics(t *testing.T) {
	r := newRegistry()
	r.RecordPass(passReport(), 0)

	path := filepath.Join(t.TempDir(), "textfile", "datum.prom")
	if err := metrics.WriteTextfile(path, r); err != nil {
		t.Fatalf("WriteTextfile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading textfile: %v", err)
	}
	if string(data) != r.Render() {
		t.Error("the textfile and the listener disagree")
	}
	// A collector usually runs as a different user, and nothing here is secret.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %04o", info.Mode().Perm())
	}
}

func scrape(t *testing.T, address string) string {
	t.Helper()
	resp, err := http.Get("http://" + address + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(body)
}

// The outcome counter has to carry a series for every outcome a pass can have. A
// missing one silently counts nothing, which is how two drifted passes came to be
// reported as zero passes.
func TestEveryPassOutcomeHasASeries(t *testing.T) {
	r := newRegistry()
	for _, outcome := range state.Outcomes() {
		rep := passReport()
		rep.Outcome = outcome.String()
		r.RecordPass(rep, 0)
	}

	text := r.Render()
	for _, outcome := range state.Outcomes() {
		series := `datum_passes_total{outcome="` + outcome.String() + `"}`
		got, ok := sample(t, text, series)
		if !ok {
			t.Errorf("%s is missing", series)
			continue
		}
		if got != 1 {
			t.Errorf("%s = %v, want 1", series, got)
		}
	}
}

// An observe-mode pass reports drift and is not a failure, so it must not move the
// consecutive-failure gauge or the success timestamp.
func TestADriftedPassCountsAsASuccessfulPass(t *testing.T) {
	r := newRegistry()
	rep := passReport()
	rep.Outcome = state.OutcomeDrifted.String()
	r.RecordPass(rep, 0)

	text := r.Render()
	if got := mustSample(t, text, `datum_passes_total{outcome="drifted"}`); got != 1 {
		t.Errorf("drifted passes = %v, want 1", got)
	}
	if got := mustSample(t, text, "datum_pass_last_success_timestamp_seconds"); got == 0 {
		t.Error("a drifted pass did not count as a success")
	}
}
