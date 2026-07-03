# Capabilities

A distribution is not a provider. It is a set of choices about which provider satisfies each
resource type, and naming that set is what keeps distribution knowledge out of everything above
the [provider boundary](index.md).

## The question to ask instead

The tempting question, asked wherever behaviour differs, is whether the host is Ubuntu. That
question spreads, because the next difference asks it again somewhere else, and before long the
distribution check is in the resource model, the planner and the fleet configuration.

The better question is which provider satisfies each type on this host.

```text
Package   -> apt
Service   -> systemd
User      -> linux-user
Group     -> linux-group
Sysctl    -> proc-sys
File      -> posix-file
```

That mapping is a capability set. It is resolved once, from the host, and every resource of a
given type uses the provider the set names for it. Nothing downstream asks what the distribution
is, because the distribution has already been reduced to a set of provider choices.

## Distributions as capability sets

The distributions the design targets differ in some capabilities and agree on others.

| Type | Debian / Ubuntu | Fedora / RHEL | Alpine | Arch |
| ---- | --------------- | ------------- | ------ | ---- |
| `Package` | `apt` | `dnf` | `apk` | `pacman` |
| `Service` | `systemd` | `systemd` | `openrc` | `systemd` |
| `User` | `linux-user` | `linux-user` | `linux-user` | `linux-user` |
| `Group` | `linux-group` | `linux-group` | `linux-group` | `linux-group` |
| `Sysctl` | `proc-sys` | `proc-sys` | `proc-sys` | `proc-sys` |
| `File` | `posix-file` | `posix-file` | `posix-file` | `posix-file` |
| `Symlink` | `posix-file` | `posix-file` | `posix-file` | `posix-file` |
| `Repository` | `apt` | `dnf` | `apk` | `pacman` |

Two things are visible in that table that a distribution-centric model obscures. Most capabilities
are shared, so the great majority of provider code is written once and is not per-distribution at
all. And the differences do not line up with distribution boundaries, since Alpine differs from the
others in `Service` and agrees on everything else, so the meaningful unit is the capability, not the
distribution.

`Repository` is the one type where every distribution needs its own provider, since each package
manager expresses a package source differently. That difference is also why it is [a type rather
than a file](../resources/types/repository.md#a-package-source-is-its-own-type), since a file would
carry the format into the repository.

## How a capability set is resolved

Resolving a capability set is [provider selection](selection.md), viewed as producing a complete
mapping rather than choosing one provider at a time.

The host is identified from [`os-release`](selection.md#what-os-release-provides), each resource type
in the manifest has its provider selected by the [proposed rule](selection.md#proposed-rule), and the
result is the set of providers this host will use. The set is recorded in the plan, so the capability
mapping is visible before anything is applied, in the same way an individual provider choice is.

A type with no provider on this host leaves a gap in the set. Resources of that type are
[skipped](selection.md#when-no-provider-matches) and the host is
[`degraded`](../concepts/state.md#host-state-across-passes) instead of failed, because a missing
capability is a coverage gap and not a fault.

## Why this is worth formalising now

The capability framing is not a feature, it is a discipline that keeps the provider boundary
honest, and disciplines are cheaper to adopt before there is code than after.

A resource type is defined by what it means, not by how any distribution realises it. A provider is
defined by the one type it satisfies on the one class of system it understands. The set that maps
types to providers for a host is the only place the two meet, and it is resolved in one step from
one input. Nothing else in Datum is permitted to ask what the distribution is, and the capability
set is what makes that restriction practical and not aspirational, because it gives every downstream
component the provider it needs without the component having to know why that provider was chosen.

!!! note "Proposed design"

    Capability sets describe the intended structure and are not implemented. The provider names
    used here, `linux-user`, `proc-sys`, `posix-file` and the rest, are illustrative, and the
    [support matrix](support-matrix.md) records that none of them exist yet.
