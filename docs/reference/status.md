---
description: "Interpret datum status reports, resource states and reconciliation outcomes. Understand current local reporting and the proposed fleet-wide view."
seo_title: "Datum status reports and host outcomes"
---

# Status

`datum status` reports the latest pass on the local host. The command and its text and JSON output
are implemented. This page describes the host report and a proposed fleet-wide view; the examples
are models, not exact renderings of the current command.

!!! note "Illustrative model"

    These examples group useful host-status fields for explanation; they are not the exact layout
    returned by `datum status`. Fleet-wide aggregation is not implemented.

## What status answers

A single host's status answers four questions at once, and keeping them distinct is the point.

What was Datum told to do, meaning the revision and manifest it resolved. What is the host's
condition now, meaning its [state](../concepts/state.md#host-state-across-passes). What happened on
the last pass, meaning when it ran, how long it took and what it changed. And how do the individual
resources break down, meaning the counts by [resource state](../concepts/state.md#resource-state-within-a-pass).

## The host status model

```text
host
  identity        web-001

  desired
    revisionAttempted    8b91f20
    revisionApplied      8b91f20
    manifest             sha256:3f2a9c4e
    lastKnownGood        8b91f20

  state
    condition       converged
    drifted         false
    rebootRequired  false
    mode            enforce

  reconciliation
    lastAttempt     2026-02-08T09:14:22Z
    lastSuccess     2026-02-08T09:14:22Z
    duration        183ms

  resources
    total       14
    converged   14
    drifted      0
    failed       0
    blocked      0
    skipped      0
```

| Field | Meaning |
| ----- | ------- |
| `desired.revisionAttempted` | The newest revision the last pass tried to resolve, whether or not it succeeded. |
| `desired.revisionApplied` | The revision whose desired state the host is actually reconciling. |
| `desired.manifest` | The [digest](../fleet/effective-manifests.md) of the effective manifest applied. |
| `desired.lastKnownGood` | The newest revision that resolved and validated cleanly, per [last known good](../reconciliation/last-known-good.md). |
| `state.condition` | The host [state](../concepts/state.md#host-state-across-passes). |
| `state.mode` | The [reconciliation mode](../concepts/reconciliation-modes.md) this host runs in. |
| `reconciliation.lastAttempt` | When the last pass ran, converged or not. |
| `reconciliation.lastSuccess` | When a pass last completed without failure. |
| `resources.*` | Counts by [resource state](../concepts/state.md#resource-state-within-a-pass). |

`lastAttempt` and `lastSuccess` are separate because their divergence is the signal that a host has
started failing. A host whose last attempt is recent and whose last success is hours old is
attempting and failing, which is a different and more urgent situation than a host that has not been
attempted lately.

## Divergence between desired and observed

All three revision fields being equal is the healthy case. Their divergence is how a bad commit shows
up in status without needing a separate error field.

```text
desired
  revisionAttempted  9c02ab   (failed to resolve: unknown type PackageSet)
  revisionApplied    8b91f20
  lastKnownGood      8b91f20
```

The host is reconciling `8b91f20`, the newest commit did not resolve, and the reason is on the
record. Separating attempted from applied is what makes that legible, because a single `revision`
field has to mean one or the other and each reading loses a question somebody needs answered.
Reading it across a fleet shows immediately which hosts have picked up a bad revision, which is
every host the commit reached, and confirms they are still enforcing the last good state instead of
sitting idle.

## Fleet status

A fleet is a set of hosts, so fleet status aggregates host status and introduces nothing new.

```text
fleet     example
revision  8b91f20

hosts     500
  converged        486
  drifted            0
  failed             3
  degraded           9
  awaiting-reboot    2
  unknown            0

behind revision      11   (last known good older than 8b91f20)
```

`behind revision` counts hosts whose last-known-good is older than the fleet's newest revision, which
covers both hosts that failed to resolve a newer commit and hosts that have not yet run since it
landed. Distinguishing those two requires comparing `lastAttempt` against the revision's age, which is
detail the aggregate leaves to a per-host view.

!!! note "Implementation status"

    Each host knows only its own status. Answering a fleet-wide question means collecting from
    every host, and Datum provides no mechanism for that. The model above is what a collector
    would aggregate, and collecting it is somebody else's job for now.

## Machine-readable output

Status can be consumed by other tools as well as read by people, so the command supports JSON
alongside human-readable text.

```text
datum status --output json
```

!!! note "Stability"

    JSON output is implemented, but it is not a stable interface before 1.0. Its shape may change;
    consumers should pin the Datum version they use and review the
    [stability policy](stability.md).

## What status is not

Status is a report from a host about itself, and a host reports what it observed. On a host
compromised at root, that report is only as trustworthy as the host, so a fleet of `converged` hosts
is a set of claims by those hosts and not independent evidence about them. This is the same point
the [threat model](../security/threat-model.md#an-attacker-with-root-on-one-managed-host) makes, and
it bears repeating here, because a green status dashboard is exactly the thing that gets mistaken
for attestation.

Status also says nothing about drift that happened after the last pass. A host reported `converged`
at its last pass can have been edited by hand a minute later, and the report will not reflect that
until the next pass observes it. Status is as fresh as the last pass and no fresher.

## Relationship to metrics

Status is the authoritative local record of a host's last pass, and
[metrics](../observability/metrics.md) are a numeric projection of it shaped for aggregation across a
fleet.

The division is that status carries per-resource detail and metrics carry counts. A fleet of five
hundred hosts with forty-seven resources each would need over twenty thousand metric series to expose
what status already holds locally, which is why metrics aggregate by state and the detail is read from
status on the one host that needs looking at.
