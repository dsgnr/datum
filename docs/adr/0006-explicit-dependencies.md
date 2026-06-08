# ADR-0006: Represent resource dependencies explicitly

## Status

Accepted

## Context

Some resources have to be applied before others. A package installs a unit file before the
service can start, a directory exists before a file is written into it, a group exists
before a user references it. Something has to determine that order.

Document order is the obvious candidate and does not work. Composition means a resource's fields can
come from several layers, folded in precedence order and not reading order, so a merged resource
does not occupy a single position in the repository. `File[nginx-config]` contributed by a role
layer and a host layer exists in two files at once, and asking which line it is on has no answer.

Even setting composition aside, ordering by position makes a repository fragile in ways
that are invisible in review. Moving a document to improve readability would change the
order of operations across the fleet, splitting one file into two would change it again,
and a different filesystem enumerating files differently would change it a third time.

Inference from the resources themselves is the more interesting alternative. Path nesting
could order a file after the directory containing it, ownership could order it after the
user owning it, and a service could be ordered after any package. Each rule is plausible
in isolation and the set is never complete, so the order would depend partly on the
repository and partly on which inference rules the implementation happened to include.
Predicting a plan would require knowing those rules, which conflicts directly with
behaviour being derivable from the configuration and the documentation.

## Decision

Dependencies are declared. `requires` lists resource references that must be processed
before a resource, and it is the only source of ordering along with `restartOn`, which
orders as well as triggering a reaction.

Nothing is inferred. A `File` whose path sits inside a managed `Directory` gets no edge
unless one is declared, even though the relationship is obvious from the paths.

A reference that does not resolve to a resource in the same effective manifest is an error,
and a cycle is an error. Both are raised before the host is read, and neither is resolved by
dropping an edge.

## Consequences

Plan order is fully determined by the repository. Two runs over the same manifest produce
the same order, and reading the resources shows what constrains what.

Dependencies are the most common thing to forget, and the resulting failures are the
confusing kind. A missing edge works on a host where the package is already installed and
fails on a fresh one, which means the mistake is often found long after it was made.

That cost is accepted, and warning about likely missing edges is recorded as proposed
behaviour. A warning keeps ordering determined by the repository while catching the common
case, which is the only mitigation that does not reintroduce inference.

Ordering has to be written out even where it is obvious, which makes manifests more verbose
than they would otherwise be. The verbosity is visible in review, which is the compensating
benefit.

The graph is derived from the manifest on every pass and never stored, so there is no
cached ordering that could disagree with the repository.

Removal ordering is not handled. `requires` means processed before, regardless of the action, so a
manifest that removes a group and the users referencing it has to express the reverse of the
creation order explicitly. Whether the planner should invert edges for `remove` actions is an open
question, and doing so would make plan order depend on the action rather than only on the declared
graph.
