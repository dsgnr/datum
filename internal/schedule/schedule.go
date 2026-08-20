// SPDX-License-Identifier: Apache-2.0

// Package schedule decides when the next pass runs.
//
// Each host's passes are offset within the interval by an amount derived from its name,
// so hosts sharing an interval do not reconcile simultaneously. A host failing against
// the Git remote backs off exponentially, and a host failing locally keeps its
// interval.
package schedule

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// Failure is what went wrong last, which is what decides the delay, not the outcome
// alone. Only an upstream failure backs off.
type Failure int

const (
	// None is a pass that got far enough to reconcile, whatever it then found. A
	// host with a broken resource is not a host that should stop checking.
	None Failure = iota
	// Upstream is a fetch, a signature or a revision that would not resolve. The
	// cost lands on something every host shares.
	Upstream
	// Local is an action that failed or verification that did not confirm. The next
	// fetch is a cheap no-op and the fix is a commit to pick up promptly.
	Local
)

func (f Failure) String() string {
	switch f {
	case Upstream:
		return "upstream"
	case Local:
		return "local"
	default:
		return "none"
	}
}

// MaxBackoff is the ceiling on the interval multiplier. Unbounded back-off produces
// a host that has effectively stopped checking, and one four hours behind can be
// recovered while one that next tries in six days cannot.
const MaxBackoff = 8

// Schedule computes pass times for one host.
type Schedule struct {
	interval time.Duration
	offset   time.Duration

	// upstreamFailures counts consecutive upstream failures, which is what the
	// multiplier is derived from. One success resets it completely.
	upstreamFailures int
}

// New returns the schedule for a host.
//
// The offset comes from the host name, not from a random number, so a host reconciles
// at the same minutes past the hour for its whole life. An offset chosen at startup
// would move on every restart, which makes a pass something to guess at instead of wait
// for.
func New(host string, interval, splay time.Duration) Schedule {
	s := Schedule{interval: interval}
	if interval <= 0 {
		s.interval = time.Minute
	}
	if splay > 0 {
		s.offset = time.Duration(hash(host) % uint64(splay))
	}
	return s
}

// Offset is the host's position within the interval, which `datum config check`
// prints because nobody can compute it by hand.
func (s Schedule) Offset() time.Duration { return s.offset }

// Interval is the configured interval, without any back-off applied.
func (s Schedule) Interval() time.Duration { return s.interval }

// UpstreamFailures is the current consecutive count, for the metric.
func (s Schedule) UpstreamFailures() int { return s.upstreamFailures }

// Backoff is the multiplier currently applied to the interval. Three consecutive
// upstream failures reach the cap, which is a four-hour ceiling at the default
// interval.
func (s Schedule) Backoff() int {
	if s.upstreamFailures == 0 {
		return 1
	}
	if s.upstreamFailures >= 3 {
		return MaxBackoff
	}
	return 1 << s.upstreamFailures
}

// Record takes the result of a pass and adjusts the delay that follows it.
func (s *Schedule) Record(failure Failure) {
	switch failure {
	case Upstream:
		// Capped here as well as in Backoff, so the counter cannot grow without
		// bound on a host that has been unable to fetch for a month.
		if s.Backoff() < MaxBackoff {
			s.upstreamFailures++
		}
	default:
		// A local failure is not a reason to be cautious about the remote, so it
		// resets the multiplier in the same way a success does.
		s.upstreamFailures = 0
	}
}

// Next returns the time of the pass after now.
//
// Ticks are absolute, offset minutes past each interval boundary, so a pass that
// overran does not drag the schedule behind it. The tick that fell due while a pass was
// running is skipped instead of queued, which falls out of always returning a time
// after now.
func (s Schedule) Next(now time.Time) time.Time {
	interval := s.interval * time.Duration(s.Backoff())

	// Measured from the epoch so that every host with the same interval agrees on
	// where the boundaries are, and only the offset separates them.
	elapsed := now.UTC().Sub(time.Unix(0, 0).UTC())
	boundary := elapsed - elapsed%interval
	next := time.Unix(0, 0).UTC().Add(boundary + s.offset%interval)
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

// Wait is how long to sleep before the next pass.
func (s Schedule) Wait(now time.Time) time.Duration {
	return s.Next(now).Sub(now)
}

// hash is only used to spread hosts, so what matters is that it is stable across
// restarts and versions, not that it is fast or unpredictable.
func hash(host string) uint64 {
	sum := sha256.Sum256([]byte(host))
	return binary.BigEndian.Uint64(sum[:8])
}
