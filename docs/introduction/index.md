# Introduction

Everything Datum manages is a resource, a typed description of one thing on a
host. A resource states the condition that thing should be in and carries no
instruction for how to reach it.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
```

There are seven [resource types](../resources/types/index.md) proposed for the first
implementation: `Package`, `File`, `Directory`, `Service`, `User`, `Group` and `Sysctl`.
The set is small because the behaviour every resource shares has to be settled first. How
a resource is identified, how its state is read back from the host, how it orders itself
against other resources and how a change is verified are all decisions that every type
added later inherits, so getting those right matters more than accumulating types. The
[resources](../resources/index.md) section specifies that shared behaviour.

## Providers

A `Package` resource describes package state and does not mean `apt`. Which package manager
realises it is a property of the host, so `apt`, `dnf`, `apk` and `pacman` sit behind one
resource type as providers. `Service` has one candidate provider, `systemd`.

Provider names stay out of resource documents. Selection is derived from the host, mostly by
reading `/etc/os-release`. Once a distribution check is allowed into the resource model it
turns up in the planner and the fleet configuration as well, and none of those should hold
an opinion about packaging.

Providers do not agree with each other. Holding a package at a version is a different
mechanism on each of the four, and on some it needs an extra plugin installed first.
Whether the package manager can report the file list for an installed package varies too.

Differences that will not sit behind a single resource field are documented on the resource type and
on the provider that implements it. The [providers](../providers/index.md) section covers the
boundary, provider selection, and the differences that cannot be hidden.

## Hosts, labels and matchers

A repository does not hold one file per machine. It holds layers of
configuration, each carrying a matcher that says which hosts the layer applies
to, together with one `Host` document per machine that supplies its identity and
its labels.

```yaml
datum: v1alpha1
type: Host

name: web-001
labels:
  environment: production
  site: london
  role: web
  architecture: amd64
```

A layer matching `role: web` applies to `web-001`, as does one matching `environment: production`
along with every other production host. Resolving every matching layer in a defined order produces
the effective manifest, the full set of resources to reconcile there. Provenance survives
resolution, so which layer contributed a resource, and why its matcher matched, both have answers.

Adding a machine to the fleet is a `Host` document with the right labels and nothing else.
The [fleet](../fleet/index.md) section specifies repository layout, matcher semantics,
precedence and conflict handling.

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

Observation happens before any decision and again after any change, which makes
[drift](../concepts/drift.md) ordinary input. A machine somebody edited by hand is not an
error to report. It produces a non-empty plan on the next pass and converges, so the engine
needs no special case for it.

[Concepts](../concepts/index.md) defines each phase and establishes the vocabulary the rest
of this site uses.

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
whole pipeline. [Design principles](design-principles.md) records the constraints
the design is held to, and [project status](project-status.md) explains how this
site distinguishes settled decisions from open proposals.
