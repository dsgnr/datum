// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/agent"
	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/lock"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/schedule"
)

// clock drives the agent without waiting for a real interval. Each tick released is
// one scheduled pass.
type clock struct {
	mu      sync.Mutex
	now     time.Time
	waiting chan chan time.Time
}

func newClock() *clock {
	return &clock{
		now:     time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
		waiting: make(chan chan time.Time, 8),
	}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	c.waiting <- ch
	return ch
}

// tick releases the agent's current wait and returns once it is waiting again, so a
// test can assert on the state a pass left behind.
func (c *clock) tick(t *testing.T) {
	t.Helper()
	select {
	case ch := <-c.waiting:
		ch <- c.Now()
	case <-time.After(2 * time.Second):
		t.Fatal("the agent never waited")
	}
}

type recorder struct {
	mu     sync.Mutex
	calls  int
	report report.Report
	fail   schedule.Failure
	err    error
	block  chan struct{}
	inside chan struct{}
}

func newRecorder() *recorder {
	return &recorder{report: converged()}
}

func (r *recorder) pass(ctx context.Context) (report.Report, schedule.Failure, error) {
	r.mu.Lock()
	r.calls++
	block, inside := r.block, r.inside
	rep, fail, err := r.report, r.fail, r.err
	r.mu.Unlock()

	if inside != nil {
		inside <- struct{}{}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return report.Report{}, schedule.Local, ctx.Err()
		}
	}
	return rep, fail, err
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *recorder) set(rep report.Report, fail schedule.Failure, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.report, r.fail, r.err = rep, fail, err
}

func converged() report.Report {
	started := time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC)
	return report.Report{
		Host:            "web-001",
		RevisionApplied: "8b91f20",
		Outcome:         "converged",
		HostState:       "converged",
		Mode:            "enforce",
		StartedAt:       started,
		FinishedAt:      started.Add(time.Second),
		DurationMS:      1000,
		Counts:          report.Counts{Total: 3, Converged: 3},
	}
}

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// t.TempDir is 0755 and the state directory contract is 0700.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

func newAgent(t *testing.T, cfg config.Config, p agent.Pass, c *clock) *agent.Agent {
	t.Helper()
	a, err := agent.New(agent.Options{
		Config: cfg,
		Pass:   p,
		Log:    &syncBuffer{},
		Now:    c.Now,
		After:  c.After,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func baseConfig(t *testing.T) config.Config {
	cfg := config.Default()
	cfg.Host = "web-001"
	cfg.State = stateDir(t)
	cfg.Metrics.Listen = "none"
	// The agent refuses to start while it cannot honour a verification requirement, so
	// the tests say out loud that these hosts apply unverified desired state.
	cfg.Trust.Require = config.RequireNone
	return cfg
}

// run starts the agent and returns a stop function.
func run(t *testing.T, a *agent.Agent) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Run did not return after the context was cancelled")
		}
	}
}

// Five hundred machines booting together must not all reconcile at once, so the agent
// waits for its offset instead of passing immediately.
func TestThereIsNoPassAtStartup(t *testing.T) {
	c := newClock()
	r := newRecorder()
	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	// Wait for the agent to reach its first wait, then check nothing ran.
	select {
	case ch := <-c.waiting:
		if r.count() != 0 {
			t.Errorf("%d passes ran before the first tick", r.count())
		}
		ch <- c.Now()
	case <-time.After(2 * time.Second):
		t.Fatal("the agent never waited")
	}
}

func TestEachTickRunsOnePass(t *testing.T) {
	c := newClock()
	r := newRecorder()
	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	for i := 1; i <= 3; i++ {
		c.tick(t)
		waitFor(t, func() bool { return r.count() == i })
	}
}

func TestAPassWritesItsResultIntoTheMetrics(t *testing.T) {
	c := newClock()
	r := newRecorder()
	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	waitFor(t, func() bool {
		return strings.Contains(a.Registry().Render(), `datum_passes_total{outcome="converged"} 1`)
	})
	if !strings.Contains(a.Registry().Render(), `datum_host_state{state="converged"} 1`) {
		t.Error("host state was not recorded")
	}
}

