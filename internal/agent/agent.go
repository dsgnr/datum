// SPDX-License-Identifier: Apache-2.0

// Package agent runs Datum as a long-lived service.
//
// The agent is resident and not a timer-driven one-shot, because the metrics endpoint
// has to live somewhere. A process that exits between passes cannot answer a scrape,
// and a failed scrape is the one signal that detects a dead agent without depending on
// a threshold.
//
// Between passes it serves metrics from what the last pass recorded and waits. There is
// no watching of the repository and no connection held open to anything.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/lock"
	"github.com/dsgnr/datum/internal/metrics"
	"github.com/dsgnr/datum/internal/report"
	"github.com/dsgnr/datum/internal/schedule"
	"github.com/dsgnr/datum/internal/statedir"
)

// Pass is one reconciliation, supplied by the caller so this package drives the
// schedule without depending on the command layer.
//
// The returned report is recorded and served. An error means the pass did not get far
// enough to produce one, and Upstream in that case backs the schedule off.
type Pass func(ctx context.Context) (report.Report, schedule.Failure, error)

// Options configure one agent.
type Options struct {
	Config config.Config
	Pass   Pass

	// Log is where the agent narrates. A service writes to the journal through stderr
	// rather than managing its own log file.
	Log io.Writer

	// Registry is what the endpoint serves. Supplied by the caller when the pass also
	// records into it, such as counting the revisions a trust control refused, and
	// created here otherwise.
	Registry *metrics.Registry

	// Now and After exist so the tests can drive the clock instead of sleeping
	// through a thirty-minute interval.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// Agent is the resident process.
type Agent struct {
	opts     Options
	schedule schedule.Schedule
	registry *metrics.Registry
	server   *metrics.Server

	// consecutiveFailures counts failed passes of either kind, which is the gauge a fleet
	// uses to tell one bad pass from forty. Nothing here acts on it.
	consecutiveFailures int
}

// New prepares an agent without starting anything.
func New(opts Options) (*Agent, error) {
	if opts.Pass == nil {
		return nil, errors.New("an agent needs something to run")
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.After == nil {
		opts.After = time.After
	}

	cfg := opts.Config

	// Checked before anything else. A readable state directory discloses plans, and a
	// writable one allows downgrade protection to be reset.
	if err := statedir.Ensure(cfg.State); err != nil {
		return nil, err
	}

	s := schedule.New(cfg.Host, time.Duration(cfg.Reconciliation.Interval), cfg.Reconciliation.SplayOr())
	registry := opts.Registry
	if registry == nil {
		registry = metrics.NewRegistry(cfg)
	}
	registry.SetOffset(s.Offset())

	// Seeded from the last pass on disk, so a restarted agent serves the state the host is
	// actually in, not zeros until its first pass.
	if latest, found, err := report.Latest(cfg.State); err == nil && found {
		registry.RecordPass(latest, 0)
	}

	return &Agent{opts: opts, schedule: s, registry: registry}, nil
}

// Registry is what the metrics endpoint serves.
func (a *Agent) Registry() *metrics.Registry { return a.registry }

// Schedule is the computed schedule, for reporting.
func (a *Agent) Schedule() schedule.Schedule { return a.schedule }

// Address is what the metrics listener bound, empty when it is off.
func (a *Agent) Address() string { return a.server.Address() }

// Run serves metrics and reconciles until the context is cancelled.
//
// There is no pass at startup. Five hundred machines booting together would otherwise
// all reconcile at once, and instead each waits for its own offset within the first
// interval.
func (a *Agent) Run(ctx context.Context) error {
	cfg := a.opts.Config

	if cfg.Metrics.Serving() {
		server, err := metrics.Listen(cfg.Metrics.Listen, a.registry)
		if err != nil {
			return err
		}
		a.server = server
		defer a.server.Close()
		a.logf("serving metrics on %s", a.server.Address())
	} else {
		a.logf("metrics listener disabled")
	}
	a.writeTextfile()

	a.logf("host %s, interval %s, offset %s, next pass %s",
		cfg.Host, cfg.Reconciliation.Interval, round(a.schedule.Offset()),
		a.schedule.Next(a.opts.Now()).Format(time.RFC3339))

	for {
		wait := a.schedule.Wait(a.opts.Now())
		select {
		case <-ctx.Done():
			// A clean stop, not an error. Whatever asked the agent to stop knows why, and a
			// service exiting non-zero on a normal shutdown gets reported as a crash.
			a.logf("stopping")
			return nil
		case <-a.opts.After(wait):
		}
		a.runOnce(ctx)
	}
}

// runOnce takes the lock, runs a pass, and records the result.
func (a *Agent) runOnce(ctx context.Context) {
	cfg := a.opts.Config

	// The scheduled pass never waits. Something else holding the lock means a pass is
	// already running or an operator is running one by hand, and either way this tick is
	// skipped instead of queued behind it.
	held, err := lock.Acquire(cfg.State, false)
	if err != nil {
		if errors.Is(err, lock.ErrHeld) {
			a.logf("skipping this tick, %v", err)
			return
		}
		a.logf("could not take the pass lock: %v", err)
		a.recordFailure(schedule.Local, err)
		return
	}
	defer held.Release()

	startedAt := a.opts.Now()
	a.registry.RecordAttempt(startedAt)

	// The pass is bounded so that one hung action cannot hold the lock for ever and
	// leave the host looking converged and quiet.
	passCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Reconciliation.Timeout))
	defer cancel()

	rep, failure, err := a.opts.Pass(passCtx)
	switch {
	case err != nil && ctx.Err() != nil:
		// Asked to stop partway through. An abandoned pass is a partially applied
		// pass, which the model already accommodates, so nothing is reverted.
		a.logf("pass abandoned because the agent is stopping")
		return
	case err != nil && passCtx.Err() != nil:
		a.recordFailure(failure, fmt.Errorf("the pass did not finish within %s", cfg.Reconciliation.Timeout))
	case err != nil:
		a.recordFailure(failure, err)
	default:
		a.recordPass(rep, failure)
	}
	a.writeTextfile()
}

