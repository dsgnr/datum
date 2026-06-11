# Status

Status is designed before implementation on purpose, because status added afterwards reports
whatever the code happened to keep rather than what an operator needs. Fixing the shape first is
also a way of forcing the architecture to preserve the right information, since every field here is
something a component has to be able to produce.

!!! note "Proposed format"

    The fields are proposed. The renderings are illustrative and the layout is not settled. What
    the model has to contain is firmer than how it is printed.

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
    revision      7ab21f
    manifest      sha256:9d74e3
    lastKnownGood 7ab21f

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
    total       47
    converged   47
    drifted      0
    failed       0
    blocked      0
    skipped      0
```

| Field | Meaning |
| ----- | ------- |
| `desired.revision` | The revision the last pass resolved. |
| `desired.manifest` | The [digest](../fleet/effective-manifests.md) of the effective manifest applied. |
| `desired.lastKnownGood` | The last revision that resolved cleanly. Differs from `revision` when a newer commit failed to resolve, per [last known good](../reconciliation/last-known-good.md). |
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

`desired.revision` and `desired.lastKnownGood` being equal is the healthy case. Their divergence is
how a bad commit shows up in status without needing a separate error field.

```text
desired
  revision       9c02ab   (failed to resolve: unknown type PackageSet)
  lastKnownGood  7ab21f
```

The host is reconciling `7ab21f`, the newest commit did not resolve, and the reason is on the
record. Reading it across a fleet shows immediately which hosts have picked up a bad revision, which
is every host the commit reached, and confirms they are still enforcing the last good state instead
of sitting idle.

## Fleet status

A fleet is a set of hosts, so fleet status aggregates host status and introduces nothing new.

```text
fleet     example
revision  7ab21f

hosts     500
  converged        486
  drifted            0
  failed             3
  degraded           9
  awaiting-reboot    2
  unknown            0

behind revision      11   (last known good older than 7ab21f)
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

Status is consumed by other tools as well as read by people, so a structured form is required
alongside the human one.

```text
datum status --output json
```

!!! note "Open question"

    The exact JSON schema is undecided, and it is one of the interfaces that becomes a
    [contract](../reference/stability.md) the moment anyone automates against it. The field names
    above are the candidate schema, and they need settling before the first release, not after tools
    depend on a shape that was never designed.

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
