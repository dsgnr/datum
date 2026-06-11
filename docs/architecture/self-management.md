# Managing Datum with Datum

Datum runs on a host, and the agent is software on that host, so a repository will eventually
describe the agent as one of the things it manages.

```text
Datum
  │
  └── manages Datum
```

That loop needs acknowledging before implementation, because the failure it introduces is one an
otherwise-correct reconciler walks straight into.

## The problem

Suppose a manifest declares the agent's own package.

```yaml
datum: v1alpha1
type: Package

name: datum-agent

desired:
  version: "1.5.0"
```

The running agent, at version 1.4, resolves this, plans an update, and applies it. Applying it
replaces the binary that is currently executing. Depending on how the update is performed, the
process reconciling the rest of the manifest can be killed partway through, leaving the pass
half-applied and possibly leaving no agent running to notice or retry.

This is not hypothetical for a self-managing tool. It is the ordinary case the first time someone
adds the agent to its own repository.

## The boundary

There is a boundary between the agent as a reconciler and the agent as a managed resource, and the
two need different handling.

Reconciling everything else on the host is ordinary work. Reconciling the agent itself is a
bootstrap operation that cannot be performed by the process it replaces, in the same way a
[trust anchor](../adr/0010-no-self-managed-trust-anchors.md) cannot be managed by the thing it
protects.

!!! note "Open question"

    How self-update works is undecided. What is firm is that the running reconciler does not replace
    its own binary mid-pass, because a process cannot safely overwrite itself while executing and
    cannot retry an update that killed it.

The plausible mechanisms each move the replacement outside the running process.

A supervisor
:   A small, rarely-changing process starts the agent, and the agent asks the supervisor to perform an update between passes rather than during one. The supervisor is then the thing that must not
    manage itself.

External package management
:   The agent's own package is excluded from what Datum manages, and the platform's normal update
    mechanism handles it. Simple, and it means the agent is not actually self-managing, which for
    many fleets is the right answer.

Atomic replacement and restart
:   The new binary is installed alongside the old, and a restart, ordered by the init system and not by the reconciler, switches to it between passes. This leans on the same
    [reboot and restart](../concepts/state.md#reboots) handling that already keeps providers from
    restarting things mid-pass.

## What the design commits to now

Only the boundary, not the mechanism. A resource that targets the agent's own binary, package or
configuration is a self-management operation, and the reconciler does not perform it inline the way
it performs an ordinary change.

The narrower case of the agent's [trust anchors](../adr/0010-no-self-managed-trust-anchors.md) is
already decided, and those are refused outright. The agent's binary and package are a softer version
of the same principle, where the operation is permitted but has to happen outside the running
reconciler instead of being forbidden.

Recording the boundary now keeps a later self-update design from having to unpick an assumption that
the reconciler can change anything on the host including itself. It cannot, and the one exception is
itself.
