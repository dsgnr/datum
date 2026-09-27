---
description: "Understand Datum’s core concepts: desired and observed state, field-level drift, ordered plans, verification, convergence and failure handling."
seo_title: "Desired state, drift and reconciliation - Datum"
---

# Core concepts

Datum compares what a repository declares with what a host currently reports, builds a
plan, then applies and verifies changes. These definitions are shared by the user guide
and the engineering specification.

```text
desired state + observed state -> drift -> plan -> apply -> verify
```

Each phase answers a different question. Observation reads the host; diffing identifies
fields that disagree; planning orders actions; applying changes the host; verification
reads it again to establish the result. Previewing a plan uses the same observation and
comparison as a full pass.

## Desired state

Desired state is the set of resources selected for one host at one repository revision.
The repository describes a fleet; resolving its layers and matchers produces a host's
[effective manifest](../fleet/effective-manifests.md). Two hosts at the same revision can
have different desired states.

### Repository revision

A resolution records the exact Git commit, not a branch name that can move. Recording the
host and commit makes the intended configuration reproducible and lets an operator trace
changes to their source. A manifest digest identifies content but cannot order revisions;
Git history and the [accepted revision](../security/repository-trust.md#verifying-that-a-revision-is-current)
establish which revision follows another.

### Absence does not mean removal

An undeclared resource is unmanaged. Deleting `Package[nginx]` from the repository leaves
nginx installed and stops Datum managing it. Removal requires an explicit declaration:

```yaml
datum: v1alpha1
type: Package
name: nginx
desired:
  state: absent
```

This [declared-only ownership](../resources/ownership.md) lets a fleet adopt existing
machines incrementally. Whether a `Directory` should support reclaiming all undeclared
contents is a separate [open question](../development/open-questions.md).

### Resolution does not read the host

Matchers use labels from the repository's `Host` document. Resolution therefore works
from a checkout without reaching the target machine. Provider selection happens afterwards:
it reads the host to choose an implementation such as `apt` or `dnf`, without changing
which resources apply.

Matching observed facts would remove that independence. The current model uses declared
labels only; matching observed facts remains a [design question](../development/open-questions.md).

### Desired state is not a record of what Datum did

Every pass compares repository intent with the host as it is now. It does not treat a
record of previous actions as the host's current state. Reports describe past work;
they are not inputs to the next comparison.

## Observed state

Observed state is what providers read for the resources in the effective manifest at a
point in time. Observation is scoped to declared targets, rather than an inventory of the
whole machine. A `File` for `/etc/nginx/nginx.conf` observes that target, not every file in
`/etc/nginx`.

Observation changes nothing on the host. Planning depends on that guarantee, and
verification depends on reading a result without altering it. Providers report existence
and the fields their resource type defines, such as file ownership, mode and content digest.
Rendering a content difference is separate from measuring it.

### Fields that cannot be observed

A field a provider cannot read is unknown, not an assumed value. Unknown fields cannot
be compared, planned for or verified.

!!! note "Proposed behaviour"

    The intended report marks unknown fields in the plan and treats unobservable identifying
    state as an error. The exact reporting has not been designed.

### Observation is a snapshot

Other processes can change a target between observation and apply. Datum does not lock
the whole machine. Verification reads affected resources again to check the result, and
the next pass measures the host afresh. Observations are not cached between passes.

## Drift

Drift is a difference between desired and observed state for a declared resource. A manual
host change and a repository commit produce the same kind of difference and follow the
same reconciliation path. A host returning after a month offline compares its current
state with the selected revision; it needs no replay of intermediate changes.

### Drift is per field

```text
File[nginx-config]
  mode     0644 -> 0600       drift
  owner    root               match
  content  sha256:91c4de2a    match
```

The plan names individual differences so providers can make narrow changes. Correcting a
mode need not rewrite content. Undeclared targets and unknown fields are outside drift
reporting, so a converged manifest does not imply that the whole machine is managed.

### Drift no provider will correct

An existing user's [UID](../resources/types/user.md#identifiers) or group's
[GID](../resources/types/group.md#identifiers) is compared and reported but not changed.
Changing it could orphan file ownership, and the manifest does not describe which files
to reassign. These fields remain visibly `drifted` while other resources can converge.

### Where drift comes from

Manual edits, package upgrades, runtime settings lost on reboot and competing management
tools can all produce drift. If another process keeps rewriting the same target, Datum
may correct it on every pass without resolving the underlying conflict.

!!! note "Open question"

    Detecting repeated correction across passes has not been designed. It would require
    diagnostic history, separate from the current observations used to compute drift.

[`observe` mode](reconciliation-modes.md) reports drift without applying changes. It uses
the same comparison as enforcement and can run permanently as a drift detector.

## Plan

A plan is the ordered set of actions for one desired state, one observation and the
resources' dependencies. Producing a plan changes nothing on the host.

### Actions

Every resource appears in the plan, including those requiring no change.

| Action | Meaning |
| ------ | ------- |
| `none` | Observed state satisfies desired state; no apply call is needed. |
| `create` | The target is missing and should exist. |
| `update` | The target exists and declared fields or triggered behaviour require a change. |
| `remove` | The target exists and is explicitly declared absent. |
| `skip` | The action cannot be attempted because a dependency failed or no provider is available. |

A service restart triggered by a changed configuration file is an `update`, with the
trigger recorded as its reason. It does not introduce a separate action type.

### What a plan contains

A plan identifies the host, revision and manifest, and records each resource's action,
differing values, selected provider, provenance and reasons for skipping it. The design
also calls for explaining which labels caused a layer to match.

```text
update   File[nginx-config]
         mode      0644 -> 0600
         from      roles/web, hosts/web-001

update   Service[nginx]
         reason    File[nginx-config] changed, restartOn matched

none     Package[nginx]
```

This is illustrative output. The [CLI reference](../reference/cli.md) describes the
implemented commands; a stable machine-readable plan format remains an
[open question](../development/open-questions.md).

### Ordering

The [dependency graph](../architecture/dependency-graph.md) determines action order,
not file or document position. The plan puts unrelated resources in a stable order too,
so the same inputs produce the same ordering. Whether unrelated actions can be applied
concurrently is a separate reconciler decision.

### A plan is not a stored artefact to replay

Reconciliation builds its own plan from a fresh observation. An earlier preview can go
stale and is never replayed as an instruction to apply.

!!! note "Open question"

    Applying only when a fresh plan is equivalent to a previously approved plan could
    support change control without acting on stale observations. This has not been designed.

When every action is `none`, the plan is empty of changes. An unchanged host should reach
this point on the pass after a successful reconciliation; that is the expected idempotent
behaviour.

## Reconciliation

A reconciliation pass resolves desired state, observes, diffs, plans, applies and verifies
for one host. The pass reports an overall outcome even if only some actions succeeded.

### One pass at a time

An [exclusive pass lock](../reconciliation/locking.md) covers observation through
verification, preventing two Datum processes from applying overlapping plans. It does
not prevent another program from changing the host.

### Convergence

A host is converged when every resource in its effective manifest has been observed and
satisfies desired state. Successful commands alone do not establish convergence:
a service restart can return zero and the service can still fail afterwards.
[Verification](../resources/lifecycle.md#verify) reads affected resources back to confirm
the result.

### Pass outcomes

| Outcome | Meaning |
| ------- | ------- |
| `converged` | No changes were needed. |
| `changed` | Actions were applied and affected resources verified. |
| `failed` | An action failed or verification did not confirm its result. |
| `drifted` | Differences were reported without correction, because of observe mode or an uncorrectable field. |

Drift left after an attempted correction fails verification. Drift deliberately left
untouched is a report, not an apply failure. Pass outcomes are distinct from the
[resource and host states](state.md) shown in status reports.

### There is no rollback

A failed pass does not undo successful actions. Recover by correcting or reverting the
change in Git and reconciling again. Use a new revert commit, rather than resetting the
tracked branch behind the host's accepted revision.

Desired-state rollback is not system-state rollback. Restoring earlier declarations
cannot recover deleted data, undo database migrations or guarantee that an older package
is still available. A fleet needing transactional rollback must provide it through
mechanisms such as filesystem snapshots or image generations.

### Failure containment and retry

When an action fails, dependent resources are blocked; independent resources can still
reconcile. A configuration file that failed to write must not trigger its service restart.
A failed action is not retried within the same pass. The next pass observes again and
plans from the state that remains.

[Scheduling](../reconciliation/scheduling.md) spreads passes deterministically across the
interval. [Failure handling](../reconciliation/failure-handling.md) describes upstream
back-off and local failures that keep the normal interval.

## Continue reading

Use [reconciliation modes](reconciliation-modes.md) to choose whether the agent applies
changes, and [state and lifecycle](state.md) to interpret its reports. The
[fleet guide](../fleet/index.md) explains how labels and layers select resources;
[resource behaviour](../resources/index.md) defines what those resources manage.
The [glossary](../reference/glossary.md) provides a compact lookup for all terms.
