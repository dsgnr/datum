# Time and ordering

Several security controls want to reason about time. A signature has an issue and an expiry, a
cached revision might grow stale, and a manifest should not be replayable indefinitely. Time is
also the input a managed host is least able to trust, because its clock may be wrong, unset, or
attacker-influenced.

This page states how the design reasons about time without depending on it more than it must.

## Wall clocks are not trustworthy on hosts

A freshly provisioned machine may have no correct time until it reaches an NTP source, and the
machines Datum most wants to manage, at [edge sites and on intermittent
links](../reconciliation/last-known-good.md#offline-and-intermittent-hosts), are exactly the ones
whose clocks drift or reset. An attacker with local influence can also move a clock on purpose.

A control that depends only on wall-clock time is therefore a control an attacker can defeat by
setting the clock back, and one that fails for an honest host whose clock is simply wrong. Neither
is acceptable on its own.

## Ordering does not need a clock

Most of what these controls actually need is ordering, not time, and ordering can be established
without a clock.

[Downgrade protection](repository-trust.md#verifying-that-a-revision-is-current) uses Git ancestry,
so a revision is acceptable only if it descends from the last one applied. That is a fact about
history, independent of any clock, and an attacker cannot make an old commit descend from a newer
one.

That answers "is this newer than what I have" using something monotonic rather than wall-clock time,
which is why the design reaches for it first.

## Where time is still needed

Ordering cannot express everything. "This signature was valid when issued but should not be honoured
forever" is a statement about elapsed time, and no counter captures it.

Signatures may therefore carry timestamps.

```text
issuedAt   2026-02-08T09:00:00Z
expiresAt  2026-02-15T09:00:00Z
```

!!! note "Proposed behaviour"

    Timestamps are proposed and unimplemented. Where they are used, they bound how long something
    stays valid, and they are always paired with a monotonic check instead of relied on alone.

The pairing is the point. A signature that expires by wall-clock time bounds how long a captured
artefact stays useful, and the [descendant check](repository-trust.md#verifying-that-a-revision-is-current)
prevents a replay within that window. Expiry limits the window, and ordering closes it.

## Consequences for offline hosts

An expiry measured in wall-clock time is awkward for a host that has been offline long enough to
matter, because the host cannot fetch anything newer and the material it holds eventually expires.

That tension is real and is left to the [cached-revision expiry
question](../reconciliation/last-known-good.md#how-long-a-cached-revision-stays-usable), because how
long stale desired state stays usable and how long a signature stays valid are the same decision
seen from two directions. A factory floor that must keep running favours long or absent expiry, and
a laptop that might be stolen favours short expiry, and the design does not force one answer.

!!! note "Open question"

    Whether an agent that cannot establish trustworthy time refuses time-dependent checks, falls
    back to ordering alone, or continues on its last-known-good until it regains a clock, is
    undecided. The safe default is probably to keep enforcing the last known good, which needs no
    clock, and to decline only the operations that genuinely require one, such as accepting a new
    manifest whose signature can only be validated within a time window.
