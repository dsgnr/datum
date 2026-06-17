# Creating a repository

A Datum repository is a Git repository containing documents. There is no database to initialise, no
server to register with, and no state outside the files.

## The minimum

A repository that reconciles needs four documents, and those four documents can be spread across two
files or collapsed into one.

```yaml title="fleet/datum.yaml"
datum: v1alpha1
type: Fleet

name: example
---
datum: v1alpha1
type: Layer

name: base

precedence: 0
```

```yaml title="fleet/hosts/web-001.yaml"
datum: v1alpha1
type: Host

name: web-001

labels:
  role: web
---
datum: v1alpha1
type: Package

name: curl

desired:
  state: present
```

A repository holding those four documents reconciles as it stands. The `Fleet` document marks the root
and the `Layer` with no matcher applies to every host. The `Host` document declares a machine, and the
`Package` resource belongs to the `base` layer by virtue of it being the [nearest `Layer`
document](../fleet/repository-layout.md#layer) above it in the tree.

## Datum requires no directory structure

Discovery walks the fleet root and dispatches on each document's `type`, so paths carry exactly one
piece of meaning, which is that a resource document belongs to the nearest `Layer` document at or
above it.

Beyond that single rule the layout is convention. The names `base`, `environments`, `sites`, `roles`
and `hosts` come from the [conventional layout](../fleet/repository-layout.md) and Datum attaches no
meaning to any of them. A repository that calls them `common`, `tiers` and `machines` behaves
identically, as does one that puts every document in a single file.

That freedom is deliberate, because hard-coded directory semantics would mean the layout could not be
changed without changing behaviour, leaving a repository stuck with whatever shape it happened to start
with.

The nearest-`Layer` rule is what sets the practical limit. Putting every document in one file means
every resource belongs to the same layer, which is workable for a single host and not for a fleet.
Structure therefore emerges from wanting different matchers, not from Datum insisting on it.

## datum init

!!! note "Proposed behaviour"

    `datum init` does not exist yet, and what it creates is specified here because a generated
    repository is the first thing a new user reads, so a cluttered one teaches the wrong habits from
    the beginning.

```text
$ datum init

created fleet/datum.yaml
created fleet/base/layer.yaml

next steps
  add a Host document under fleet/hosts/
  add resources under fleet/base/ or a new layer
```

The command creates two files and nothing beyond them.

```yaml title="fleet/datum.yaml"
# Marks the root of the fleet. Everything below this directory is searched
# for Host, Layer and resource documents.
datum: v1alpha1
type: Fleet

name: example
```

```yaml title="fleet/base/layer.yaml"
# A layer with no matcher applies to every host in the fleet.
# Resource documents in this directory belong to this layer.
datum: v1alpha1
type: Layer

name: base

precedence: 0
```

There is no example host, because a host name has to correspond to a real machine and a placeholder
either gets committed by accident or gets deleted immediately. There are no example resources for
the same reason, compounded by the tendency of generated examples to be copied rather than
understood.

The comments in both files describe the format and not the reasoning behind it. There are two lines
per file, placed in files the user is about to edit, covering the one thing that is not obvious from
the content itself.

`datum init --with-examples` adds a commented example host and resource for anybody who wants them,
which keeps the default output clean while putting the fuller starting point one flag away.

## datum init does not touch Git

The command creates Datum configuration in the current directory and does not run `git init`.

Adding Datum to an existing repository is at least as common as starting a new one, and a tool that
unexpectedly creates a Git repository inside another one causes a confusing and time-consuming mess. It
does warn when the current directory is not inside a Git work tree. Desired state has to be committed
before it can be reconciled, and a repository that is never committed to has no effect on any host.

## Template repositories and datum init

Both a template repository and an initialising command have a place, and the jobs they do are
different.

`datum init` is the supported way a repository is created. It ships with the agent, so what it
generates always matches the schema version that agent understands.

A template repository serves as an example, not an initialiser. It carries the things `init` should
not generate, meaning a CI workflow, a realistic multi-layer structure, and a worked set of hosts
and roles. It is intended to be read or forked instead of treated as the supported path to a new
repository.

Which of the two is authoritative matters, because two mechanisms generating repository structure means
two definitions of a correct repository. Those two definitions diverge the first time the schema
changes.

## Keeping a repository from accumulating unused content

The generated repository is two files because everything beyond that grows from use. The risks that
follow are a repository accumulating layers nobody matches and resources nobody needs, and both of
those are visible in the repository rather than hidden inside it.

A layer that matches no host contributes nothing to any manifest and should be reported, since an
unmatched layer is more often a matcher typo than an intention.

!!! note "Open question"

    Whether `datum validate` should report layers matching no host in the fleet is undecided. It is
    a useful signal and it is also legitimate during a rollout, where a layer is added before the
    hosts it targets exist, so reporting it as an error would be wrong and reporting nothing loses
    the most common matcher mistake.
