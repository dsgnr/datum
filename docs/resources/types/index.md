# Resource types

Nine types are proposed for the first implementation. The set is small because every
type inherits the common behaviour decided in the rest of this section, and because
adding types is cheap once that behaviour is right and expensive before it is.

## Domains

The types are grouped into four domains, by the part of a host each type describes.

A domain is a grouping. It is not part of a resource's identity, it does not appear in a
document, and no reconciliation behaviour depends on it. A resource is written
`type: Package` and referred to as `Package[nginx]` whichever domain the type sits in.
The grouping gives this section an order and gives later work somewhere to attach
behaviour that applies to a whole domain.

### Core

Packages, package sources and the filesystem, which is most of what a manifest declares.

| Type | Manages | Target identity |
| ---- | ------- | --------------- |
| [`Package`](package.md) | Installed packages and versions | Package name |
| [`Repository`](repository.md) | Package sources the host may install from | Source identifier |
| [`File`](file.md) | File content, ownership and mode | Absolute path |
| [`Directory`](directory.md) | Directory existence and metadata | Absolute path |
| [`Symlink`](symlink.md) | Symbolic links and their targets | Absolute path |

### Identity

The local account database.

| Type | Manages | Target identity |
| ---- | ------- | --------------- |
| [`User`](user.md) | Local accounts and group membership | User name |
| [`Group`](group.md) | Local groups | Group name |

### Runtime

What is running on the host.

| Type | Manages | Target identity |
| ---- | ------- | --------------- |
| [`Service`](service.md) | Whether a service runs now and at boot | Unit name |

### Kernel

Kernel parameters.

| Type | Manages | Target identity |
| ---- | ------- | --------------- |
| [`Sysctl`](sysctl.md) | Kernel parameters, running and persisted | Parameter key |

### Where a new type goes

A type belongs to the domain describing what it manages on the host. A type that reads as
though it belongs to two domains is usually two types.

`Runtime` holds one type today and is expected to take a unit-level type alongside
`Service`, managing a unit file rather than the state of a service.

The domains cover the current set and do not cover everything. Of the types [left out
below](#types-considered-and-left-out), `Mount` sits between Core and Kernel, since it
manages a filesystem through a kernel interface, and `Firewall`, `Hostname` and `Timezone`
have no domain here at all. Adding a fifth domain is expected before any of those is
added, and stretching one of these four to fit would lose the property that makes the
grouping worth having.

!!! note "Proposed schemas"

    Every field on every page in this section is proposed. The `datum: v1alpha1` schema
    marker means field names, defaults and semantics can change without a migration
    path.

## How these types were chosen

They are the smallest set that can describe a working service end to end. Installing
nginx, writing its configuration with the right ownership, creating the account it
runs as, and starting it at boot needs `Package`, `File`, `Directory`, `User`, `Group`
and `Service`, and `Sysctl` is included because kernel tuning is where drift is least
visible and most consequential.

`Repository` and `Symlink` were added to that set for narrower reasons. A package source
is [a type rather than a file](repository.md#a-package-source-is-its-own-type), since a
file would put the format of a sources list into the repository. A symbolic link is a
distinct thing on disk from the file it points at, and a `File` resource cannot describe
one without its fields meaning two things.

What they have in common is that each one can be read back from the host.
Observability is the selection criterion, since a type whose state cannot be
observed cannot be diffed, planned or verified.

## What each page specifies

Each type page gives the fields with their types and whether they are required,
what observation reports, which actions apply, and the provider differences that
cannot be hidden. Each also records the open questions specific to that type,
because most of the unresolved detail in the resource model sits at this level
rather than in the common behaviour.

## Types considered and left out

`Exec` or `Command` is the obvious omission and it is deliberate. A resource that runs
a command has no state to read back, so it cannot be diffed or verified, and its
presence would make every other guarantee conditional on whether a repository uses it.

`Cron` was left out because scheduled work is a file in a directory, and a `File`
resource writing to `/etc/cron.d` describes it without a new type. Whether that stays
true once timers and schedules need validating is not certain.

`Mount` and `Firewall` are both plausible next types and both need more design than the
current set. Mounts exist in two places like `Sysctl` does, being the running mount
table and `/etc/fstab`, and firewall rules have ordering semantics inside a single
resource that nothing else in the model has.

`Hostname` and `Timezone` are each a single value with an obvious target
identity, and both are held back only by being uninteresting, not difficult.
