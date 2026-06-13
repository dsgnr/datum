# Metrics

Metrics answer aggregate questions across a fleet. They are not a per-resource export, for
cardinality reasons covered below, and the per-resource detail lives in logs and
[status](../reference/status.md) instead.

!!! note "Proposed design"

    The metric names and shapes are proposed. They follow Prometheus naming conventions because that
    is what most fleets already collect, and nothing about the design depends on Prometheus
    specifically.

## Exposure

An agent writes metrics to a file rather than listening on a port.

```text
/var/lib/node_exporter/textfile/datum.prom
```

This is the textfile-collector pattern, and it is chosen because it preserves a property the
architecture already claims. [Resolution on the host](../architecture/deployment-models.md) works on
a machine with no inbound network access, and an agent that opened a metrics port would give that up,
adding a listener running next to a process that holds repository credentials and runs as root.

The cost is a dependency on something else already scraping that directory, which on most fleets is
`node_exporter` and on some is nothing at all.

!!! note "Open question"

    Whether an agent should also be able to expose metrics over HTTP, for fleets with no textfile
    collector, is undecided. It is clearly convenient and it introduces a listener, which is a
    security decision rather than a convenience one.

## The metrics

**Pass timing and outcome.**

```text
datum_pass_last_attempt_timestamp_seconds   gauge
datum_pass_last_success_timestamp_seconds   gauge
datum_pass_duration_seconds                 gauge
datum_passes_total{outcome}                  counter
```

`outcome` is one of `converged`, `changed` or `failed`, matching the [pass
outcomes](../concepts/reconciliation.md#pass-outcomes).

**Host condition.**

```text
datum_host_state{state}     gauge, 1 for the current state and 0 for the others
datum_reboot_required       gauge, 0 or 1
datum_mode{mode}            gauge, 1 for the active mode
```

`state` takes the values from [host state](../concepts/state.md#host-state-across-passes), and `mode`
is `enforce` or `observe`.

**Resource counts.**

```text
datum_resources_total       gauge
datum_resources{state}      gauge
```

`state` takes the values from [resource state](../concepts/state.md#resource-state-within-a-pass),
which is what makes a `degraded` host's coverage gap visible as `datum_resources{state="skipped"}`
rather than only as a host state.

**Desired state identity.**

```text
datum_revision_timestamp_seconds            gauge
datum_revision_info{revision,manifest}      gauge, always 1
datum_last_known_good_timestamp_seconds     gauge
datum_last_known_good_info{revision}        gauge, always 1
```

## Emit absolute timestamps, never elapsed time

This is the single most important detail on the page, because getting it wrong produces a monitoring
system that reports healthy while the fleet is dead.

A metric like `datum_seconds_since_last_pass` seems natural and is a trap. When an agent stops, the
textfile stops being rewritten, and the collector keeps serving the last file it found. A frozen
elapsed-time gauge therefore keeps reporting the small value it held when the agent died, and every
alert built on it stays quiet forever.

An absolute timestamp does not have that failure. A frozen
`datum_pass_last_success_timestamp_seconds` holds the moment of the last success, and the age
computed from it grows on its own as the clock advances.

```text
time() - datum_pass_last_success_timestamp_seconds
```

That expression keeps rising whether the agent is failing, stopped, or removed, which is exactly the
behaviour staleness detection needs. Every time-related metric here is therefore an absolute
timestamp, and no metric reports an age.

## Answering "is this host up to date?"

The revision a host applied is a string, and strings do not compare usefully in a metrics query. The
commit timestamp of that revision does.

```text
max(datum_revision_timestamp_seconds) - datum_revision_timestamp_seconds
```

That gives, per host, how far behind the newest revision any host has reported it is, in seconds.
Nothing needs to know what the newest revision actually is, because the fleet's own maximum supplies
it, which matters because no component reads Git on the fleet's behalf.

!!! note "Important limitation"

    The maximum across reporting hosts is the newest revision any host has applied, not the newest
    revision in the repository. A fleet where every host is a week behind reports zero lag, because
    they are all equally behind. Detecting that needs something that knows the repository, and
    nothing does.

    A small job that reads Git and exports the newest revision's timestamp would close the gap, and
    it is not part of Datum.

## Cardinality

There are no per-resource metrics. A fleet of 500 hosts with 47 resources each would produce over
23,000 series before labels, and the questions those series would answer are ones logs and status
answer better.

Aggregate counts by state cost six series per host regardless of manifest size, which is what makes
the export scale with the fleet rather than with the configuration.

The revision info metrics are the one place where churn matters more than count. A new revision
creates a new series per host, and the previous one goes stale and ages out of an instant query
within the collector's staleness window. Active series stay at one per host per metric, and the
storage cost is proportional to how often the fleet deploys.

That churn is accepted, because the alternative of omitting the revision makes the up-to-date
question unanswerable, and knowing which revision a host is running is most of the value of
monitoring a GitOps system at all.

## Tracing

A pass has distinct phases with measurable durations, so a span per phase would help with a pass that
has become slow.

!!! note "Open question"

    Whether Datum emits traces is undecided and is lower priority than metrics and logs. A pass is a
    short, local, single-process operation, so the distributed part of distributed tracing does not
    apply, and `datum_pass_duration_seconds` plus per-action durations in logs answer most of what a
    trace would.
