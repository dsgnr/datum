# State and lifecycle

Datum reports state at two levels, a resource within a pass and a host across passes. This page
defines the vocabulary for both.

## Resource state within a pass

A resource moves through a small set of states during one pass. These are finer than the [plan
actions](plan.md#actions), since an action is what the plan intends and a state is what the resource
reached.

| State | Meaning |
| ----- | ------- |
| `pending` | In the plan, not yet reached. |
| `applying` | The provider is acting on it now. |
| `converged` | Observed state satisfies desired state, whether or not this pass changed it. |
| `drifted` | Observed state differs from desired state, and nothing was applied. Only in [`observe` mode](reconciliation-modes.md). |
| `failed` | The provider was asked to act and did not succeed, or verification did not confirm the result. |
| `blocked` | Not attempted, because something it [requires](../resources/dependencies.md) failed or was blocked. |
| `skipped` | Not attempted, because no [provider](../providers/selection.md#when-no-provider-matches) supports it on this host. |

`blocked` and `skipped` are distinct states. A blocked resource is waiting on a failure elsewhere in
the same manifest, so correcting that failure clears it. A skipped resource cannot be reconciled on
this host at all, and retrying does not change that.

`converged` does not indicate whether this pass did any work. A resource already in desired state
and one corrected during the pass both end `converged`. What changed during a pass is carried by the
action.

## Host state across passes

A host has a state summarising its most recent pass, which persists between passes so that a host
can be queried between reconciliations.

| State | Meaning |
| ----- | ------- |
| `converged` | Every resource in the effective manifest is `converged`. |
| `drifted` | At least one resource is `drifted`, none `failed`. Reached only under `observe` mode. |
| `failed` | At least one resource is `failed`. |
| `degraded` | Some resources are `skipped`, the rest `converged`. The host is as converged as it can be and is not fully managed. |
| `awaiting-reboot` | Converged as far as the running system allows, with a change that needs a [reboot](#reboots) to take effect. |
| `unknown` | No pass has completed, or the last report cannot be read. |

`degraded` distinguishes a host carrying unsupported resources from one that is fully converged.
Without it, a host where part of the manifest never applied would report `converged`. A `degraded`
host is correct for everything it can manage and is not managing everything it was asked to.

## Reboots

Some changes take effect only after the machine restarts, a kernel package upgrade being the common
case.

A provider that makes such a change reports that a reboot is required. It does not reboot the
machine.

!!! note "Proposed behaviour"

    Reboot handling is proposed and unimplemented. A provider signals the need and does not act on
    it. A provider running `reboot` during a pass would take the host down while other resources
    were part-applied, with no check on whether the fleet tolerates losing that host.

A pass that applied a change needing a reboot verifies everything it can, records the outcome, and
leaves the host in `awaiting-reboot`.

```text
Package[linux-image]   updated
Service[nginx]         converged
File[sysctl.conf]      converged

host state: awaiting-reboot
reason:     Package[linux-image] updated, running kernel differs
```

The state separates converged from current. A host in `awaiting-reboot` matches its desired state in
everything checkable without restarting, and it is not yet running that state.

### Reboot policy

Whether and when the reboot happens is a policy decision, separate from the reconciliation that
established the need.

```text
never            report and wait for a human
manual           a human triggers it, Datum tracks that it is needed
maintenance      reboot only within a defined window
automatic        reboot as soon as it is needed
```

!!! note "Open question"

    Where reboot policy lives, and which of these options exist, is undecided. Rebooting hosts in
    a controlled order so that a service stays available is cross-host sequencing, which
    [Datum does not do](../architecture/deployment-models.md). An agent acting alone can support
    `never` and `manual` without further machinery, and `automatic` needs a decision about what
    happens when several hosts reach it at once.

    Reboot policy is host or fleet configuration rather than desired state, as [reconciliation
    mode](reconciliation-modes.md#where-the-mode-is-set) is. Whether a machine may restart itself is
    local to that machine.

## Why the states are separate

The five host states call for different responses, so a single `ok` or `error` would not carry
enough to choose one. A host that has drifted, one that failed, one that cannot be fully managed and
one waiting to reboot are distinct situations.

Every component has to preserve enough information to report each state. The [status
model](../reference/status.md) consumes it.
