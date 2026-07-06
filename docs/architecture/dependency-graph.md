# Dependency graph

The resource graph is built from the effective manifest before the host is read. It
is what turns a set of resources into an order, and it is the only thing that
determines that order.

```text
Package[nginx]
      │
      ▼
File[nginx-config]
      │
      ▼
Service[nginx]
```

Nodes are resources, identified by resource reference. Edges are declared
relationships between them, and there is no other source of ordering in the system.

## Document order does not define execution order

Where a resource appears in a file, and where that file appears on disk, have no
effect on when it is applied.

The reason is composition. A resource's fields can come from several layers, and the layers are
folded in precedence order, not in reading order, so there is no single position in the repository
that a merged resource occupies. `File[nginx-config]` contributed by a role layer and a host layer
exists in two files at once, and asking which line it is on has no answer.

Even without composition, ordering by position would make a repository fragile in a
way that is invisible in review. Moving a document to improve readability would change
the order of operations on a thousand machines, and splitting one file into two would
change it again. Sorting the files differently on a different filesystem would change
it a third time.

Declared edges avoid all of that, at the cost of requiring dependencies to be written
out. That cost is accepted, and the consequence is recorded on the
[dependencies](../resources/dependencies.md) page, since forgetting an edge is the most
common mistake the model allows.

## Edge kinds

Two fields produce edges, and both order.

| Field | Ordering | Reaction |
| ----- | -------- | -------- |
| `requires` | Yes | No |
| `restartOn` | Yes | Yes |

`requires` says the referenced resource is processed first. `restartOn` says the same
and adds that a change to the referenced resource causes this one to be updated.

Reaction implying order is deliberate. A service declaring `restartOn` for its
configuration file requires the file to be written before the restart, so the
reference does not have to be repeated in `requires`.

## Validation

The graph builder rejects three things, and all of them before the host is read.

**Unresolved references.** An edge pointing at a resource reference that is not
in the manifest is an error, not an edge silently dropped.

```text
error: unresolved dependency

  Service[nginx] requires Package[nginx]
  Package[nginx] is not present in the manifest for web-001
```

The common cause is a layer that was expected to match and did not, and the
error names the unresolved reference.

**Cycles.** A cycle has no valid order, and dropping an edge to break it is not
attempted.

```text
error: dependency cycle

  Service[nginx] -> File[nginx-config] -> Package[nginx] -> Service[nginx]
```

**Duplicate target identities.** Two resources managing the same thing on the host are
a [conflict](../resources/conflicts.md), detected here because this is the first point
at which the whole manifest is examined together.

## From partial order to total order

The graph constrains order partially. Two resources with no path between them have no
required relationship, and a topological sort of the graph alone would allow many valid
orders.

A plan is a single sequence, so the remaining freedom is resolved deterministically by
sorting candidates by resource reference at each step. `Package[curl]` and
`Package[nginx]` are unrelated, and `Package[curl]` comes first because `curl` sorts
before `nginx`.

Determinism here is not cosmetic. Two runs over the same manifest and the same observed
state have to produce identical plans, because a plan that differs between runs cannot
be compared against a previously reviewed one, and a diff between two plans stops being
readable if unrelated actions move around.

## Failure propagation

When an action fails, every resource reachable from it through the graph is skipped.

```text
failed   File[nginx-config]     permission denied
skip     Service[nginx]         dependency File[nginx-config] failed
none     Package[nginx]         present, 1.24.0-2
```

Propagation is transitive. A resource depending on something that was skipped is
also skipped, with the reason naming the resource it depended on instead of the
original failure, so the chain can be followed and every skip does not point at
the same root cause.

Resources not reachable from the failure are unaffected. A manifest where one file fails to write
still reconciles the other forty resources.

## Concurrency

The graph contains enough information to run unrelated actions at the same time, since
any two resources with no path between them have no ordering requirement.

Nothing in the design requires it, and the first implementation is expected to apply actions one at
a time in plan order. A sequential reconciler is much easier to reason about when something fails
halfway, and the actions Datum performs are mostly bounded by package manager locks and disk, not by
anything concurrency would help.

The plan stays a total order whichever is used, so enabling concurrency later would not
change what a plan looks like or how it is reviewed.

!!! note "Open question"

    Package managers hold exclusive locks, so concurrent `Package` actions
    serialise on that lock. Whether concurrency should be per resource type, or
    should exist at all, has not been examined.

## What the graph does not do

It does not infer edges. A `File` inside a managed `Directory` gets no edge unless one
is declared, and the reasoning for that is on the
[dependencies](../resources/dependencies.md) page.

It does not choose actions. The graph is built before observation, so at the
point it exists nothing is known about what needs changing. It constrains the
order of the actions the planner produces.

It does not survive the pass. The graph is derived from the manifest on every pass and is not
stored.

!!! note "Proposed behaviour"

    Warning about a likely missing edge is one option, given how often forgetting one is the cause
    of a failure that only appears on a fresh host. A `File` whose path sits under a managed
    `Directory` with no edge between them, or a `Service` with no edge to any `Package`, are both
    detectable without changing what the plan does. A warning keeps the order fully determined by
    the repository while catching the common mistake.
