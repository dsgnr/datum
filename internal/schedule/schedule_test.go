// SPDX-License-Identifier: Apache-2.0

package schedule_test

import (
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/schedule"
)

const interval = 30 * time.Minute

func at(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parsing %q: %v", text, err)
	}
	return parsed
}

// The whole point of deriving the offset from the host name is that it does not move,
// so a restart has to produce the same schedule.
func TestTheOffsetIsStableForAHost(t *testing.T) {
	first := schedule.New("web-001", interval, interval)
	second := schedule.New("web-001", interval, interval)
	if first.Offset() != second.Offset() {
		t.Fatalf("offset moved between %s and %s", first.Offset(), second.Offset())
	}
}

func TestDifferentHostsGetDifferentOffsets(t *testing.T) {
	seen := map[time.Duration]string{}
	collisions := 0
	for _, host := range []string{
		"web-001", "web-002", "web-003", "db-001", "db-002",
		"cache-001", "cache-002", "edge-001", "edge-002", "edge-003",
	} {
		offset := schedule.New(host, interval, interval).Offset()
		if offset < 0 || offset >= interval {
			t.Errorf("%s offset %s is outside the interval", host, offset)
		}
		if other, ok := seen[offset]; ok {
			t.Logf("%s and %s share offset %s", host, other, offset)
			collisions++
		}
		seen[offset] = host
	}
	// Spreading is the purpose, so a hash putting most of a fleet on one minute
	// would defeat it.
	if collisions > 1 {
		t.Errorf("%d hosts collided, which is not spreading anything", collisions)
	}
}

func TestSplayZeroPutsEveryHostOnTheBoundary(t *testing.T) {
	for _, host := range []string{"web-001", "db-001", "edge-009"} {
		s := schedule.New(host, interval, 0)
		if s.Offset() != 0 {
			t.Errorf("%s has offset %s with splay 0", host, s.Offset())
		}
		next := s.Next(at(t, "2026-02-08T09:05:00Z"))
		if !next.Equal(at(t, "2026-02-08T09:30:00Z")) {
			t.Errorf("%s next = %s, want the interval boundary", host, next)
		}
	}
}

// A host keeps its own interval between passes whatever its offset is.
func TestConsecutivePassesAreOneIntervalApart(t *testing.T) {
	s := schedule.New("web-001", interval, interval)
	now := at(t, "2026-02-08T00:00:00Z")
	first := s.Next(now)
	second := s.Next(first)
	if gap := second.Sub(first); gap != interval {
		t.Errorf("gap between passes = %s, want %s", gap, interval)
	}
}

func TestNextIsAlwaysInTheFuture(t *testing.T) {
	s := schedule.New("web-001", interval, interval)
	now := at(t, "2026-02-08T09:00:00Z")
	// Landing exactly on a tick must still move forward, or a loop would fire twice.
	tick := s.Next(now)
	if !s.Next(tick).After(tick) {
		t.Error("Next returned a time that is not after the tick it was given")
	}
	if !s.Next(now).After(now) {
		t.Error("Next returned a time that is not after now")
	}
}

// A pass that overran means the tick it would have fired is behind us, and the due tick
// is skipped rather than fired immediately or queued.
func TestAnOverrunTickIsSkippedRatherThanQueued(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	start := at(t, "2026-02-08T09:00:00Z")

	// A pass starting on the boundary and taking 35 minutes crosses the 09:30 tick.
	finished := start.Add(35 * time.Minute)
	next := s.Next(finished)
	if !next.Equal(at(t, "2026-02-08T10:00:00Z")) {
		t.Fatalf("next = %s, want 10:00 with 09:30 skipped", next)
	}
	// And the wait is a real wait, not zero, so the loop does not spin.
	if wait := s.Wait(finished); wait <= 0 {
		t.Errorf("wait = %s", wait)
	}
}

func TestUpstreamFailuresBackOffToTheDocumentedLadder(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	want := []int{2, 4, 8, 8, 8}
	if got := s.Backoff(); got != 1 {
		t.Fatalf("a healthy host has multiplier %d", got)
	}
	for i, multiplier := range want {
		s.Record(schedule.Upstream)
		if got := s.Backoff(); got != multiplier {
			t.Errorf("after %d failures multiplier = %d, want %d", i+1, got, multiplier)
		}
	}
}

func TestBackoffLengthensTheWait(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	now := at(t, "2026-02-08T09:00:01Z")
	base := s.Wait(now)

	s.Record(schedule.Upstream)
	s.Record(schedule.Upstream)
	s.Record(schedule.Upstream)
	// Three failures is the four-hour ceiling with a thirty-minute interval.
	if s.Backoff() != 8 {
		t.Fatalf("multiplier = %d", s.Backoff())
	}
	if backedOff := s.Wait(now); backedOff <= base {
		t.Errorf("backed-off wait %s is not longer than %s", backedOff, base)
	}
}

func TestOneSuccessResetsTheMultiplier(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	for range 4 {
		s.Record(schedule.Upstream)
	}
	if s.Backoff() != 8 {
		t.Fatalf("multiplier = %d", s.Backoff())
	}
	s.Record(schedule.None)
	if got := s.Backoff(); got != 1 {
		t.Errorf("multiplier after a success = %d, want 1", got)
	}
	if got := s.UpstreamFailures(); got != 0 {
		t.Errorf("upstream failures after a success = %d", got)
	}
}

// Shortening or lengthening the interval because a resource failed would mean a fleet
// changing its cadence precisely when something is already wrong.
func TestALocalFailureKeepsTheInterval(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	for range 3 {
		s.Record(schedule.Local)
	}
	if got := s.Backoff(); got != 1 {
		t.Errorf("multiplier after local failures = %d, want 1", got)
	}
}

// A host unable to fetch for a long time should sit at the cap instead of counting
// upward for ever.
func TestTheFailureCountStopsAtTheCap(t *testing.T) {
	s := schedule.New("web-001", interval, 0)
	for range 500 {
		s.Record(schedule.Upstream)
	}
	if got := s.Backoff(); got != 8 {
		t.Errorf("multiplier = %d, want the cap", got)
	}
	if got := s.UpstreamFailures(); got > 5 {
		t.Errorf("failure count ran to %d, which serves no purpose past the cap", got)
	}
}

func TestAZeroIntervalDoesNotDivideByZero(t *testing.T) {
	s := schedule.New("web-001", 0, 0)
	if wait := s.Wait(time.Now()); wait <= 0 {
		t.Errorf("wait = %s", wait)
	}
}