// A tick the agent cannot take the lock for is skipped, not queued, because a queue
// accumulates passes acting on progressively staler observations.
func TestATickIsSkippedWhenTheLockIsHeld(t *testing.T) {
	c := newClock()
	r := newRecorder()
	cfg := baseConfig(t)
	a := newAgent(t, cfg, r.pass, c)

	held, err := lock.Acquire(cfg.State, false)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	stop := run(t, a)
	defer stop()

	c.tick(t)
	// The agent should come back round to waiting without having run anything.
	select {
	case ch := <-c.waiting:
		if r.count() != 0 {
			t.Errorf("a pass ran while the lock was held")
		}
		held.Release()
		ch <- c.Now()
	case <-time.After(2 * time.Second):
		t.Fatal("the agent did not return to waiting")
	}
	waitFor(t, func() bool { return r.count() == 1 })
}

// An upstream failure backs the schedule off, because the cost lands on a remote every
// host shares.
func TestAnUpstreamFailureBacksOff(t *testing.T) {
	c := newClock()
	r := newRecorder()
	r.set(report.Report{}, schedule.Upstream, errors.New("fetch failed"))
	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	waitFor(t, func() bool { return a.Schedule().Backoff() == 2 })
	c.tick(t)
	waitFor(t, func() bool { return a.Schedule().Backoff() == 4 })

	// One success returns the host to its normal cadence rather than working back down.
	r.set(converged(), schedule.None, nil)
	c.tick(t)
	waitFor(t, func() bool { return a.Schedule().Backoff() == 1 })
}

// A local failure keeps the interval. Accelerating or backing off because a resource
// broke would change a fleet's cadence precisely when something is already wrong.
func TestALocalFailureKeepsTheInterval(t *testing.T) {
	c := newClock()
	r := newRecorder()
	failed := converged()
	failed.Outcome = "failed"
	failed.HostState = "failed"
	r.set(failed, schedule.Local, nil)

	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	waitFor(t, func() bool {
		return strings.Contains(a.Registry().Render(), "datum_pass_consecutive_failures 1")
	})
	if got := a.Schedule().Backoff(); got != 1 {
		t.Errorf("backoff = %d after a local failure, want 1", got)
	}
}

