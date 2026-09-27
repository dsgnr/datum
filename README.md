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
fetches its own revision, verifies who signed it, reconciles on its own interval and serves metrics
between passes. A revision it refuses leaves the host on the last one that worked.

Everything above the provider boundary is portable, so most of it is developed and tested without a
host. A provider is the only part that touches an operating system. Reading a host works anywhere.
Applying is Linux-only, since the safety rules the writing path depends on have no portable
equivalent.

| Works | Not yet |
| ----- | ------- |
| `render`, `explain`, `validate`, `affected` | `apk` and `pacman` |
| `observe`, `diff`, `plan` | `trust.strictPaths` |
| `reconcile`, `status` | `--revision` and `--json` |
| `agent`, `config check`, `init`, `revision`, `version` | |
| Fetching, verification, last known good | |
| Refusing resources that target Datum | |
| `Package` through `apt` and `dnf` | |
| `Repository` through `apt` and `dnf` | |
| `Service` through `systemd` | |
| `User`, `Group` through the shadow utilities | |
| `Sysctl` through `/proc/sys` | |
| Discovery, matchers, composition, precedence | Secrets and reboots |
| Label substitution, manifest digests | |
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

Building requires Git, Make and the Go toolchain specified in `go.mod`. The ordinary Go tests
run locally; the provider integration targets above additionally require a running Docker daemon.

Datum runs on Linux, so a build on macOS or Windows is for development only. The binary is
statically linked with cgo disabled, so cross-compiling needs no toolchain beyond Go.

```bash
make build-linux    # linux/amd64 and linux/arm64 into ./bin
make dist           # those two and this machine's
make package        # a .deb and an .rpm into ./dist
make test-package   # install both and check what landed
```

The packages are built by each distribution's own tools in a container, so Docker is required
alongside the build prerequisites above. Installing one puts the binary in `/usr/bin`, a default
configuration naming no host in `/etc/datum`, and a systemd unit that is left disabled. [Installing the
agent](https://getdatum.sh/lifecycle/installation/) covers what lands where.

For a first run, follow the [quickstart](https://getdatum.sh/lifecycle/): inspect the example
fleet, then preview and apply one managed file in a Linux shell. The
[installation guide](https://getdatum.sh/lifecycle/installation/) takes a host from package
installation to a continuously running agent, starting in observe mode.

The [example fleet](examples/README.md) demonstrates composition across three hosts.

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

Start with the [quickstart](https://getdatum.sh/lifecycle/) and installation guide under
Getting started. The User guide covers core concepts, writing desired state and operating agents.
Reference provides command, configuration and resource field lookups. Design holds the architecture,
security model, worked scenarios and development material.

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
