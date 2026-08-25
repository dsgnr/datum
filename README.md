# Datum

Datum continuously reconciles Linux systems against their desired state in Git.

**[Documentation](https://getdatum.sh/)**

A repository describes the state each machine should be in, covering packages, file contents and
permissions, services, users, groups and kernel parameters. Datum reads that description, inspects
the machine, and changes only what differs. Reconciliation repeats, so a file edited by hand or
replaced during a package upgrade is corrected on a later pass.

One repository can describe a single machine or several thousand, running more than one Linux
distribution.

## Why this exists

Datum started as a learning exercise. I wanted a lifecycle machine built end to end, from reading a
repository through to changing a host and checking the change afterwards, and I wanted the whole of
it specified before any of it was written.

It is also a project I find interesting in its own right. The influences are modern, taking the
reconciliation loop and declarative state that cluster tooling settled on and pointing them at
ordinary Linux hosts.

The third reason is that the approach looks usable in an enterprise. NixOS solves a similar problem
more thoroughly, and a fleet whose support contract names Ubuntu or RHEL cannot adopt it. Datum
works on the distributions those contracts already cover and leaves the operating system as it is.

## Status

Datum is being built specification first.

A complete pass runs end to end. A repository can be parsed, a host resolved into an effective
manifest, that manifest compared against what is actually on a machine, and the resulting plan
applied and verified. All nine resource types have a provider, and the agent runs as a service that
reconciles on its own interval and serves metrics between passes.

Everything above the provider boundary is portable, so most of it is developed and tested without a
host. A provider is the only part that touches an operating system. Reading a host works anywhere.
Applying is Linux-only, since the safety rules the writing path depends on have no portable
equivalent.

| Works | Not yet |
| ----- | ------- |
| `render`, `explain`, `validate`, `affected` | `apk` and `pacman` |
| `observe`, `diff`, `plan` | Fetching from a remote |
| `reconcile`, `status` | Signature verification, last known good |
| `agent`, `config check` | |
| `Package` through `apt` and `dnf` | |
| `Repository` through `apt` and `dnf` | |
| `Service` through `systemd` | |
| `User`, `Group` through the shadow utilities | |
| `Sysctl` through `/proc/sys` | |
| Discovery, matchers, composition, precedence | Secrets and reboots |
| Label substitution, manifest digests | `init` and `migrate` |
| The resource graph, cycles, duplicate targets | |
| Per-type field validation | |
| Applying, verification, failure propagation | |
| The pass lock and pass reports | |
| A provider for `File`, `Directory` and `Symlink` | |

Configuration formats and command names will change before the first release. The `v1alpha1` marker
on every document records the schema that document was written against.

The documentation serves as both a user guide and an engineering specification. Where behaviour is
undecided, it says so instead of describing a guess.

## Building

```bash
make build      # build ./bin/datum for this machine
make test       # run the Go tests
make test-apt   # run the apt provider against a real Debian container
make test-dnf   # run the dnf provider against a real Fedora container
make test-sysctl   # run the sysctl provider against a real kernel
make test-user  # run the user provider against a real account database
make test-systemd  # run the systemd provider against a booted systemd
make lint       # formatting, vet and tests, which is what CI runs
```

Go 1.25 or newer, and no other dependency.

Datum runs on Linux, so a build on macOS or Windows is for development only. The binary is
statically linked with cgo disabled, so cross-compiling needs no toolchain beyond Go.

```bash
make build-linux    # linux/amd64 and linux/arm64 into ./bin
make dist           # those two and this machine's
```

`make shell` builds for whatever architecture Docker reports and opens an Ubuntu container with
the binary and the example fleet already mounted, which is the quickest way to run it on Linux
from a machine that is not.

```bash
./bin/datum validate --repo examples/fleet
./bin/datum render --host web-001 --repo examples/fleet
./bin/datum explain 'File[nginx-config]' --host web-001 --repo examples/fleet
./bin/datum affected --from HEAD~1 --to HEAD --repo examples/fleet
```

Those four read repository content only. The next three read a machine and change nothing on it.

```bash
./bin/datum observe --host web-001 --repo examples/fleet
./bin/datum diff    --host web-001 --repo examples/fleet
./bin/datum plan    --host web-001 --repo examples/fleet
```

`diff` and `plan` exit 2 when something differs, so either works as a drift check in a scheduled
job without parsing the output.

Every command above reads and changes nothing, so any of them is safe to run on a host in the middle
of an incident.

`reconcile` is the one that writes, and it needs Linux and enough privilege to change the targets
the manifest names.

```bash
./bin/datum reconcile --host web-001 --repo examples/fleet --state /var/lib/datum
./bin/datum status --state /var/lib/datum
```

`--mode observe` runs the same observation and the same diff and applies none of it, which produces
a drift report. The state directory has to be mode 0700. A pass refuses to run otherwise and does
not correct the mode, since a widened directory discloses the plans already written there.

`examples/fleet` is a three-host repository to try the commands against, described in
[examples/README.md](examples/README.md).

## Previewing the documentation

```bash
make docs-install
make docs-serve
```

The site is then served locally on port 8000.

```bash
make docs-build      # build into ./site
make docs-check      # build with --strict, which is what CI runs
```

Strict mode fails on broken internal links and unknown heading anchors, so a merged change cannot
leave the site broken. The toolchain is [Zensical](https://zensical.org/), pinned in
`requirements-docs.txt`.

## Where to start

Read the site in order. The introduction covers what Datum manages and how one repository maps
onto many machines, and the sections after it work through the vocabulary, the fleet model,
resources, providers, the architecture and the security model.

For the current state of the design, three places are the most useful entry points. `docs/adr/`
records the decisions that are settled and why, `docs/development/open-questions.md` lists
everything that is not, and `docs/security/` states what is being trusted and what is not
defended.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The most useful contributions at this stage are arguments
against a decision, cases the model cannot express, and answers to open questions.

A change that makes a statement in `docs/` false is not finished until the statement is fixed. The
documentation is the specification, so the two drifting apart is a defect in both.

## Licence

[Apache License 2.0](LICENSE).
