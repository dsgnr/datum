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

The listener is on by default and bound to loopback, and the full binding rules, how to expose it to a
remote scraper and how to turn it off are under [binding and exposure](#binding-and-exposure).

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

!!! note "Proposed behaviour"

    10056 is taken from the second exporter range in the Prometheus port allocation list, where 9100
    to 9999 is fully allocated. The entry has to be added to that list so another exporter does not
    claim the same number.

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

Setting `metrics.listen` to `none` disables the listener entirely, leaving the
[textfile output](#writing-a-file-as-well) as the only signal. That exists because loopback is not a
privilege boundary, so a host with untrusted local users may prefer no listener at all, and because a
fleet is entitled to decide that a root process should open no socket whatsoever.

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
datum_revision_attempted_timestamp_seconds       gauge
datum_revision_attempted_info{revision}          gauge, always 1
datum_revision_applied_timestamp_seconds         gauge
datum_revision_applied_info{revision,manifest}   gauge, always 1
datum_last_known_good_timestamp_seconds          gauge
datum_last_known_good_info{revision}             gauge, always 1
```

Three revisions, not one, because a single `revision` series has to mean either the newest revision
the host tried or the one it is actually running, and the two questions a fleet asks need different
answers. Attempted is what the host last fetched and tried to resolve. Applied is the revision whose
desired state it is reconciling, which stays behind attempted whenever resolution fails. Last known
good is the newest that resolved and validated cleanly, and it only ever advances.

On a healthy host all three are equal. Attempted running ahead of applied is the signal that a
revision failed to resolve, and it is the only combination that needs an alert, which is covered
under [alerting](alerting.md#alerts-worth-having).

## Security controls

Every trust control in the design works by refusing something, and a refusal that nothing outside the
host can see is indistinguishable from a control that was never switched on.

```text
datum_trust_require{mode}                     gauge, 1 for the configured mode
datum_trust_signer_info{keyid}                gauge, always 1, one series per trusted key
datum_trust_baseline_present                  gauge, 0 or 1
datum_revisions_refused_total{reason}         counter
datum_resources_refused_total{reason}         counter
```

`mode` takes the values of
[`trust.require`](../security/repository-trust.md#verifying-that-a-revision-is-genuine), which is
what makes an unverified fleet visible. A fleet running `require: none` is a fleet that decided not
to verify, and that decision belongs on a dashboard instead of in a configuration file nobody reads.

`reason` on the two refusal counters names the control that fired.

| Counter | Reasons |
| ------- | ------- |
| `datum_revisions_refused_total` | `unsigned`, `untrusted-signer`, `ambiguous-tag`, `not-descendant`, `unsupported-schema` |
| `datum_resources_refused_total` | `trust-anchor`, `hard-link`, `untrusted-path`, `unsafe-mode` |

Counters, not gauges, because the useful question is whether refusals are happening at all and
whether the rate changed, and a gauge showing the most recent pass loses everything before it.

`datum_trust_baseline_present` reports whether the host had a
[baseline revision](../lifecycle/enrolment.md#the-baseline-revision) when it started, which is how a
fleet finds the machines whose provisioning skipped that step. Those machines are the ones for which
downgrade protection was never armed, and nothing else reveals them.

`datum_trust_signer_info` exists for key rotation. Rotating a signer is [a provisioning task, not
something Datum does](../adr/0010-no-self-managed-trust-anchors.md), so the only way to know how far
a rotation has progressed is to ask every host which keys it trusts.

None of these disclose anything an attacker benefits from. A key identifier is public, a refusal count
is not a credential, and the mode is inferable from behaviour anyway, which is the test applied to
everything on this endpoint.

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
commit timestamp of that revision does, which is why the applied series answers this question and
the attempted one does not.

```text
max(datum_revision_applied_timestamp_seconds) - datum_revision_applied_timestamp_seconds
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

There are no per-resource metrics. A fleet of 500 hosts with 14 resources each would produce 7,000
series before labels, and the questions those series would answer are ones logs and status answer
better.

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