func (a *Agent) recordPass(rep report.Report, failure schedule.Failure) {
	if failure == schedule.None {
		a.consecutiveFailures = 0
	} else {
		a.consecutiveFailures++
	}
	a.schedule.Record(failure)
	a.registry.RecordPass(rep, a.consecutiveFailures)

	next := a.schedule.Next(a.opts.Now())
	a.logf("pass %s, %d of %d resources converged, next pass %s",
		rep.Outcome, rep.Counts.Converged, rep.Counts.Total, next.Format(time.RFC3339))
	if backoff := a.schedule.Backoff(); backoff > 1 {
		a.logf("backing off, the interval is %dx while the remote is unreachable", backoff)
	}
}

func (a *Agent) recordFailure(failure schedule.Failure, err error) {
	if failure == schedule.None {
		// A pass that produced no report failed, whatever it reported about itself.
		failure = schedule.Local
	}
	a.consecutiveFailures++
	a.schedule.Record(failure)
	a.registry.RecordFailure(err.Error(), a.consecutiveFailures)

	a.logf("pass failed: %v", err)
	a.logf("next pass %s", a.schedule.Next(a.opts.Now()).Format(time.RFC3339))
}

func (a *Agent) writeTextfile() {
	path := a.opts.Config.Metrics.Textfile
	if path == "" {
		return
	}
	if err := metrics.WriteTextfile(path, a.registry); err != nil {
		// Not fatal. A failed metrics write is logged and reconciliation continues.
		a.logf("could not write %s: %v", path, err)
	}
}

func (a *Agent) logf(format string, args ...any) {
	fmt.Fprintf(a.opts.Log, time.Now().UTC().Format(time.RFC3339)+" "+format+"\n", args...)
}

// round trims an offset to whole seconds, since a hash-derived duration is otherwise
// printed to the nanosecond.
func round(d time.Duration) time.Duration { return d.Round(time.Second) }
