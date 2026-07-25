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

Datum is being built specification first.

Everything that resolves desired state works. A repository can be parsed, a host can be resolved
into an effective manifest, and the result can be validated and explained. Nothing that changes a
machine exists yet, so there are no providers and no reconciler.

| Works | Not yet |
| ----- | ------- |
| `datum render`, `datum explain`, `datum validate` | Observing, diffing, planning, applying, verifying |
| Discovery, matchers, composition, precedence | Providers for any resource type |
| Label substitution, manifest digests | The agent as a resident process |
| The resource graph, cycles, duplicate targets | Secrets and reboots |

Configuration formats and command names will change before the first release. The `v1alpha1` marker
on every document records the schema that document was written against.

The documentation serves as both a user guide and an engineering specification. Where behaviour is
undecided, it says so instead of describing a guess.

## Building

```bash
make build      # build ./bin/datum
make test       # run the Go tests
make lint       # formatting, vet and tests, which is what CI runs
```

Go 1.25 or newer, and no other dependency.

```bash
datum validate --repo path/to/repository
datum render --host web-001 --repo path/to/repository
datum explain 'File[nginx-config]' --host web-001 --repo path/to/repository
```

None of those read or change a managed machine, which is why they are the part that exists.

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

Code that changes a host is still premature. That includes providers, the reconciler and anything
central, for the reasons in `docs/development/index.md`.

## Licence

[Apache License 2.0](LICENSE).
