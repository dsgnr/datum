# ADR-0014: Go as the implementation language

Status: Accepted

## Context

The agent runs as root on every managed host, on several Linux distributions, and has to be
installable on a machine that has nothing else on it. That last requirement does most of the
deciding.

An interpreted implementation would need its runtime present before Datum could run, which makes the
runtime a dependency the agent cannot install through desired state, since installing it is what
Datum would be there to do. Adding a package manager and an interpreter to the trusted base of every
host also widens the [supply chain](../security/index.md#what-is-not-defended) considerably.

The other hard requirement comes from [applying state safely](../security/provider-safety.md). The
filesystem rules need `openat2`, `O_NOFOLLOW`, `fchown` and `fchmod` on a descriptor, `renameat`,
`unlinkat` and `flock`, used directly rather than through a convenience layer that re-resolves a
path. A language that hides the distinction between a path and a descriptor cannot express those
rules at all.

## Decision

The agent, the command line and the providers are written in Go.

A single statically linked binary with no runtime dependency satisfies the installation requirement.
`golang.org/x/sys/unix` exposes the syscalls the provider rules need, including `openat2`.
Cross-compiling for the architectures in the target list is part of the toolchain, not a separate
exercise.

The standard library covers most of what is needed, which keeps the dependency list short. That
matters more here than usual, because every dependency is code running as root on every host.

## Consequences

Go's error handling suits a tool whose job is to report precisely what went wrong and where. The
verbosity is a cost paid in exchange for failures that are hard to overlook, which is the right
trade for something that changes machines as root.

The absence of a strong type system for desired state is the notable cost. Resource types are
validated at runtime against documents instead of checked at compile time, so the field rules are
code and tests, not a schema the compiler enforces.

Garbage collection is not a problem for this workload. A pass is short, is bounded by [a
timeout](../reconciliation/failure-handling.md#a-pass-is-bounded), and spends most of its time
waiting on a package manager instead of allocating.

Providers being compiled into the agent means adding a resource type is a rebuild. That is
consistent with [extensions](../resources/applications.md#extensions) being out-of-process, since the
extension protocol is what makes a provider addable without one, and it means the shipped providers
have no plugin loading path to get wrong.
