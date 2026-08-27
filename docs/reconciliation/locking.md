# One pass at a time

Two concurrent passes against one host observe the same state, build overlapping plans and apply
them against each other. This page specifies the mechanism that prevents it, which [the concepts
section asserts](../concepts/reconciliation.md#one-pass-at-a-time) without saying how.

!!! note "Implementation status"

    The lock is implemented as described, including `--wait` and the scheduled pass that skips its
    tick rather than queueing behind whatever holds it.

## An operator racing the scheduled pass

An agent scheduling its own passes can avoid overlapping them, since one process controls when each
starts. That is not the dangerous case.

The dangerous case is an operator running `datum reconcile` during an incident while the
[scheduled pass](scheduling.md) is mid-apply. Those are two separate processes, both running as root,
both holding a plan built from an observation the other one is invalidating. An agent that serialises
only its own passes does nothing about this at all.

```text
00:07:00   scheduled pass begins, observes File[nginx-config] as mode 0644
00:07:04   operator runs datum reconcile, observes the same
00:07:05   scheduled pass writes the file and restarts nginx
00:07:06   manual pass writes the file again and restarts nginx again
```

The outcome there is survivable and the general shape is not. Two providers acting on one target
with interleaved observations can leave a file with content from one pass and a mode from another.
Both passes report success, so a later pass does not reliably correct it.

## An exclusive lock, taken by everything that reconciles

Every process that applies changes takes an exclusive advisory lock on one file before it observes,
and holds it until verification is finished.

```text
/var/lib/datum/pass.lock
```

The lock is taken before observation and not before apply, because the observation is what the plan
depends on. Taking it at apply time would allow two passes to observe concurrently and then apply in
sequence, each acting on a snapshot the other had invalidated.

The lock is released after verification rather than after apply, since verification reads the host
back and a concurrent writer between the two would cause a spurious verification failure.

| Command | Takes the lock |
| ------- | -------------- |
| `datum reconcile` | Yes, for the whole pass |
| `datum observe`, `datum diff`, `datum plan` | No |
| `datum render`, `datum explain`, `datum validate`, `datum affected` | No, they never read the host |
| `datum status` | No, it reads a report |

The read-only commands do not take it at all. An operator investigating a host during an incident
should never be blocked by a reconciliation in progress, and a plan read while a pass is running is
stale in the same way any plan is stale the moment it is printed. `datum plan` is therefore
available at any time, including while a pass is running.

## The second caller fails rather than waits

A process that cannot take the lock reports who holds it and exits.

```text
$ datum reconcile

error: a pass is already running on this host
  holder   pid 4182, pass 01JQ8Z3M0000000000000000, started 00:07:00
  waited   0s

exit 3
```

Failing immediately rather than blocking is the right default for an interactive command. An
operator who runs `datum reconcile` and sees nothing for four minutes has no way to tell a slow pass
from a hung one, whereas an error naming the holder answers the question and leaves the choice of
what to do next with the person making it.

`--wait` blocks until the lock is available, for scripts where queueing is what is wanted, and the
scheduled pass never waits. A scheduled pass that cannot take the lock skips its tick and records that
it did, for the same reason
[an overrunning pass is not queued](scheduling.md#a-pass-that-overruns-is-not-queued).

## A new exit code

| Code | Meaning |
| ---- | ------- |
| `3` | Could not acquire the pass lock. Another pass is running. |

A distinct code, not the generic `1`, means a script can tell "somebody else is reconciling" from
"reconciliation failed", and those call for opposite responses. The first should be retried and the
second should not, and a caller that cannot distinguish them has to either retry real failures or
give up on transient contention. The full set is in the [command
reference](../reference/cli.md#exit-codes).

## Why an advisory file lock

An exclusive `flock` on a file in the state directory needs no daemon, no shared memory and no cleanup
path, and the kernel releases it when the holding process dies for any reason.

That last property is what rules out the alternatives. A PID file written at the start of a pass has
to be removed at the end, so a process killed mid-pass leaves a stale file, and every implementation
then grows a heuristic for deciding whether the recorded process is still alive. A lock held by a
file descriptor is released when the process exits, so there is no stale lock to clear.

The lock file lives under `state`, which is
[already root-owned at mode `0700`](../security/provider-safety.md#reports-and-content-disclosure) and
[already refused as a target for Datum's own resources](../security/repository-trust.md#trust-anchors-are-never-managed-by-datum),
so no new path needs protecting.

!!! note "Important limitation"

    The lock coordinates Datum with Datum and nothing else. A package installed by hand, a
    configuration management tool run alongside Datum, or an administrator editing a managed file
    during a pass are all invisible to it, and the design has no way to exclude them. Such a change
    is detected afterwards, since [verification reads the host
    back](../resources/lifecycle.md#verify) and the next pass observes what was left behind.
    Repeated correction of the same resource is the signal that something else is managing it, which
    is [a question the design has not yet
    answered](../resources/conflicts.md#conflicts-datum-cannot-see).
