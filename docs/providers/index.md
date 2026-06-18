# Providers

A provider implements one resource type on one class of system. The resource
type defines the state to be described, and the provider implements reading that
state and changing it.

```text
Package
 ├── apt
 ├── dnf
 ├── apk
 └── pacman
```

```text
Service
 └── systemd
```

A `Package` resource describes package state and does not name a provider. The
provider is selected from the host operating system, so the same resource
applies unchanged to a Debian machine and a Fedora one.

## The boundary

Everything above the provider boundary deals in resource types and fields. Everything
below it deals in package managers, init systems and filesystem calls.

| Above the boundary | Below the boundary |
| ------------------ | ------------------ |
| `state: present` | `apt-get install nginx` |
| `mode: "0640"` | `fchmod` on a temporary file before rename |
| `enabled: true` | `systemctl is-enabled nginx` |
| Ordering, conflicts, provenance | Command invocation, output parsing, error mapping |

Components above the boundary do not read `/etc/os-release` or branch on a
distribution. The fleet resolver, graph builder, planner and reconciler are
written once and behave identically on every distribution. A distribution check
above the boundary indicates a missing provider interface.

## What a provider does

Three operations, matching the parts of the lifecycle a provider owns.

Observe
:   Report the current state of the resource's target, using the fields the type
    defines, including whether the target exists. Observation is read-only.

Apply
:   Carry out exactly the action the plan specified.

Verify
:   Reached through the observer rather than implemented separately, so the check uses
    the same reading path as the initial observation.

A provider declares which fields of the type it can observe, which it can set,
and which distributions it supports. Those declarations are part of its
contract.

## Provider constraints

A provider carries out the action the planner selected without re-checking
whether it is needed. A provider that re-checked could disagree with the plan,
and the plan would no longer describe what the pass does.

A provider is called with one resource. Layers, selectors, labels and precedence
are resolved before any provider runs, and a provider has no view of the
manifest the resource came from.

Requests a provider cannot express are reported as errors. A `Package` provider
on a system whose package manager has no version pinning mechanism fails a
request to pin instead of installing the version unpinned.

A provider operates on the target of its own resource. A `Service` provider
asked to restart a unit does not install the package that provides it, which is
either a separate resource with a dependency edge or undeclared.

## One provider per resource

For a given resource on a given host, exactly one provider runs. There is no
composition of providers and no fallback chain that tries `apt` and then `dnf`.

Two providers claiming the same resource on the same host is an error, and
preference order is not used to choose between them.

## The pages

[Provider selection](selection.md)
:   How the provider for a resource is chosen, what `/etc/os-release` can and cannot
    tell Datum, and what happens when no provider matches.

[Multi-distribution design](multi-distribution.md)
:   Differences between distributions, which of them cannot be represented by a single
    field, and how those are documented.

[Support matrix](support-matrix.md)
:   Which resource types and providers are supported on which distributions, and what an entry in
    the matrix has to mean.
