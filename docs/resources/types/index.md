# Resource types

Seven types are proposed for the first implementation. The set is small because every
type inherits the common behaviour decided in the rest of this section, and because
adding types is cheap once that behaviour is right and expensive before it is.

| Type | Manages | Target identity |
| ---- | ------- | --------------- |
| [`Package`](package.md) | Installed packages and versions | Package name |
| [`File`](file.md) | File content, ownership and mode | Absolute path |
| [`Directory`](directory.md) | Directory existence and metadata | Absolute path |
| [`Service`](service.md) | Whether a service runs now and at boot | Unit name |
| [`User`](user.md) | Local accounts and group membership | User name |
| [`Group`](group.md) | Local groups | Group name |
| [`Sysctl`](sysctl.md) | Kernel parameters, running and persisted | Parameter key |

!!! note "Proposed schemas"

    Every field on every page in this section is proposed. The `datum.dev/v1alpha1`
    API version means field names, defaults and semantics can change without a
    migration path.

## How these types were chosen

They are the smallest set that can describe a working service end to end. Installing
nginx, writing its configuration with the right ownership, creating the account it
runs as, and starting it at boot needs `Package`, `File`, `Directory`, `User`, `Group`
and `Service`, and `Sysctl` is included because kernel tuning is where drift is least
visible and most consequential.

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

`Repository`, for package sources, is the type most likely to be needed soonest,
because installing anything outside a distribution's default repositories currently
requires a `File` resource writing a sources list in a format that differs per package
manager. That pushes a distribution difference up into the fleet configuration, which
is where the design says it should not be.