func TestConsecutiveFailuresResetOnSuccess(t *testing.T) {
	c := newClock()
	r := newRecorder()
	failed := converged()
	failed.Outcome = "failed"
	r.set(failed, schedule.Local, nil)

	a := newAgent(t, baseConfig(t), r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	c.tick(t)
	waitFor(t, func() bool {
		return strings.Contains(a.Registry().Render(), "datum_pass_consecutive_failures 2")
	})

	r.set(converged(), schedule.None, nil)
	c.tick(t)
	waitFor(t, func() bool {
		return strings.Contains(a.Registry().Render(), "datum_pass_consecutive_failures 0")
	})
}

// A pass that overruns its timeout is abandoned so it cannot hold the lock for ever
// and leave the host looking converged and quiet.
func TestAPassIsAbandonedWhenItOverrunsTheTimeout(t *testing.T) {
	c := newClock()
	r := newRecorder()
	r.block = make(chan struct{})
	r.inside = make(chan struct{}, 1)

	cfg := baseConfig(t)
	cfg.Reconciliation.Interval = config.Duration(time.Minute)
	cfg.Reconciliation.Timeout = config.Duration(50 * time.Millisecond)
	a := newAgent(t, cfg, r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	<-r.inside
	// The pass is blocked, so only the timeout can end it.
	waitFor(t, func() bool {
		return strings.Contains(a.Registry().Render(), "datum_pass_consecutive_failures 1")
	})
	close(r.block)
}

// Stopping has to release the lock, or a restart would find it held and skip.
func TestStoppingReleasesTheLock(t *testing.T) {
	c := newClock()
	r := newRecorder()
	cfg := baseConfig(t)
	a := newAgent(t, cfg, r.pass, c)

	stop := run(t, a)
	c.tick(t)
	waitFor(t, func() bool { return r.count() == 1 })
	stop()

	held, err := lock.Acquire(cfg.State, false)
	if err != nil {
		t.Fatalf("the lock was not released: %v", err)
	}
	held.Release()
}

// An in-flight pass must not keep the process alive past a stop request.
func TestStoppingInterruptsAnInFlightPass(t *testing.T) {
	c := newClock()
	r := newRecorder()
	r.block = make(chan struct{})
	r.inside = make(chan struct{}, 1)
	defer close(r.block)

	a := newAgent(t, baseConfig(t), r.pass, c)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	c.tick(t)
	<-r.inside
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the agent did not stop while a pass was running")
	}
}

// A restarted agent should serve the state the host is actually in rather than zeros.
func TestAnAgentSeedsItselfFromTheLastReportOnDisk(t *testing.T) {
	cfg := baseConfig(t)
	if _, err := report.Write(cfg.State, converged()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	c := newClock()
	a := newAgent(t, cfg, newRecorder().pass, c)
	if !strings.Contains(a.Registry().Render(), `datum_host_state{state="converged"} 1`) {
		t.Error("a restarted agent does not serve the last pass")
	}
}

func TestTheMetricsEndpointServesWhileTheAgentWaits(t *testing.T) {
	cfg := baseConfig(t)
	cfg.Metrics.Listen = "127.0.0.1:0"

	c := newClock()
	r := newRecorder()
	a := newAgent(t, cfg, r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	waitFor(t, func() bool { return r.count() == 1 && a.Address() != "" })

	resp, err := http.Get("http://" + a.Address() + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestTheTextfileIsWrittenEachPass(t *testing.T) {
	cfg := baseConfig(t)
	path := cfg.State + "/datum.prom"
	cfg.Metrics.Textfile = path

	c := newClock()
	r := newRecorder()
	a := newAgent(t, cfg, r.pass, c)
	stop := run(t, a)
	defer stop()

	c.tick(t)
	waitFor(t, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), `datum_passes_total{outcome="converged"} 1`)
	})
}

// The state directory holds plans and the recorded revision, so the agent refuses a
// directory anybody else can read.
func TestAnAgentRefusesAWorldReadableStateDirectory(t *testing.T) {
	cfg := baseConfig(t)
	if err := os.Chmod(cfg.State, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	_, err := agent.New(agent.Options{Config: cfg, Pass: newRecorder().pass})
	if err == nil {
		t.Fatal("a world-readable state directory was accepted")
	}
}

func TestAnAgentNeedsSomethingToRun(t *testing.T) {
	if _, err := agent.New(agent.Options{Config: baseConfig(t)}); err == nil {
		t.Fatal("an agent with no pass was accepted")
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition was never met")
}

// syncBuffer is a writer the agent's goroutine and the test can both touch.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// trust.require defaults to signed-commit so that a fleet which has not thought about
// signing gets an error rather than silently applying unverified desired state. Nothing
// verifies yet, so honouring that means refusing to start.
func TestAnAgentRefusesToRunWhenItCannotHonourTrustRequire(t *testing.T) {
	for _, require := range []config.Require{config.RequireSignedCommit, config.RequireSignedTag} {
		cfg := baseConfig(t)
		cfg.Trust.Require = require
		cfg.Trust.TagPattern = "release-*"

		_, err := agent.New(agent.Options{Config: cfg, Pass: newRecorder().pass})
		if err == nil {
			t.Fatalf("trust.require %s started an agent that verifies nothing", require)
		}
		if !strings.Contains(err.Error(), "trust.require: none") {
			t.Errorf("the error does not say what to do: %v", err)
		}
	}
}
