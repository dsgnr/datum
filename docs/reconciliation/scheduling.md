# When a pass runs

This page specifies what causes a pass to run. The rest of the documentation describes what one pass
does.

!!! note "Implementation status"

    The agent runs as a resident service, the offset is derived from the host name, and a tick it
    cannot take the lock for is skipped. What it cannot do yet is fetch, so the repository has to be
    on disk and `--repo` names it. [Running Datum on a host](../lifecycle/running.md) covers starting
    it.

    The interval, splay and timeouts take the default values below.

## The agent is a resident process

The agent runs as a long-lived service started by the init system, and it schedules its own passes.

```text
systemd starts datum.service
  |
  +-- serves /metrics continuously
  +-- runs a pass every interval
```

A timer-driven one-shot process was rejected. The [metrics
endpoint](../observability/metrics.md#binding-and-exposure) has to be served by a running process,
and a process that exits between passes cannot answer a scrape. `up{job="datum"} == 0` is then
available as [the alert for a dead
agent](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch), which
needs no threshold.

Two specified behaviours require a resident process. The [self-update
boundary](../architecture/self-management.md#what-the-design-commits-to-now) places replacement
between passes, and a host in [`awaiting-reboot`](../concepts/state.md#reboots) continues to report
that state across passes.

## The interval

```yaml title="/etc/datum/agent.yaml"
reconciliation:
  interval: 30m
```

Thirty minutes by default. Every key in that block is documented in the [agent configuration
reference](../reference/agent-config.md#reconciliation). At that interval drift is corrected within
an hour of appearing, and five hundred hosts fetch from one Git remote roughly seventeen times a
minute.

The appropriate value depends on the fleet. A fleet where drift is rare can reconcile hourly, and
one adopting Datum on machines several people have shell access to will want a shorter interval.

No other mechanism depends on a particular interval. The [staleness
alert](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch) is
expressed as a multiple of it, and everything else reads the state the last pass recorded.

## Passes are spread deterministically

Every host offsets its schedule by an amount derived from its own identity, so a fleet configured
with one interval does not reconcile in lockstep.

```text
offset = hash(host identity) mod interval
```

A host named `web-001` with a thirty-minute interval might land at 00:07 and 00:37, and `web-002` at
00:19 and 00:49, with both keeping thirty minutes between their own passes. Five hundred hosts
spread across thirty minutes reach the Git remote at roughly seventeen per minute instead of five
hundred at once.

The offset is derived from the host identity rather than from a random number. A random offset
chosen at startup changes a host's pass time on every service restart, and a fleet-wide restart
reshuffles the whole schedule. A derived offset keeps a host reconciling at the same minutes past
the hour for its lifetime.

The same mechanism covers a mass reboot. Five hundred machines booting together each wait for their
own offset within the first interval rather than running a pass immediately.

```yaml
reconciliation:
  interval: 30m
  splay: 30m        # defaults to the interval, 0 disables spreading
```

Setting `splay: 0` makes every host reconcile on the interval boundary, which suits a small fleet
where simultaneous passes are acceptable.

## A pass that overruns is not queued

If a pass is still running when the next one is due, the due pass is skipped and not queued behind
it.

A queue would accumulate passes on a host that takes longer than its interval to reconcile, and each
queued pass would act on an older observation than the one before it. Skipping preserves the
guarantee that a pass acts on a current observation, and the skipped ticks appear as a gap between
attempts.

```text
pass takes longer than the interval   the next tick is skipped
pass takes longer than the timeout    the pass is aborted, see failure handling
```

## Running a pass by hand

`datum reconcile` remains available and runs a pass immediately, outside the schedule.

This is the command to use during an incident, where waiting for the next interval is not
acceptable. It leaves the schedule unchanged, so the next scheduled pass runs when it would have,
and it [takes the same lock](locking.md) a scheduled pass takes.

## What the agent does between passes

The agent serves metrics from the state the last pass recorded and waits for the next tick. It
watches neither the repository nor the filesystem and holds no long-lived connections.

A push mechanism would require something central to push from, which the [deployment
model](../architecture/deployment-models.md) does not include. Watching the local filesystem would
make the agent react to every change on the machine rather than measure state on a schedule. Polling
bounds the latency of a correction by the interval.

!!! note "Open question"

    Whether an agent should support being triggered into a pass by a signal or a local socket is
    undecided. It would let a deployment pipeline reconcile immediately after a merge, and it would
    add a second inbound control surface to a process running as root.

## Stopping the agent

`SIGTERM` stops scheduling and ends a pass that is still running. `SIGINT` does the same, so that a
terminal behaves like a service manager.

A pass interrupted this way is a [partially applied
pass](../concepts/reconciliation.md#pass-outcomes), the same state a [pass that exceeds its
timeout](failure-handling.md#a-pass-is-bounded) leaves. Completed actions stay completed, nothing is
reverted, and the next pass observes the current state. Finishing the pass before exiting would make
a stop take as long as the timeout allows, after which a service manager sends `SIGKILL`.

The [pass lock](locking.md) is released in both cases, since the kernel drops it when the process
ends. A restarted agent does not find a lock left by its predecessor.

The agent exits `0` when it was asked to stop, so a deliberate restart is not reported as a crash.
