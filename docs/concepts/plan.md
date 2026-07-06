# Plan

A plan is the ordered set of actions that would resolve the drift found in one
pass. A plan is derived from one desired state, one observation and the
dependencies declared between resources, and is valid only for that combination.

A plan is data, not an execution, so producing one changes nothing on the host.
That is what allows the same code path to serve a preview and a real pass,
rather than the preview being a separate approximation that drifts away from the
thing it is meant to predict.

## Actions

Every resource in the manifest appears in the plan with exactly one action.

| Action | Meaning |
| ------ | ------- |
| `none` | Observed state already satisfies desired state. No provider call is made. |
| `create` | The target does not exist and desired state requires it. |
| `update` | The target exists and one or more fields differ from desired state. |
| `remove` | The target exists and desired state requires its absence. |
| `skip` | The action was not attempted, because a dependency failed or no provider is available. |

Resources with action `none` are included and not filtered out, because a plan
that lists only changes cannot answer the question of whether a resource was
considered. A plan over forty resources reports forty resources, and the summary
line reports how many of each action it contains.

There is no separate action for restarting a service. A service that is already
running and enabled, but whose configuration file changed in this pass, is an
`update` whose reason is the changed dependency:

```text
update   Service[nginx]
         reason   File[nginx-config] changed, restartOn matched
```

Keeping the action set small means the summary of a plan has a fixed shape, and
adding resource types does not add vocabulary that has to be explained.

## What a plan contains

For each resource, a plan records the resource reference, the action, the fields
that differ with their observed and desired values, the provider selected to carry
the action out, the layer the resource came from and the labels that caused it to match, and
the reason if the action is `skip`.

```text
host       web-001
revision   8b91f20
manifest   sha256:3f2a9c4e

update   File[nginx-config]
         path      /etc/nginx/nginx.conf
         provider  file
         mode      0644 -> 0600
         content   differs
         from      roles/web, hosts/web-001

none     Package[nginx]      present, 1.24.0-2

0 to create, 1 to update, 0 to remove, 0 to skip, 13 unchanged
```

!!! note "Proposed output format"

    The rendering is illustrative. The required contents are settled, the layout
    is not, and a machine-readable form will be needed alongside the human one
    before anything can consume plans programmatically.

## Ordering

The order of actions comes from the dependency graph, never from the order
documents appear in a file or the order files appear on disk.

A plan is a total order over the actions it contains, even though the graph only
constrains it partially. Two resources with no dependency relationship have no
required order, and the plan still puts one before the other so that the output
is stable and two runs over the same input produce identical plans. Whether
unrelated actions may then be applied concurrently is a separate question about
the reconciler, not about the plan.

## A plan is not a stored artefact to replay

Reconciliation always builds its own plan from a fresh observation. A plan saved
from an earlier pass is not applied later.

The reason is that a plan encodes an observation, and an observation goes out of
date. Applying a stored plan would act on an observation that is no longer
current, so the plan is rebuilt from the host on every pass.

!!! note "Open question"

    Change control processes often want a reviewed and approved plan to be the
    thing that gets applied, and that conflicts directly with rebuilding the plan
    at apply time. A possible resolution is to apply a freshly built plan only if
    it is equivalent to the approved one, and to fail if it is not, which
    preserves the review without acting on stale data. This has not been designed.

## An empty plan

A plan where every action is `none` means the host is converged. The pass
completes without invoking any provider to make a change.

This is the expected result of most passes, and it is the observable definition
of idempotence. A second pass immediately after a successful one produces an
empty plan, and a resource type whose second pass does not is a resource type
with a bug, not a special case to document.
