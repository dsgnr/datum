# Alerting

Which conditions should page somebody, and which are normal operation. A reconciliation system
produces a large amount of routine activity that resembles a fault, so the distinction is set out
explicitly.

!!! note "Proposed design"

    The alerts below are recommendations, not part of Datum. They are documented because the
    [metrics](metrics.md) exist to support them, and a metric nobody can write a useful alert
    against is a metric that should not exist.

## Staleness is the alert that matters most

A host that has stopped reconciling is the failure most likely to go unnoticed, because it produces
no error and emits nothing. Every other alert on this page fires because something reported a
problem, and this one fires because nothing reported anything.

```text
time() - datum_pass_last_success_timestamp_seconds > 3600
```

The threshold is a multiple of the reconciliation interval rather than an absolute figure. Alerting
at three or four missed passes tolerates a transient failure and a slow pass while still catching a
host that has genuinely stopped, and alerting at one missed pass produces noise on any fleet large
enough for coincidences.

This works when the agent has stopped, when its host is unreachable, and when the agent was removed,
because all three stop the timestamp advancing. It relies on metrics being [absolute
timestamps](metrics.md#emit-absolute-timestamps-never-elapsed-time) and not elapsed times, which is
why that detail is worth being careful about.

A host that is down entirely stops being scraped, so this alert needs pairing with whatever already
detects an unreachable machine. Datum's staleness alert catches the case that is otherwise invisible,
which is a host that is up and answering scrapes while its agent does nothing.

## Alerts worth having

| Condition | Query | Why |
| --------- | ----- | --- |
| Not reconciling | `time() - datum_pass_last_success_timestamp_seconds > 3600` | The host has stopped converging. |
| Failing | `datum_host_state{state="failed"} == 1` | A resource failed to apply or verify. |
| Coverage gap | `datum_host_state{state="degraded"} == 1` | Part of the manifest cannot be reconciled here. |
| Behind the fleet | `max(datum_revision_timestamp_seconds) - datum_revision_timestamp_seconds > 86400` | A host has not picked up changes others have. |
| Stuck on a bad revision | `datum_revision_timestamp_seconds != datum_last_known_good_timestamp_seconds` | The newest revision failed to resolve, and the host is on [last known good](../reconciliation/last-known-good.md). |
| Awaiting reboot too long | `datum_reboot_required == 1` held for longer than the reboot policy allows | A change has been applied and is not in effect. |

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
