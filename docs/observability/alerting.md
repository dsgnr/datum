# Alerting

Which conditions should page somebody, and which are normal operation. A reconciliation system
produces a large amount of routine activity that resembles a fault, so the distinction is set out
explicitly.

!!! note "Proposed design"

    The alerts below are recommendations, not part of Datum. They are documented because the
    [metrics](metrics.md) exist to support them, and a metric nobody can write a useful alert
    against is a metric that should not exist.

## A host that stops reconciling is the failure to catch

The other alerts here fire on a reported problem. This one fires on the absence of a report, which
makes it the one most easily omitted.

It has two distinct shapes, and a fleet needs both.

**The agent has exited.** The [endpoint](metrics.md#exposure) stops answering and the scrape fails.

```text
up{job="datum"} == 0
```

Immediate, needs no threshold, and needs no assumption about the reconciliation interval. This is
the main practical argument for serving metrics from the agent rather than writing a file, because a
file left behind by a dead agent keeps scraping successfully.

**The agent is alive and not converging.** The process is up, the endpoint answers, and passes are
failing or wedged.

```text
time() - datum_pass_last_success_timestamp_seconds > 7200
```

`up` does not cover this case, since the agent is running and answering scrapes. The threshold is a
multiple of the reconciliation interval, not an absolute figure. Two hours is four missed passes at
the default of thirty minutes, which tolerates a transient failure and a slow pass while still
catching a genuine stall.

This relies on metrics being [absolute
timestamps](metrics.md#emit-absolute-timestamps-never-elapsed-time) and not elapsed times, and it is
the only staleness signal available to a fleet using the textfile mechanism instead of the endpoint.

Both alerts need pairing with whatever already detects an unreachable machine, since a host that is
entirely down produces `up == 0` for every job on it and is not specifically a Datum problem.

## Alerts worth having

| Condition | Query | Why |
| --------- | ----- | --- |
| Agent gone | `up{job="datum"} == 0` | The agent is not running or the host is unreachable. |
| Not converging | `time() - datum_pass_last_success_timestamp_seconds > 7200` | The agent is running and passes are not succeeding. |
| Failing | `datum_host_state{state="failed"} == 1` | A resource failed to apply or verify. |
| Coverage gap | `datum_host_state{state="degraded"} == 1` | Part of the manifest cannot be reconciled here. |
| Behind the fleet | `scalar(max(datum_revision_applied_timestamp_seconds)) - datum_revision_applied_timestamp_seconds > 86400` | A host has not picked up changes others have. |
| Stuck on a bad revision | `datum_revision_attempted_timestamp_seconds != datum_revision_applied_timestamp_seconds` | The newest revision failed to resolve, and the host is on [last known good](../reconciliation/last-known-good.md). |
| Awaiting reboot too long | `datum_reboot_required == 1` held for longer than the reboot policy allows | A change has been applied and is not in effect. |
| Verification disabled | `datum_trust_require{mode="none"} == 1` | A host is applying desired state it has not verified. |
| Downgrade protection unarmed | `datum_trust_baseline_present == 0` | Provisioning skipped the [baseline revision](../lifecycle/enrolment.md#the-baseline-revision). |
| Revisions being refused | `increase(datum_revisions_refused_total[1h]) > 0` | A signature, tag or ancestry check is rejecting what the host fetched. |
| Resources being refused | `increase(datum_resources_refused_total[1h]) > 0` | A safety control is refusing to apply something a repository declared. |

The last four are different in kind from the ones above them, because they fire on a control instead
of a failure. A host refusing revisions is behaving as specified, and the reason to page on it is
that the host has stopped receiving changes, which has the same effect as an agent that has stopped
running.

The two `mode="none"` and `baseline_present == 0` conditions are better treated as an inventory
query than an alert on most fleets, because they are steady states, not events. Both indicate a
control that is switched off, so they are listed here even though they are steady states.

A commit that fails to resolve trips the bad-revision alert on every host the change matched. Group
the alert by revision rather than by host, or one bad commit produces one page per host.

## Alerts not worth having

**Individual resource changes.** A resource being updated is Datum working. Alerting on `changed`
passes means alerting on every deployment, and the signal is drowned within a day.

**A single failed pass.** Transient failures happen, a package manager lock being the common cause,
and the next pass retries. Alerting on a sustained failure rather than an instantaneous one is what
the staleness and failing conditions above already do.

**Drift itself, on an enforcing host.** Drift is [ordinary
input](../concepts/drift.md) that the next pass corrects. A host that drifts and converges is
behaving correctly, and the interesting case is a resource that drifts repeatedly, which is a
[different question](../development/open-questions.md) Datum does not yet answer.

**Drift on an observing host, by default.** A host in [`observe`
mode](../concepts/reconciliation-modes.md) is expected to report drift, since nothing is correcting
it. During adoption the useful signal is whether drift is growing, so alerting on its presence
produces a permanent alarm.

## What to put on a dashboard

The fleet view answers how many hosts are in each state, how far behind the slowest host is, and how
many hosts are on a last-known-good revision instead of the newest.

```text
hosts by state            converged 486  degraded 9  failed 3  awaiting-reboot 2
worst revision lag        4h 12m
hosts behind newest       11
passes failing            3
```

The per-host view is where somebody lands after an alert, and it needs the host's state, its
revision and last-known-good, the timestamps of its last attempt and last success, and its resource
counts by state. That is exactly the [status model](../reference/status.md), which is why status is
designed as a model and not as console output.

## What these alerts cannot detect

Every alert here fires on what a host reports about itself, so none of them detect a host that has
been compromised and is reporting healthy. A machine under an attacker's control can emit a perfect
set of converged metrics indefinitely.

Observability catches hosts that are broken, behind, or silent. It does not establish that a host is
in the state it claims, and the [threat
model](../security/threat-model.md#an-attacker-with-root-on-one-managed-host) is explicit that
reporting is not attestation. Closing that would need something the host cannot forge, which is a
different problem from monitoring and is recorded as an open question rather than solved here.
