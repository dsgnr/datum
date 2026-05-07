# Effective manifests

An effective manifest is the resolved desired state for one host at one repository
revision. It is what composition produces and what the reconciliation engine takes
as input, and it is the boundary between deciding what should apply and doing
anything about it.

```text
host       web-001
revision   8b91f20
manifest   sha256:3f2a9c4e
resources  14
```

## What it contains

The manifest holds every resource that applies to the host, fully merged, with the
provenance of each field. It holds no resource that does not apply, and no record
of layers that were considered and did not match.

```text
manifest  sha256:3f2a9c4e
host      web-001
revision  8b91f20

Package[curl]               base
Package[ca-certificates]    base
Package[nginx]              roles/web
File[nginx-config]          roles/web, hosts/web-001
Service[nginx]              roles/web
Sysctl[net.ipv4.ip_forward] base, environments/production
User[www-data]              roles/web
...
```

What it does not contain is anything about the machine. No observed state, no
provider selection, and no plan. A manifest for a host that has never been built
is as complete as one for a host reconciling every minute, because producing it
requires only the repository.

## Content addressing

The manifest is identified by a digest of its own content.

A digest gives one value that stands for the entire resolved desired state, which
makes several questions cheap to answer. Two hosts reporting the same digest were
given the same instructions. A host reporting yesterday's digest has not picked up
a change. A host reporting a digest that was never produced from the repository is
running something it was not given.

For the digest to mean anything it has to be canonical, meaning that
semantically identical manifests produce identical digests regardless of key
ordering, whitespace, or the order in which layers happened to be folded. That
is a constraint on the serialisation, not on the model, and it is the kind of
detail that is easy to get wrong late.

!!! note "Open question"

    Whether provenance is inside the digest or alongside it is undecided, and the
    answer matters. Including it means that moving a resource between two layers
    changes the digest without changing what will happen on the host, so every
    host appears to have new desired state after a refactor. Excluding it means two
    manifests with identical resources and different provenance share a digest,
    which weakens the claim that the digest identifies what Datum was told to do.

## What content addressing could support

None of the following exists. They are recorded because they are the reason the
manifest is a named artefact with a digest rather than an internal intermediate
value.

Reproducibility
:   A revision and a host name are enough to regenerate a manifest and confirm it
    matches a digest reported earlier.

Status reporting :   A host reports the digest it last reconciled, which turns
fleet-wide status into a comparison of values instead of a diff of
configuration.

Rollback :   Reverting a commit produces the earlier manifest again, and the
digest confirms it is the same desired state, not something similar.

Signing
:   A digest is a small, stable thing to sign, so a host could verify that the
    desired state it received was produced by an authority it trusts.

Provenance
:   A recorded digest ties a change on a machine back to the exact resolved input
    that caused it.

!!! note "Implementation status"

    Signing and rollback support are not designed, let alone
    built. Listing them here is a statement about why the manifest is shaped this
    way, not a claim that any of it works.

## The manifest is the engine's only input

Everything after composition consumes the manifest and nothing else from the
repository. The graph builder, observer, planner and reconciler never read a
`Layer`, evaluate a selector, or look at the revision beyond recording it.

It also bounds what the rest of the system has to understand. A bug in ordering or
verification can be reproduced from a manifest without a repository, and a
disagreement about what should have happened can be settled by comparing two
manifests rather than by reasoning about selectors.
