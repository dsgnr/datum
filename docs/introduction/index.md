---
description: "Learn what Datum manages, how Linux hosts reconcile desired state from Git, and where to begin with installation, fleet authoring and operations."
seo_title: "Git-based Linux configuration management - Datum"
---

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

Datum implements nine [resource types](../resources/types/index.md): `Package`, `File`,
`Directory`, `Symlink`, `Service`, `User`, `Group`, `Sysctl` and `Repository`. Provider support
varies by type and platform; the [support matrix](../providers/support-matrix.md) shows what runs
where.

The set is small because the shared behaviour has to be settled first. Identity, how state
is read back, how resources order themselves against each other and how a change is
verified are all inherited by every type added later. The
[resources](../resources/index.md) section specifies that behaviour.

## Providers

A `Package` resource describes package state and does not mean `apt`. Which package manager
realises it is a property of the host. `apt` and `dnf` providers are implemented; `apk` and
`pacman` support is planned. `Service` currently has the `systemd` provider.

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
[drift](../concepts/index.md#drift) ordinary input. A machine somebody edited by hand is not an
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

Secret values should not sit in the repository, since every host can read everything committed
there. The resource model accepts a
[secret reference](../resources/secrets.md), but resolving references is not implemented yet;
Datum does not store or serve secrets.

## Security

An agent running as root and taking instructions from a repository several people can write
to is the central security concern. The [security](../security/index.md) section explains the
trust model and threat scenarios; its implementation-status notes distinguish working controls
from proposed ones.

Two things need saying before reading further. Anyone who can merge to the tracked branch can run
configuration as root on every host their change matches, and that is a trust assumption, not
something Datum defends against. A host reports its own status, so a converged fleet report is a
claim made by the hosts, not evidence about them.

## Choose a path

[Why Datum?](why-datum.md) explains the problem it addresses. [How Datum works](how-datum-works.md)
follows one change through reconciliation, and [project status](project-status.md) separates
implemented behaviour from work still underway.

[Follow a worked scenario](../journeys/first-host.md)
:   See an existing Ubuntu server adopted in observe mode, from repository documents through the
    first plan and steady state. The [journeys index](../journeys/index.md) has scenarios for
    mixed fleets, production changes, manual drift and a bad commit.

[Learn the model](../concepts/index.md)
:   Start with desired and observed state, then follow the path through drift, planning and
    reconciliation. Continue to [fleet composition](../fleet/index.md) and
    [resource behaviour](../resources/index.md).

[Try Datum](../lifecycle/index.md)
:   Start with the quickstart, then [install and run](../lifecycle/installation.md) the agent, then use the
    [CLI reference](../reference/cli.md) and the sections on
    [security](../security/index.md), [reconciliation](../reconciliation/index.md) and
    [observability](../observability/index.md).

[Contribute to Datum](../development/index.md)
:   Check [project status](project-status.md), review the [open questions](../development/open-questions.md),
    then see the [development guide](../development/index.md) and
    [architecture decisions](../adr/index.md).
