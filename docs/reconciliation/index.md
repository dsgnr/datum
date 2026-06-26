# Reconciliation

Reconciliation is the loop that makes a host match its desired state and keeps it matching. The
[concepts](../concepts/reconciliation.md) section defines the loop and its phases, and the
[architecture](../architecture/reconciliation-flow.md) section follows the data through one pass.
This section covers behaviour that spans passes rather than happening within one.

[Scheduling](scheduling.md)
:   What makes a pass happen, why the agent is a resident process, the interval, and how a fleet
    avoids reconciling in lockstep.

[One pass at a time](locking.md)
:   The lock that stops a scheduled pass and an operator's `datum reconcile` applying overlapping
    plans to one host.

[Failure, back-off and timeouts](failure-handling.md)
:   What the agent does after a failed pass, why upstream and local failures are timed differently,
    and what bounds a pass that does not finish.

[Last known good](last-known-good.md)
:   What an agent reconciles when the newest revision fails to resolve, how the same mechanism keeps
    offline hosts working, and why declarative is not the same as reproducible.

Between them these answer what a pass does over time, as distinct from what one pass does, which is
the [concepts](../concepts/reconciliation.md) section.
