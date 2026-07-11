# Testing

Datum changes operating systems as root, so a test suite that only proves the code compiles and the
YAML parses proves very little. This page records what is tested where, what a supported
distribution has been through, and what blocks a release.

!!! note "Proposed design"

    Nothing is implemented, so nothing here is running. The levels, the matrix and the gating are
    proposed, and they are written now because the [support
    matrix](../providers/support-matrix.md) already promises that entries appear only after
    behaviour is tested, and that promise needs a definition.

## The levels

| Level | Runs against | Answers | Speed |
| ----- | ------------ | ------- | ----- |
| Unit | Nothing external | Does resolution, merging, ordering and diffing behave? | Milliseconds |
| Integration | A real filesystem and real tooling, one provider at a time | Does this provider observe and apply correctly? | Seconds |
| System | A whole host, container or VM | Does a full pass converge, and is it idempotent? | Tens of seconds |
| End to end | A Git remote and several hosts | Does a commit reach hosts and converge them? | Minutes |

The level a test belongs at is decided by what it needs, not by what it is about. A test of
precedence ordering needs no host and is a unit test even though precedence is a fleet concept. A
test that `apt` reports an installed version correctly needs `apt` and a package database, so it is
an integration test even though it covers two lines of code.

**Unit tests** cover everything above the [provider
boundary](../providers/index.md#the-boundary), which is most of Datum. Document parsing, matcher
evaluation, layer merging, precedence, conflict detection, graph construction, cycle detection, plan
ordering and action selection are all pure functions from repository content and observed state to a
plan, and all of them can be tested without a host existing.

That is a deliberate consequence of the architecture. Resolution [does not read the
host](../concepts/desired-state.md#resolution-does-not-read-the-host), so the half of Datum most
likely to contain subtle logic errors is the half that needs no infrastructure to test.

**Integration tests** cover one provider against real tooling. The `apt` provider against a real
`apt`, the `File` provider against a real filesystem, the `systemd` provider against a real systemd.
Mocks are not used at this level, because the differences that matter between distributions are in
command output formats and failure modes, which is exactly what a mock gets wrong.

**System tests** run complete passes against a host and assert on the outcome, the plan, and the
state afterwards. This is where idempotence, drift correction, failure containment and verification
are tested.

**End-to-end tests** involve a Git remote and more than one host, and cover the things that only
exist at that scale, such as a commit reaching hosts, revision lag appearing in metrics, and a bad
commit leaving hosts on their [last known
good](../reconciliation/last-known-good.md).

## What containers can and cannot test

Containers are cheap, fast, and genuinely sufficient for a large part of the matrix. They are also
insufficient in ways that matter for exactly the resource types Datum manages.

| Works in a container | Why |
| -------------------- | --- |
| `Package` on every provider | A package manager needs a filesystem and a network, both of which a container has |
| `File` and `Directory` | Ordinary filesystem operations, including the [symlink and hard link cases](../security/provider-safety.md#resolving-a-managed-path) |
| `User` and `Group` | `/etc/passwd` and `/etc/group` are files |
| Manifest validation, resolution, planning | No host interaction at all |
| Malformed desired state | Fails before the host is touched, so the host can be anything |

| Needs a virtual machine | Why |
| ----------------------- | --- |
| `Service` under systemd | systemd as PID 1 with cgroup and dbus access is awkward and unrepresentative in a container |
| `Sysctl` | `/proc/sys` is shared with the host or read-only, and writing it either fails or affects the host |
| Reboots | A container has nothing to reboot |
| Kernel package upgrades | The running kernel is the host's |
| Network configuration | Changing it inside a container does not exercise what it does on a machine |
| Anything asserting the state survives a restart | Requires something that restarts |

The dividing line is whether the resource touches the kernel or the init system. Everything that is
filesystem or package manager work is faithfully testable in a container, and everything that is
kernel or PID 1 work is not.

!!! note "Important limitation"

    A container passing a `Sysctl` test proves nothing, because writing `/proc/sys` in a container
    either fails or changes the host running the test suite. This is the clearest case where a test
    that appears to pass is worse than no test, and `Sysctl` is therefore VM-only in the matrix
    regardless of how much slower that makes it.

## Which runtime for what

| Runtime | Used for |
| ------- | -------- |
| Podman or Docker | Package, File, Directory, User, Group providers across every distribution |
| systemd-nspawn | systemd `Service` tests where a full VM is not warranted |
| Full VMs | Sysctl, reboots, kernel upgrades, network configuration, and the per-distribution acceptance run |

`systemd-nspawn` is the useful middle. It gives a real systemd as PID 1 with a real dbus, which a
plain container does not, at a fraction of a VM's cost, and it covers most `Service` behaviour
including enablement across a container restart.

It does not cover reboots of the host kernel or anything in `/proc/sys`, so it narrows the VM set
without removing it.

## The matrix

Two axes, and they are not multiplied out in full.

| Distribution | Versions | Rationale |
| ------------ | -------- | --------- |
| Debian | Current stable, previous stable | The `apt` reference |
| Ubuntu | Current LTS, previous LTS | Different enough from Debian to need its own row despite `ID_LIKE` |
| Fedora | Current | The `dnf` reference, and moves fastest |
| RHEL or a rebuild | Current major, previous major | Long-lived versions are where old tooling behaviour shows up |
| Alpine | Current stable | The `apk` and OpenRC reference |
| Arch | Rolling | The only row with no `VERSION_ID`, which is itself a case to test |

Rocky Linux and AlmaLinux are represented by one RHEL-family row instead of three, because they
differ from RHEL in branding and `ID`, not in package or service behaviour. If that assumption turns
out to be wrong for a specific provider, that provider's tests gain the extra rows and the whole
suite does not.

Distributions are tested against their actual released images. A minimal official image for each
version, pulled and not built, because the differences that matter are in what the distribution
ships by default, and a locally-built image tests the build instead of the distribution.

## Architectures

`amd64` and `arm64`. Both are ordinary targets now, and `arm64` is where assumptions about paths,
package naming and available versions tend to break.

Other architectures are not tested and therefore not supported. Running a full matrix on more would
cost more than the coverage returns until somebody needs one, at which point that architecture gains
rows instead of being assumed to work.

## Testing specific behaviours

**Idempotence** is tested by running a pass twice and asserting the second plan is empty. That is
the [observable definition](../resources/lifecycle.md#idempotency), not a proxy for it, and every
provider gets the test for every action it supports. A provider that passes on the first run and
produces a non-empty second plan has a normalisation bug, and this test is the only reliable way to
find it.

**Artificial drift** is introduced by changing the host directly and running another pass. Delete
the package, rewrite the file, stop the service, change the mode, then assert the plan contains
exactly the expected action and that the pass converges. Drift is [ordinary
input](../concepts/drift.md), so a drift test is a normal pass against a modified host and nothing
special.

**Partial failures** are produced by making one resource fail on purpose, usually by making a path
unwritable, and asserting three things. The failing resource is `failed`, resources that
[require](../resources/dependencies.md) it are `blocked`, and unrelated resources still converge.
That last assertion is the one that catches an over-eager abort.

**Interrupted reconciliation** is tested by killing the agent mid-pass and running another pass. The
assertion is that the second pass converges, which is the property that matters, because there is no
partially-applied plan to resume and the next pass observes the host and plans from what it finds.
Killing the agent at different points, including between a staged write and its rename, is tested
explicitly.

**Agent crash recovery** is the same test with the agent restarted by its supervisor instead of the
harness, and it additionally asserts that the recorded last-known-good revision survived and that no
lock or temporary file blocks the next pass. Leftover `.datum.tmp` files in target directories are
the expected failure here.

**An unavailable Git remote** is tested by blocking access and asserting the host keeps reconciling
its last known good, reports the failure to fetch, and continues correcting drift. The inverse test
matters as much, and it asserts that the host advances once the remote returns.

**Malformed desired state** covers every row of the [manifest validation
table](../reference/manifest-format.md#validation-summary), and each asserts the same two things,
which are that the error names the offending document and that the host was not touched. This suite is
cheap, runs in containers, and protects the property that a bad commit costs nothing.

**Package upgrades and repository failures** need a controlled package repository, not the
distribution's real one, so the suite hosts its own repository with two versions of a test package.
That makes version pinning, upgrades, downgrades and a repository returning errors or timing out all
testable without depending on the internet or on what upstream happens to be serving.

**Reboots** are tested in VMs by applying a change that requires one, asserting the host reports
[`awaiting-reboot`](../concepts/state.md#reboots), rebooting, and asserting the state afterwards. The
important assertion is that Datum did not reboot the machine itself.

**Network configuration** is tested in VMs with out-of-band access, so that a change which breaks
networking does not disconnect the harness. A VM with a serial console or a second interface reserved
for the harness makes a test that breaks connectivity survivable, which is not possible when the only
access is the thing under test.

**Privileged filesystem operations** are tested in throwaway VMs or containers, never on a
developer's machine or a shared runner. The [path resolution
tests](../security/provider-safety.md#resolving-a-managed-path) deliberately create hostile
conditions, including symlinks pointing at sensitive files and directories swapped mid-operation, and
those tests are the ones that damage a host if the code under test is wrong.

**Unsupported operations** are tested by asserting the right refusal. A `Service` resource on Alpine
with no OpenRC provider must produce `skip` with a reason and a `degraded` host, not a failure and not
a silent success, and that assertion is as important as the ones covering things that work.

## Provider conformance

Every provider passes a common conformance suite before it counts as existing.

The suite is written once against the provider interface and parameterised by provider, so adding a
package provider means running the existing suite rather than writing new tests. It asserts the
behaviour every provider owes regardless of what it manages.

| Conformance requirement |
| ----------------------- |
| Observation reports the target's actual state, including absence, and never desired values |
| Observation changes nothing, asserted by observing a host and diffing it against a snapshot |
| Every action the type defines is implemented, or declared unsupported |
| A second pass after a successful one produces an empty plan |
| Verification detects a discrepancy introduced on purpose |
| A failure is reported as a failure, never as success or a partial change |
| Fields the provider cannot observe are reported unobservable, never guessed |

That suite is what makes the provider boundary real and not aspirational. A second package provider
passing the same tests as the first is the actual evidence that `apt` assumptions did not leak
upward, which is why the [implementation order](../providers/support-matrix.md) puts a second
package provider early.

## What "supported" requires

A [support matrix](../providers/support-matrix.md) entry needs all of the following, on that
distribution at that version, against its released image.

The provider passes the full conformance suite, exercising every action the resource type defines,
and the idempotence test passes. Verification is shown to catch an introduced discrepancy, failures
produce reported errors rather than partial changes, and any field the provider cannot observe is
documented on the resource type page.

Anything short of that is not a support entry. Work in progress, a provider that mostly works, and a
distribution somebody tried by hand all read as unsupported, which is the point of writing the
criteria down before there is pressure to bend them.

## What runs when

| Trigger | Runs | Budget |
| ------- | ---- | ------ |
| Pull request | Unit, integration in containers, malformed-state suite, one distribution's system tests | Minutes |
| Merge to main | Everything above plus system tests across every container-testable distribution | Tens of minutes |
| Nightly | Full matrix including VMs, reboots, sysctl, network, architectures | Hours |
| Release | Full matrix, and it must be green | Hours |

The split is by cost and by likelihood of catching something. A pull request runs everything fast
enough to keep a contributor waiting for, which is most of the logic and one distribution end to
end. The VM matrix runs nightly because it is slow and because the failures it finds are usually
distribution-specific instead of introduced by the change under review.

Release gating is the full matrix passing. A release with a failing VM row ships a provider that
does not work on a distribution the matrix claims, and the matrix is the only thing a user has to go
on.

## Keeping the matrix affordable

Six distributions, ten versions, two architectures and four test levels multiplies into something
nobody will wait for and nothing will pay for.

Three things keep it bounded. Most tests are container tests, which are cheap enough that the
distribution axis barely matters. VM tests run only for resources that need a kernel or an init
system, which is a small subset. And the full matrix runs nightly and at release, not per commit, so
the expensive axis is crossed on a schedule instead of on every push.

!!! note "Open question"

    Where the VM matrix runs is undecided and is the main cost question. Hosted runners with nested
    virtualisation are slow and simple, dedicated hardware is fast and needs maintaining, and cloud
    instances per run are fast and cost money continuously. The decision affects how often the full
    matrix can realistically run, which in turn affects whether nightly is achievable or whether it
    becomes weekly.
