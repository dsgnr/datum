# ADR-0007: Use effective manifests as the reconciliation input

## Status

Accepted

## Context

Resolving fleet configuration and reconciling a host are two different jobs. Resolution
reads a repository, evaluates matchers and merges layers. Reconciliation reads a machine,
compares, plans, applies and verifies.

They could be one process with a shared data structure passed internally, which is the
simplest thing to build. The resolved state would then be an implementation detail with no
serialised form, no name and no identity.

The consequences of that show up in several places at once. A bug in ordering could only be
reproduced by checking out the repository and re-resolving, rather than by handing over the input.
Two hosts believed to have the same configuration could not be compared except by comparing
configuration and reasoning about it.

## Decision

Resolution produces an effective manifest, which is a named artefact, not an internal value. It
holds the complete resolved desired state for one host at one revision, carries the provenance of
every field, and is identified by a digest of its content.

The reconciliation engine takes an effective manifest and nothing else from the repository.
The graph builder, observer, planner and reconciler never read a `Layer`, evaluate a
matcher, or examine the revision beyond recording it.

The manifest contains no observed state, no provider selection and no plan, all of which are
produced later from it.

## Consequences

The engine can be exercised without a repository. A manifest is enough to reproduce a
reconciliation problem, which makes ordering and verification bugs reportable by handing over
one file.

Disagreements about what should have happened can be settled by comparing two manifests instead of
reasoning about matchers and precedence.

A digest gives one value standing for the entire resolved desired state. Two hosts reporting
the same digest were given the same instructions, a host reporting an old digest has not
picked up a change, and a host reporting a digest the repository never produced is running
something it was not given.

Serialisation has to be canonical. Semantically identical manifests must produce identical
digests regardless of key ordering, whitespace or the order layers were folded in, which is a
real constraint that is easy to get wrong late and cheap to get right early.

Whether provenance is inside the digest is unresolved and matters. Including it means moving
a resource between layers changes the digest without changing what will happen on the host,
so every host appears to have new desired state after a refactor. Excluding it means two
manifests with identical resources and different provenance share a digest, which weakens the
claim that the digest identifies what Datum was told to do.

Several things a digest would enable are not designed and should not be read as promises.
Signing and staged rollback to an earlier manifest are both plausible on top of this decision
and neither exists.
