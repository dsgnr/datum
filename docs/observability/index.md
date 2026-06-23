# Observability

A fleet running Datum raises questions the reconciliation model does not answer on its own, such as
which hosts are up to date, which have drifted, which have stopped reconciling entirely and which
are waiting to reboot. The signals Datum emits are specified below, so those questions have answers
that do not involve logging into machines.

!!! note "Proposed design"

    None of this is implemented. The signals and their shapes are proposed, and they are specified
    at this level of detail because the components have to preserve the information the signals
    carry, which is cheap to design now and awkward to add later.

## The four questions

Observability exists to answer four things, and they need different signals.

Is this host reconciling at all?
:   The hardest one, because the failure is an absence. A host whose agent has stopped emits no
    error, and nothing arrives to alert on.

Is this host up to date?
:   Whether the revision it last applied is the newest one available.

Is this host in the state the repository asks for?
:   Its [host state](../concepts/state.md#host-state-across-passes), which distinguishes converged
    from drifted, failed, degraded and awaiting a reboot.

What happened, and when?
:   The detail behind a state, which is what somebody reads after an alert fires.

## The signals

Three kinds, with different jobs and different costs.

| Signal | Answers | Shape |
| ------ | ------- | ----- |
| [Metrics](metrics.md) | Aggregate questions across a fleet | Numeric, low cardinality, scraped from the agent |
| Logs | What happened on one pass on one host | Structured events, one per pass and per action |
| [Status](../reference/status.md) | The current condition of one host | A local report, read on demand |

Status is not a third copy of the same data. It is the authoritative local record a host keeps of
its last pass, and metrics are a numeric projection of it designed for aggregation. A monitoring
system consumes metrics, and a person investigating one host reads status.

## Logs

A pass emits one structured event for the pass and one for each action it took. Structured, not free
text, because the fields are what matters and parsing prose is how log pipelines become fragile.

```json
{
  "pass": "01JQ8Z3M0000000000000000",
  "host": "web-001",
  "revision": "8b91f20",
  "manifest": "sha256:3f2a9c4e",
  "mode": "enforce",
  "outcome": "changed",
  "duration_ms": 183,
  "resources": { "total": 14, "converged": 14 }
}
```

```json
{
  "pass": "01JQ8Z3M0000000000000000",
  "resource": "File[nginx-config]",
  "provider": "posix-file",
  "action": "update",
  "state": "converged",
  "fields": ["mode", "content"],
  "duration_ms": 12
}
```

A pass identifier ties the two together, so every action from one pass can be found from the pass
event and the other way round. Without it, correlating actions on a host that reconciles every few
minutes means reasoning about timestamps.

The action event names the fields that changed and does not carry their values. This follows the
[report redaction rule](../security/provider-safety.md#reports-and-content-disclosure), where a log
line recording that `content` changed is useful and one recording what it changed to puts file
content into a log pipeline, which is the one place it is hardest to get back out of.

## Nothing aggregates

Nothing [collects anything](../architecture/deployment-models.md). Each host emits
its own signals and there is no Datum component that sees more than one machine.

That is a real limitation, not a temporary one. Answering a fleet-wide question means an existing
monitoring system scraping every host, and Datum's job is to expose signals in a shape that such a
system can already consume instead of becoming one.

## Metrics are claims, not attestation

Every signal here is produced by a host about itself. On a host compromised at root, all of them
say whatever that host says.

A dashboard of green hosts is therefore a set of claims by those hosts and not independent evidence
about them, which is the same point the [threat
model](../security/threat-model.md#an-attacker-with-root-on-one-managed-host) makes about status.
The distinction matters most in the case observability is otherwise best at catching, because a
host that has been compromised and is reporting healthy is indistinguishable from a healthy host
using these signals alone.

What observability does catch is a host that stops reporting, which is why [detecting a host that
stops reconciling](alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch) is the alert
that matters most and the one most easily left out.
