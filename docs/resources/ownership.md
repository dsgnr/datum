# Ownership

The most consequential question the resource model has to answer is what a declaration
claims about everything it does not mention.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
```

This says nginx must be installed. It does not say nginx must be the only package
installed, and it does not say anything about `tcpdump` that somebody added by hand. That
is a deliberate choice with a name.

## Assertive resources

Every resource type in the [initial set](types/index.md) is assertive. An assertive
resource makes a claim about one target and holds no opinion about anything else of the
same type.

`Package[nginx]` asserts that nginx is present. `User[deploy]` asserts that the deploy account
exists with the fields it declares. Neither is affected by, and neither affects, any other package
or account on the host. This follows directly from [declared-only
ownership](../adr/0009-declared-only-ownership.md), where a target nobody declared is unmanaged, so
`tcpdump` installed by hand stays installed and produces no drift.

Assertive resources compose cleanly, because two layers can each add resources of the same
type without contradicting each other. They are also safe to adopt incrementally, because
adding a repository to an existing machine changes only what it names.

What they cannot express is a closed set. There is no assertive way to say "these are the
only packages that should exist", because each resource speaks only for its own target.

## Authoritative sets

Some state is only meaningful as a complete set. The list of accounts permitted to log in,
the sudoers entries, the packages allowed on a hardened host, and the files in a drop-in
directory are all cases where what is absent matters as much as what is present.

An assertive model cannot say that, so a second kind of resource is proposed.

!!! note "Proposed design"

    Authoritative sets are proposed, not accepted. The shape below is concrete enough to
    argue with. Nothing about it is implemented, and the interaction with composition,
    described later on this page, is the part most likely to change.

An authoritative set declares the complete membership of some category, and anything on the
host in that category but not in the set is drift.

```yaml
datum: v1alpha1
type: PackageSet

name: baseline

desired:
  authoritative: true
  members:
    - nginx
    - curl
    - ca-certificates
```

With `authoritative: true`, a package installed on the host and absent from `members` is
reported as drift and, under an enforcing [reconciliation
mode](../concepts/reconciliation-modes.md), removed. With `authoritative: false`, the set is
a convenience grouping equivalent to several assertive `Package` resources and removes
nothing.

The default is `false`. An authoritative set is destructive by design, so it has to be asked
for explicitly, and a set that becomes authoritative through a forgotten default would remove
packages nobody intended to touch, on every host it matched, as root.

## Why a set is a different type

Authority is a property of a whole category, not of one target, so it cannot be a field on
`Package`.

A `Package` resource speaks for one package. There is no target it could carry that means "every
package", and bolting an `authoritative` field onto it would make one resource silently responsible
for the removal of others it never names. Keeping the set a distinct type, `PackageSet` rather than
`Package`, means the destructive claim is visible in the type itself and cannot be arrived at by
accident.

The categories that plausibly need an authoritative form are packages, users, groups, and the
contents of a directory. Services and sysctl parameters are harder, because the complete set
of units on a host or parameters in a kernel is large, mostly not Datum's concern, and unsafe
to treat as removable.

## Authoritative sets and composition

This is where the design is hardest and least settled, because authority and composition pull
against each other.

A `PackageSet` declares a complete list. [Composition](../fleet/composition.md) exists so that
a base layer and a role layer can each contribute part of a machine's configuration. If two
layers both contribute an authoritative `PackageSet` with the same name, the merge rule for
lists is replacement, so the higher-precedence set wins outright and the base list is
discarded. A base layer asserting that `curl` belongs, and a role layer adding `nginx`, would
produce a host with only `nginx`, which is not what either layer intended.

!!! note "Open question"

    Whether authoritative sets should merge their members across layers, in defiance of the general
    list-replacement rule, is undecided and is the central difficulty. Merging members makes
    composition intuitive and reintroduces the problem that no single layer can see the complete set
    it is asserting, which is what the type exists for. Replacing means one layer owns the entire
    set, which is predictable and awkward to compose.

    A likely resolution is that an authoritative set is owned by exactly one layer and may not
    be contributed to by others, so that authority and composition never apply to the same
    resource. That has not been worked through.

## How assertive and authoritative resources coexist

Both kinds can manage the same category on the same host, and the rule is that assertive
resources add and authoritative sets bound.

`Package[nginx]` asserting presence, alongside a `PackageSet` whose members do not include nginx, is
a contradiction, because one resource requires nginx and another would remove it. This is a
[conflict](conflicts.md) detected during resolution, in the same way as two resources sharing a
target, and not a race resolved by whichever runs last.

A host with no authoritative set for a category is managed purely assertively, which is the default
and the incremental-adoption path. Introducing an authoritative set is the step that changes a
category from "Datum manages these" to "Datum manages all of these", and it is a deliberate
escalation, not a setting that drifts on.

## Expressing the three readings

Each of the three readings has a definite answer.

| Reading | Expressed as |
| ------- | ------------ |
| nginx must exist | `Package[nginx]` with `state: present` |
| nginx must exist at a version | `Package[nginx]` with `version` set |
| nginx must be the only thing here | An authoritative `PackageSet` |

The first two are accepted and specified on the [Package](types/package.md) page. The third
is proposed, because getting authoritative composition right matters more than having the
feature early, and because a fleet can go a long way on assertive resources alone.
