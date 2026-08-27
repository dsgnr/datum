# Failure, back-off and timeouts

A pass can fail before it reaches the host or partway through changing it, and the two are handled
differently. This page specifies what the agent does next in each case, and what bounds a pass that
never finishes on its own.

!!! note "Implementation status"

    The back-off ladder, the cap, the reset on success and both timeouts are implemented, and the
    consecutive-failure count is exposed as described. A failure is classed as upstream when
    resolution did not produce a manifest, which today means a bad repository, not a fetch or a
    signature, because neither of those exists yet.

## Two kinds of failure

| Failure | Example | Affects a shared resource | Timing of the next pass |
| ------- | ------- | ------------------------- | ----------------------- |
| Upstream | Fetch failed, signature refused, revision would not resolve | Yes, the Git remote | Exponential back-off |
| Local | An action failed, verification did not confirm | No | The normal interval |

The split follows from where the cost lands. A host that cannot fetch is failing against a remote
every other host shares, and five hundred hosts retrying a broken remote add load to a remote that
is already in trouble. A host whose `Service[nginx]` failed to start is failing locally, its next
fetch costs almost nothing, and the fix arrives as a commit it needs to pick up promptly.

Neither case reconciles sooner than the interval allows, which settles a question that was
previously open. A shortened interval on failure would make a fleet reconcile faster while something
is already wrong, and the latency it saves is on a machine that is already reporting a problem.

## Upstream failures back off

```text
attempt 1   interval
attempt 2   interval x 2
attempt 3   interval x 4
attempt 4   interval x 8      cap
attempt 5   interval x 8
```

With a thirty-minute interval that reaches a four-hour ceiling after three failures. The cap keeps
the interval recoverable. Unbounded back-off would reach a point where the next attempt is days
away, which leaves the host no longer checking in any practical sense.

One success resets the multiplier completely. A fleet recovering from a remote outage returns to its
normal cadence within one interval. The failures were a property of the remote, and once the remote
answers there is nothing left to back off from.

Back-off applies to the schedule alone. It does not apply to the lock or to a manual pass, so an
operator running `datum reconcile` on a backed-off host gets a pass immediately.

## Local failures keep the interval

A pass where resolution succeeded and something failed during apply or verification runs again at the
normal interval.

The next pass is the retry, and it begins with a fresh observation rather than resuming anything. A
transient package-manager lock clears within the interval. A broken resource fails again and is
reported again. Changing the timing alters neither outcome.

The repeated failure is [reported rather than alerted
on](../observability/alerting.md#alerts-not-worth-having). A single failed pass is expected, and the
condition that should wake somebody is a host whose last success is old, which the staleness alert
already covers.

## Consecutive failures are counted

```text
datum_pass_consecutive_failures    gauge
```

The count distinguishes a host that failed once from a host that has failed forty times. It is the
only signal that does so, since [a failed pass leaves no
history](../architecture/reconciliation-flow.md#what-is-written-down) beyond the current report.

The count is exposed for monitoring and the agent does not act on it. Giving up after some number of
failures would stop drift correction on a host with one broken resource, extending a narrow problem
to everything else the host declares. Escalating would require somewhere to escalate to.

!!! note "Open question"

    Whether the consecutive-failure count survives a restart of the agent is undecided. Keeping it
    would add a second piece of [cross-pass
    state](../architecture/reconciliation-flow.md#what-is-written-down) for a diagnostic value.
    Losing it on restart means a host that is restarted regularly never shows a high count, however
    badly it is failing.

## A pass is bounded

```yaml title="/etc/datum/agent.yaml"
reconciliation:
  interval: 30m
  timeout: 15m
  actionTimeout: 5m
```

Two timeouts, because a pass and an action fail in different ways.

`actionTimeout` bounds one provider action. A package manager waiting on a lock held by an unattended
upgrade is the common case, and five minutes is long enough for a large package to install and short
enough that one stuck action does not consume the whole pass.

`timeout` bounds the pass. A pass that exceeds it is abandoned, the lock is released, and the
outcome is recorded as `failed` with the phase it was in. Without it, a host with one hung action
holds its lock indefinitely and reports nothing new, which is indistinguishable from a host that is
converged and quiet. The [wedged-agent
alert](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch) depends
on the pass giving up.

An abandoned pass is a
[partially applied pass](../concepts/reconciliation.md#pass-outcomes), which the model already
accommodates. Actions that completed stay completed, nothing is reverted, and the next pass observes
whatever state the machine is actually in.

The default sets `timeout` shorter than `interval`, so a pass finishes or gives up before its
successor is due. That keeps the [skipped-tick
case](scheduling.md#a-pass-that-overruns-is-not-queued) exceptional.

## Other limits

Resolution has no timeout of its own. The fetch it depends on is bounded by [the repository fetch
limits](../security/repository-fetch.md#limits). A slow fetch of a very large repository and an
action that has hung are separate problems with separate limits.

Nothing limits how long a host may stay in a failing state. A host can fail every pass for a month,
and the agent keeps trying and keeps reporting for as long as it does.
