# Support matrix

`Package` on Debian is the first supported entry. It has been through the checks below against a
real Debian image, which is what an entry in this matrix has to mean. The three filesystem types are
reconciled end to end and have not, so they are recorded as working rather than supported.

An entry appears here when the behaviour exists, is tested against that
distribution, and matches the resource type documentation. Intent, partial work
and work-in-progress branches do not qualify.

## Resource types

| Type | Provider | Status |
| ---- | -------- | ------ |
| `Package` | `apt` | Supported on Debian |
| `Package` | `dnf` | Not implemented |
| `Package` | `apk` | Not implemented |
| `Package` | `pacman` | Not implemented |
| `File` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| `Directory` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| `Service` | `systemd` | Not implemented |
| `User` | built in | Not implemented |
| `Group` | built in | Not implemented |
| `Sysctl` | built in | Not implemented |
| `Symlink` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| `Repository` | `apt` | Not implemented |
| `Repository` | `dnf` | Not implemented |
| `Repository` | `apk` | Not implemented |

`User`, `Group` and `Sysctl` are marked as built in because their
operations are kernel and libc interfaces rather than distribution tooling. That does
not make them distribution independent, since user and group management differs in
which utilities exist and in default id ranges, and it does mean they are unlikely to
need more than one provider each.

## Distributions

| Distribution | Identifier | Status |
| ------------ | ---------- | ------ |
| Debian | `debian` | `Package` supported, `File`, `Directory` and `Symlink` working |
| Ubuntu | `ubuntu`, via `ID_LIKE=debian` | Untested, and expected to behave as Debian does |
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

Each of those is tested against a real image of the distribution instead of a mock, because command
output formats and failure modes are where distributions differ and are exactly what a mock gets
wrong. For `apt` those tests live behind a build tag, because they install and remove packages on
the machine that runs them, and `make test-apt` runs them in a throwaway Debian container.

`File`, `Directory` and `Symlink` reconcile and verify on Linux and are not recorded as supported,
since their tests run against a temporary directory rather than a named distribution. They contain
no distribution-specific behaviour, which has not been confirmed by testing.

## Likely order

!!! note "Planned"

    The order below is intent, not commitment, and no part of it is scheduled.

`File` and `Directory` came first because they need no package manager and because almost
everything else depends on writing a file correctly. `Package` with `apt` followed. `Service`
with `systemd` is next, and the three together are enough to reconcile a real service on one
distribution end to end. A second package provider comes after that, because the second one
is what tests whether the provider boundary actually holds or whether `apt` assumptions
leaked upward.

`User`, `Group` and `Sysctl` are simpler and are not on the critical path for
the remaining design work.
