# Datum

Datum continuously reconciles Linux systems against their desired state in Git.

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

Datum is in the design phase.

There is no agent, no CLI, no providers and no reconciler. This repository contains the
documentation that specifies the intended architecture and behaviour, along with the tooling
needed to build it. Configuration formats and command names will change before the first release.

The documentation serves as both a user guide and an engineering specification. Where behaviour is
undecided, it says so instead of describing a guess.

## Previewing the documentation

```bash
make install
make serve
```

The site is then at `http://localhost:8000`.

## Building the documentation

```bash
make build      # build into ./site
make check      # build with --strict, which is what CI runs
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

Product code is premature. That includes the agent, the CLI, providers and the reconciler,
for the reasons in `docs/development/index.md`.

## Licence

[Apache License 2.0](LICENSE).
