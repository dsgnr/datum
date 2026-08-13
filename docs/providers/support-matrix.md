# Support matrix

`Package` on Debian and Fedora, and `Service` on Debian, are the supported entries. Each has been
through the checks below against a real image of that distribution. The three filesystem types
reconcile end to end without having been through those checks, so they are recorded as working
rather than supported.

An entry appears here when the behaviour exists, is tested against that
distribution, and matches the resource type documentation. Intent, partial work
and work-in-progress branches do not qualify.

## Resource types

| Type | Provider | Status |
| ---- | -------- | ------ |
| `Package` | `apt` | Supported on Debian |
| `Package` | `dnf` | Supported on Fedora |
| `Package` | `apk` | Not implemented |
| `Package` | `pacman` | Not implemented |
| `File` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| `Directory` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| `Service` | `systemd` | Supported on Debian |
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
| Debian | `debian` | `Package` and `Service` supported, `File`, `Directory` and `Symlink` working |
| Ubuntu | `ubuntu`, via `ID_LIKE=debian` | Untested, and expected to behave as Debian does |
| Fedora | `fedora` | `Package` supported, `File`, `Directory` and `Symlink` working |
| RHEL | `rhel` | Untested, and served by the same provider as Fedora |
| Rocky Linux | `rocky`, via `ID_LIKE` | Untested, and expected to behave as RHEL does |
| AlmaLinux | `almalinux`, via `ID_LIKE` | Untested, and expected to behave as RHEL does |
| Alpine Linux | `alpine` | Not implemented |
| Arch Linux | `arch` | Not implemented |

## What a supported entry will have to mean

The criteria for a supported entry are as follows.

The provider observes every field of the resource type, or declares the fields it cannot observe
with the consequence recorded in the type documentation. It applies every action the type defines. A
second pass over a converged resource produces no change, which is tested rather than assumed.
Verification detects a discrepancy introduced by the test. Failures are reported as errors, without
partially applied resources.

Each of those is tested against a real image of the distribution, since command output formats and
failure modes are where distributions differ. For `apt` and `systemd` the tests sit behind a build
tag, as they install packages and start services on the machine that runs them. `make test-apt`,
`make test-dnf` and `make test-systemd` run them in throwaway containers. The systemd one boots
systemd as PID 1, because systemctl is present in plenty of images where nothing booted it and
testing against that would prove nothing.

`File`, `Directory` and `Symlink` reconcile and verify on Linux and are not recorded as supported,
since their tests run against a temporary directory rather than a named distribution. They contain
no distribution-specific behaviour, which has not been confirmed by testing.

## Likely order

!!! note "Planned"

    The order below is intent, not commitment, and no part of it is scheduled.

`File` and `Directory` came first, as they need no package manager and most other work depends on
writing a file correctly. `Package` with `apt` and `Service` with `systemd` followed, which together
reconcile a service on one distribution end to end. `dnf` came next as a second provider for one
type, which tests whether `apt` assumptions had leaked above the provider boundary. Two had. `rpm`
reports an absent package on stdout where `dpkg-query` uses stderr, and `dnf5` rejects the `--`
end-of-options separator that `apt-get` accepts. Both were found by testing against a real image.

`User`, `Group` and `Sysctl` are simpler and are not on the critical path for
the remaining design work.
