# Support matrix

Nothing is supported. There is no implementation, so there is nothing to record.

An entry appears here when the behaviour exists, is tested against that
distribution, and matches the resource type documentation. Intent, partial work
and work-in-progress branches do not qualify.

## Resource types

| Type | Provider | Status |
| ---- | -------- | ------ |
| `Package` | `apt` | Not implemented |
| `Package` | `dnf` | Not implemented |
| `Package` | `apk` | Not implemented |
| `Package` | `pacman` | Not implemented |
| `File` | built in | Not implemented |
| `Directory` | built in | Not implemented |
| `Service` | `systemd` | Not implemented |
| `User` | built in | Not implemented |
| `Group` | built in | Not implemented |
| `Sysctl` | built in | Not implemented |
| `Symlink` | built in | Not implemented |
| `Repository` | `apt` | Not implemented |
| `Repository` | `dnf` | Not implemented |
| `Repository` | `apk` | Not implemented |

`File`, `Directory`, `Symlink`, `User`, `Group` and `Sysctl` are marked as built in because their
operations are kernel and libc interfaces rather than distribution tooling. That does
not make them distribution independent, since user and group management differs in
which utilities exist and in default id ranges, and it does mean they are unlikely to
need more than one provider each.

## Distributions

| Distribution | Identifier | Status |
| ------------ | ---------- | ------ |
| Debian | `debian` | Not implemented |
| Ubuntu | `ubuntu`, via `ID_LIKE=debian` | Not implemented |
| Fedora | `fedora` | Not implemented |
| RHEL | `rhel` | Not implemented |
| Rocky Linux | `rocky`, via `ID_LIKE` | Not implemented |
| AlmaLinux | `almalinux`, via `ID_LIKE` | Not implemented |
| Alpine Linux | `alpine` | Not implemented |
| Arch Linux | `arch` | Not implemented |

## What a supported entry will have to mean

The criteria for a supported entry are as follows.

The provider observes every field of the resource type, or declares the fields it cannot observe
with the consequence recorded in the type documentation. It applies every action the type defines. A
second pass over a converged resource produces no change, which is tested rather than assumed.
Verification detects a discrepancy introduced by the test. Failures are reported as errors, without
partially applied resources.

Each of those is tested against a real image of the distribution instead of a
mock, because command output formats and failure modes are where distributions
differ and are exactly what a mock gets wrong.

## Likely order

!!! note "Planned"

    The order below is intent, not commitment, and no part of it is scheduled.

`File` and `Directory` come first because they need no package manager and because
almost everything else depends on writing a file correctly. `Package` with `apt`
follows, then `Service` with `systemd`, which together are enough to reconcile a real
service on one distribution end to end. A second package provider comes after that,
because the second one is what tests whether the provider boundary actually holds or
whether `apt` assumptions leaked upward.

`User`, `Group` and `Sysctl` are simpler and are not on the critical path for
the remaining design work.
