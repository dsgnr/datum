# Provider selection

Selection determines which provider runs for a resource on a given host. The
rule affects what a plan can state and when a failure becomes visible, so it is
specified here rather than left to the implementation.

## Where selection happens

Selection runs after desired state has been resolved and before observation.

```text
effective manifest   (no providers, resolved from the repository alone)
        │
        ▼
provider selection   (reads the host, adds a provider per resource)
        │
        ▼
observation          (each provider reads its resource's target)
```

Resolution does not perform selection, so a manifest can be produced for any
host from a checkout without reaching that machine. Selection is the first step
that reads the host, and it reads only enough to identify the system.

The selected provider appears in the plan alongside each resource, so the choice is visible before
anything is applied.

## What os-release provides

Identification comes from
[`os-release`](https://www.freedesktop.org/software/systemd/man/latest/os-release.html),
read from `/etc/os-release` with `/usr/lib/os-release` as a fallback. The spec is
explicit that the two files should not be combined, so the first one found is the one
used.

| Field | Use | Notes |
| ----- | --- | ----- |
| `ID` | The primary identifier, such as `debian`, `fedora`, `alpine`, `arch`. | Defaults to `linux` if unset. |
| `ID_LIKE` | Fallback when `ID` is not recognised. | Space-separated, ordered closest first, optional. |
| `VERSION_ID` | Distinguishing releases where behaviour differs. | Optional, and unset on rolling releases. |

`ID_LIKE` covers derivative distributions. Ubuntu declares `ID_LIKE=debian`, and
Rocky Linux and AlmaLinux declare identifiers relating them to RHEL, so a
provider supporting `debian` serves Ubuntu without Datum listing every Debian
derivative.

!!! note "Important limitation"

    `VERSION_ID` is optional and absent on rolling releases such as Arch. A
    selection rule requiring a version fails on those systems, so a version can
    narrow a choice but cannot be a precondition for making one.

    `ID_LIKE` is also a claim about being related in packaging and programming
    interfaces, which is weaker than a claim of behavioural equivalence. A derivative
    can diverge in exactly the area a provider cares about while still declaring the
    relationship honestly.

## Options considered

**Static mapping from ID.** A table from `ID` to provider, with `ID_LIKE` as a
fallback. The table has to name distributions that did not exist when it was
written, which `ID_LIKE` reduces without removing.

**Capability probing.** Select `apt` where `apt-get` is present. This handles
unknown distributions without a table, and it selects incorrectly on systems
carrying more than one package manager, which is common on developer machines
and in containers.

**Declared in the repository.** A label or field naming the provider. This is
always correct and places per-distribution configuration in the fleet
repository, which the provider boundary exists to keep out of it.

## Proposed rule

!!! note "Proposed behaviour"

    Selection is proposed and not accepted. The rule below is the current position.

Each provider declares which identifiers it supports.

```text
provider  apt      supports  debian
provider  dnf      supports  fedora, rhel
provider  apk      supports  alpine
provider  pacman   supports  arch
provider  systemd  supports  any host where systemd is the init system
```

For each resource, the candidate providers for its type are those claiming support for
the host's `ID`. If none claim the `ID`, the entries in `ID_LIKE` are tried in order,
which is the order the spec says vendors should list them, closest first.

Exactly one candidate has to remain. Zero candidates means the resource cannot
be reconciled on that host. Two candidates at the same level of specificity is
an error, and no preference order is applied to resolve it.

Probing is used as a check, not as a matcher. A provider chosen from `ID` that
then finds its package manager missing reports that as a failure, which is more
useful than silently selecting a different one.

## When no provider matches

The resource gets the action `skip`, with the reason recorded in the plan.

```text
skip   Package[nginx]
       reason   no Package provider supports ID=nixos, ID_LIKE unset
```

A skipped resource leaves the rest of the manifest to reconcile, and the gap is recorded in the plan
on every pass.

!!! note "Open question"

    Whether `skip` is the right outcome is not settled. A host can remain partly
    converged indefinitely, since a skipped resource resembles an unchanged one
    in a summary. The pass outcome probably needs to reflect skipped resources
    instead of reporting `converged`.

## Overriding selection

There is no mechanism for forcing a provider.

An override would cover systems where identification is wrong and testing a
newer provider on a subset of hosts. It would also place an implementation
detail in per-host configuration, and an optional provider field on every
resource would make the provider boundary advisory.

!!! note "Open question"

    If an override turns out to be necessary, the least damaging form is probably a
    fleet-level statement that certain hosts use a named provider for a named type,
    kept away from resource documents so that resources stay distribution neutral.
    Nothing has been designed.

## Containers

A container's `os-release` describes the image rather than the host running it,
which is the filesystem being reconciled. Container runtimes may expose the
host's file at `/run/host/os-release`, which Datum does not read.

Operations requiring an init system or a writable `/proc/sys` fail in
containers. The provider reports those as failures rather than selection
problems, since the image is the distribution it declares.
