# Introduction

Everything Datum manages is a resource, a typed description of one thing on a
host. A resource states the condition that thing should be in and carries no
instruction for how to reach it.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Package

metadata:
  name: nginx

spec:
  state: present
```

There are seven resource types proposed for the first implementation: `Package`,
`File`, `Directory`, `Service`, `User`, `Group` and `Sysctl`. The set is small
because the behaviour every resource shares has to be settled first. How a
resource is identified, how its state is read back from the host, how it orders
itself against other resources and how a change is verified are all decisions
that every type added later inherits, so getting those right matters more than
accumulating types.

## Providers

A `Package` resource describes package state and does not mean `apt`. Which package manager
realises it is a property of the host, so `apt`, `dnf`, `apk` and `pacman` sit behind one
resource type as providers. `Service` has one candidate provider, `systemd`.

Provider names stay out of resource documents. Selection is derived from the host, mostly by
reading `/etc/os-release`. Once a distribution check is allowed into the resource model it
turns up in the planner and the fleet configuration as well, and none of those should hold
an opinion about packaging.

Providers will not agree with each other, and there is no pretence otherwise.
Holding a package at a particular version is a different mechanism on each of
the four, and on some of them it needs an additional plugin installed before it
exists at all. Whether the package manager can report the exact file list for an
installed package varies too. Where a difference will not sit behind a single
resource field, the difference is documented on the resource type and on the
provider that implements it.

## Hosts, labels and selectors

A repository does not hold one file per machine. It holds layers of
configuration, each carrying a selector that says which hosts the layer applies
to, together with one `Host` document per machine that supplies its identity and
its labels.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Host

metadata:
  name: web-001
  labels:
    environment: production
    site: london
    role: web
    architecture: amd64
```

A layer whose selector matches `role: web` applies to `web-001`, and so does a
layer matching `environment: production`, along with every other production
host. Gathering every layer that matches and resolving them in a defined order
produces the effective manifest for that host, which is the full set of
resources to be reconciled there. Provenance is kept through that resolution, so
that the question of which layer contributed a resource, and why its selector
matched, has an answer.

Adding a machine to the fleet should therefore amount to a `Host` document with
the right labels and nothing else, because configuration is reused by matching
rather than by duplication.

## The reconciliation phases

Reconciliation has five phases. Each answers a different question, and stopping after any
one of them is useful on its own.

1. **Observe** reads the current state of every resource in the effective
   manifest from the host.
2. **Diff** compares observed state against desired state, resource by
   resource, and records what differs.
3. **Plan** turns those differences into an ordered set of actions.
4. **Apply** carries the actions out.
5. **Verify** reads the affected resources again and confirms that they hold the
   state that was asked for.

Observation happens both before any decision is taken and after any change is
made, which is what allows drift to be treated as ordinary input. A machine
somebody has edited by hand is not an error for Datum to report, it is a machine
that produces a non-empty plan on the next pass and converges again, so nothing
in the engine needs a special case for it.

## Scope

Datum takes a description of state as its input, and there is no resource type
that runs a command, nor any facility for executing ad-hoc commands across a
fleet. A resource whose state cannot be read back from the host cannot be diffed
or verified, which removes most of the reason for reconciling it, and it is the
route by which configuration management tools tend to acquire a shell-script
escape hatch.

Sequencing work across hosts is outside what Datum does. Each host reconciles
independently, so an instruction to take a machine out of a load balancer before
upgrading it cannot be expressed. Provisioning machines and building images sit
outside Datum as well.

Secret material is a gap rather than a decision. Configuration files frequently
need credentials in them, and there is no answer yet for how they get there.

## Reading on

[Why Datum?](why-datum.md) sets out the problems this design is responding to,
and [how Datum works](how-datum-works.md) follows a single change through the
whole pipeline. [Project status](project-status.md) explains how this site
distinguishes settled decisions from open proposals.
