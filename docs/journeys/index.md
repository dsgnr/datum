# Journeys

Five scenarios followed end to end, from a commit through resolution, validation, provider selection,
observation, planning, applying, verification and status, to the next pass.

These exist as a test of the specification, not as a tutorial. A model that reads well in isolation
can still fail to answer a concrete question, and walking a scenario the whole way through is the
cheapest way to find that out before there is code. Each page ends with what the journey tests, and
several of them end by naming something the design does not do.

[One Ubuntu server from Git](first-host.md)
:   The smallest useful deployment. Adoption of an existing machine, starting in `observe` mode, and what
    the first plan is actually for.

[500 mixed hosts](mixed-fleet.md)
:   Composition across environments, sites and roles, where distribution differences are allowed to
    surface, and the Alpine gap.

[Changing sshd configuration across production](production-change.md)
:   A change that locks out an estate if it is wrong. Why verification is a distinct phase, and the
    absence of any staged rollout.

[Somebody edits a managed file](manual-change.md)
:   The same drift handled two ways, and why the reconciliation mode is a host-local decision.

[A bad commit reaches main](bad-commit.md)
:   Validation failing before the host is read, last known good keeping the fleet working, and the
    difference between malformed and mistaken desired state.

## What the journeys exposed

Writing them changed the specification rather than only describing it, which was the point.

Three gaps came out of the exercise, and two of them have since been closed by the work they prompted.
Staging a change across a fleet is now [ring branches](../reconciliation/staged-rollout.md), and five
hundred agents reaching one Git remote is now a
[deterministically spread schedule](../reconciliation/scheduling.md#passes-are-spread-deterministically).
The third stands, because a partially-supported distribution is visibly partial, which is correct and
still leaves the Alpine hosts unable to manage services at all.

The journeys also confirmed three things the design gets right. Drift from a human and drift from a
commit are genuinely the same measurement with no special case. Validation failing before the host is
read means a bad commit costs nothing on the host. And verification reading state back is what
distinguishes a restart that worked from a service that is running, which in the sshd case is the
difference between a working estate and an unreachable one.
