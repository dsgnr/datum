# Reconciliation

Reconciliation is the loop that makes a host match its desired state and keeps it matching. The
[concepts](../concepts/reconciliation.md) section defines the loop and its phases, and the
[architecture](../architecture/reconciliation-flow.md) section follows the data through one pass.
This section covers behaviour that spans passes rather than happening within one.

[Last known good](last-known-good.md)
:   What an agent reconciles when the newest revision fails to resolve, how the same mechanism keeps
    offline hosts working, and why declarative is not the same as reproducible.

More pages will land here as behaviour that is currently an open question becomes settled, in
particular the reconciliation interval and how a failed pass affects the timing of the next one.
