# Support matrix

`Package` on Debian and Fedora, and `Service` on Debian, are the supported entries. Each has been
through the checks below against a real image of that distribution. The three filesystem types
reconcile end to end without having been through those checks, so they are recorded as working
rather than supported.

An entry appears here when the behaviour exists, is tested against that
distribution, and matches the resource type documentation. Intent, partial work
and work-in-progress branches do not qualify.

## Resource types

The rows are grouped by [domain](../resources/types/index.md#domains), in the order that
section uses.

| Domain | Type | Provider | Status |
| ------ | ---- | -------- | ------ |
| Core | `Package` | `apt` | Supported on Debian |
| Core | `Package` | `dnf` | Supported on Fedora |
| Core | `Package` | `apk` | Not implemented |
| Core | `Package` | `pacman` | Not implemented |
| Core | `Repository` | `apt` | Supported on Debian, except `priority` |
| Core | `Repository` | `dnf` | Supported on Fedora, except `suite` and `components` |
| Core | `Repository` | `apk` | Not implemented |
| Core | `File` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| Core | `Directory` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| Core | `Symlink` | `posix-file` | Reads and writes on Linux, reads elsewhere |
| Identity | `User` | `linux-user` | Working, not yet through the checks |
| Identity | `Group` | `linux-user` | Working, not yet through the checks |
| Runtime | `Service` | `systemd` | Supported on Debian |
| Kernel | `Sysctl` | `proc-sys` | Working, not yet through the checks |

`linux-user` and `proc-sys` claim no distribution, since the account database and the kernel
parameter namespace are interfaces shared by every target. They still have per-distribution
requirements. `linux-user` needs the shadow utilities, which are absent from minimal images and
replaced by BusyBox equivalents on Alpine, and default id ranges differ. Both are selected where
their requirements are met and skipped with that reason where they are not.

The three entries marked working reconcile and verify but have not been through the checks below
against a named distribution image. Their integration tests run against the real account database
and the real `/proc/sys`, so what is outstanding is the per-distribution matrix work.

The two `Repository` exceptions are fields the underlying tool has no equivalent for, and each is
[refused rather than dropped](../resources/lifecycle.md#when-a-provider-cannot-do-something).
`suite` and `components` describe an apt archive, which has no rpm equivalent. `priority` on an apt
source corresponds to a pin in `apt_preferences`, a separate file with its own matching rules, and
the provider writes no pin.

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
`make test-dnf`, `make test-sysctl`, `make test-user` and `make test-systemd` run them in throwaway
containers. The systemd suite boots systemd as PID 1, since `systemctl` is present in images where
systemd is not running. The sysctl suite is privileged and runs against procfs, whose files have
fixed sizes, cannot be created or removed, and reject values the kernel will not take.

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
