# Reconciliation modes

Detecting drift and correcting it are separate operations, which the design has treated as a
principle from the start. A mode makes that separation an operating choice rather than a matter of
which command someone runs.

```text
observe
   │
   ├── report drift
   │
   └── change nothing

enforce
   │
   ├── report drift
   └── reconcile it
```

!!! note "Proposed behaviour"

    Modes are proposed. The two named here are concrete, per-resource policy is sketched, and
    nothing is implemented.

## The two modes

| Mode | Observes and diffs | Applies |
| ---- | ------------------ | ------- |
| `observe` | Yes | No |
| `enforce` | Yes | Yes |

Both run the same observation and the same diff, so the drift an `observe`-mode pass reports is
exactly the drift an `enforce`-mode pass would act on. The report is not an approximation of what
enforcement would find, because it is produced by the same code path, which is the property that
makes `observe` mode trustworthy as a preview of enforcement.

`observe` mode stops after the [plan](plan.md). A pass in `observe` mode builds the full ordered
plan and reports it without applying any action, so the host's state is unchanged.

## Why this matters for adoption

A team evaluating Datum can run it with root access without it changing anything. A fleet can run
entirely in `observe` mode first, which turns Datum into a drift detector that reports how far each
host has diverged from its intended state without changing anything.

Observe mode answers how much of the estate matches the repository without changing any host, which
is the usual first step when adopting existing machines.

The progression is deliberate. Run `observe` across the fleet, read the drift, correct the
repository until the reported drift is what you expect and not a surprise, then move hosts to
`enforce`.

## Where the mode is set

The mode is [agent configuration](../reference/agent-config.md#reconciliation), set locally and not
in the repository, alongside the [trust
settings](../security/repository-trust.md#agent-configuration).

```yaml title="/etc/datum/agent.yaml"
host: web-001

reconciliation:
  mode: observe
```

Placing it on the host instead of in desired state is deliberate. The mode is a statement about how
much a particular machine trusts Datum, which is an operational decision local to that machine, and
putting it in the repository would let a commit switch a host from observing to enforcing without
anyone touching the host.

That mirrors the reasoning for [trust anchors](../adr/0010-no-self-managed-trust-anchors.md), where
a control deciding whether Datum may change a machine should not itself be changeable by the thing
being controlled.

## Per-resource policy

A whole-host mode is coarse. Some drift on an otherwise enforced host is expected and should be
reported rather than corrected, a file a local process rewrites at runtime being the common case.

!!! note "Open question"

    Whether individual resources can override the host mode, so that a mostly-enforcing host can
    leave certain resources in `observe`, is undecided. It would require defining what a host's
    status means when some resources are enforced and others are only observed. A single host-level
    `converged` no longer captures that, which ties this question to the [status
    model](../reference/status.md).

## What a mode does not change

A mode changes whether the reconciler applies, and nothing before that. Resolution, provider
selection, observation and planning are identical in both modes, so a host in `observe` mode
does the same work and reads the same state as one in `enforce` mode, right up to the point of
acting.

The definition of drift is the same in both modes. An `observe`-mode host that has diverged is
drifted, not converged, and its [status](../reference/status.md) says so. Reporting drift without
correcting it is not the same as being in the desired state, so a host with reported drift is
`drifted` rather than [converged](reconciliation.md#convergence).
