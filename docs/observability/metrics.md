# Metrics

Metrics answer aggregate questions across a fleet. They are not a per-resource export, for
cardinality reasons covered below, and the per-resource detail lives in logs and
[status](../reference/status.md) instead.

!!! note "Proposed design"

    The metric names and shapes are proposed. They follow Prometheus naming conventions because that
    is what most fleets already collect, and nothing about the design depends on Prometheus
    specifically.

## Exposure

The agent serves metrics over HTTP, in the same way an exporter does.

```text
GET /metrics
```

```yaml title="/etc/datum/agent.yaml"
metrics:
  listen: 127.0.0.1:10056
```

!!! note "Proposed behaviour"

    10056 is taken from the second exporter range in the Prometheus port allocation list, where 9100
    to 9999 is fully allocated. The entry has to be added to that list so another exporter does not
    claim the same number.

## Why a port rather than a file

The alternative is writing a file for a textfile collector to pick up, which avoids a listener
entirely. It also has a failure mode that defeats the most important thing metrics are for.

When an agent dies, the textfile stops being rewritten and the collector keeps serving the last file
it found. Every metric in it continues to be scraped successfully, reporting the values the agent held
at the moment it stopped, so a dead agent looks like a healthy one until somebody reasons about
timestamps.

An HTTP endpoint fails honestly. A dead agent is not listening, the scrape fails, and `up` goes to
zero straight away.

```text
up{job="datum"} == 0
```

That is a direct, immediate signal that the agent is gone, and it needs no threshold, no timestamp
arithmetic and no assumption about the reconciliation interval. Serving metrics from the process whose
health is in question is what makes the absence of that process detectable.

The two failures are distinct and both matter. `up == 0` means the agent is not running, and a
[stale success timestamp](#emit-absolute-timestamps-never-elapsed-time) means the agent is running and
not converging. An endpoint gives the first, and the metric design gives the second.

## Serving does not read the host

A scrape returns values recorded by the last pass. It does not trigger a pass, read the host, or touch
the repository.

That matters because a metrics endpoint is scraped far more often than a host is reconciled, and an
endpoint that observed the host on demand would turn a monitoring system into a source of load on
every managed machine, with a scrape storm becoming a fleet-wide read of every managed path.

It also keeps the endpoint honest about what it is. Metrics describe the last pass, and the
freshness of that description is itself reported through the pass timestamps and not implied by the
scrape having succeeded.

## Binding and exposure

The default binds to loopback.

```text
metrics:
  listen: 127.0.0.1:10056      # default
```

An exporter normally listens on all interfaces by default. The listener here runs inside a process
that holds repository credentials and runs as root, on hosts that otherwise need no inbound access,
so remote exposure is configured rather than assumed.

Exposing it to a remote scraper is one line.

```text
metrics:
  listen: 0.0.0.0:10056
```

Fleets already running a local collector, whether that is `node_exporter` with a scrape config, an
OpenTelemetry collector or a Prometheus agent on the same host, need nothing beyond the default,
because loopback is reachable from the same machine.

!!! note "Security consideration"

    The endpoint discloses the revision and manifest digest a host is running, its state, and its
    resource counts. That is reconnaissance value, not secret material, and it reveals which hosts
    are behind on configuration, which is exactly the set an attacker would find interesting.

    It carries no file content, no field values and no credentials, which follows from the same
    [redaction rule](../security/provider-safety.md#reports-and-content-disclosure) that applies to
    reports. Nothing reachable through `/metrics` is absent from what the metric catalogue below
    describes.

!!! note "Open question"

    Whether the endpoint supports TLS and authentication is undecided. Exporters conventionally have
    neither and rely on network controls, and an agent that already manages certificates for its own
    source might reasonably serve them. Leaving it unauthenticated on loopback is safe, and exposing
    it on all interfaces without either is a decision a fleet should make consciously.

## Writing a file as well

Some fleets prefer the file, either because a textfile collector is already the established pattern or
because no listener is acceptable on a particular host.

```text
metrics:
  textfile: /var/lib/node_exporter/textfile/datum.prom
```

Both mechanisms can be configured at once, and both carry identical metrics. Where only the file is
used, the `up == 0` signal is unavailable and staleness detection through the pass timestamps is the
only way a stopped agent is noticed, which makes the [absolute timestamp
rule](#emit-absolute-timestamps-never-elapsed-time) load-bearing and not merely correct.

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

A metric like `datum_seconds_since_last_pass` seems natural and is a trap, because an elapsed-time
gauge is only correct while something keeps recomputing it.

An agent that is running but no longer completing passes keeps serving metrics, so the endpoint stays
up and the gauge keeps reporting whatever it last calculated. The same applies more severely to a
[textfile](#writing-a-file-as-well), where a stopped agent leaves a frozen file that continues to
scrape successfully.

An absolute timestamp does not have that failure, because the age is computed at query time from a
value that does not need updating to stay truthful.

```text
time() - datum_pass_last_success_timestamp_seconds
```

That expression rises on its own whether the agent is failing, wedged, stopped, or removed. Every
time-related metric here is therefore an absolute timestamp, and no metric reports an age.

The rule matters even with an HTTP endpoint. `up == 0` catches an agent that has exited, and an agent
that is alive and stuck is exactly the case `up` cannot see, which is the case these timestamps exist
for.

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
