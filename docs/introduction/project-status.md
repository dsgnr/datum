# Project status

Datum is in the design phase. There is no agent, no CLI, no provider and no
reconciler. The repository contains this documentation, the tooling needed to
build it, and nothing else.

The specification comes first because the decisions that are expensive to change
later are the ones being made now. The shape of a resource, the way identity
works, where the provider boundary sits and how composition resolves are all
choices that an implementation would otherwise make incidentally, and would then
be stuck with.

## What the documentation is for

This site serves two audiences at once. For somebody evaluating Datum it
explains what the system does and how it is meant to be used. For somebody
implementing Datum it is the specification to build against, which is why pages
describe observable behaviour, failure handling and edge cases instead of
staying at the level of overview.

Where something is undecided, the documentation says so. A specification that
fills in an answer nobody has thought through reads as settled when it is not.

## How this site labels design maturity

| Label | Meaning |
| ----- | ------- |
| **Accepted** | Decided, and recorded in an [architecture decision record](../adr/index.md). Implementation should follow it, and changing it means superseding the record. |
| **Proposed** | A concrete design that has not been accepted. Detailed enough to argue with, likely to change in the detail, and not safe to depend on. |
| **Planned** | Accepted in principle but not specified. It needs to exist and how it behaves has not been worked out. |
| **Open question** | A known gap with no resolution yet. Stated so that it is visible rather than discovered during implementation. |

Nothing on this site is labelled implemented, because nothing is. Once code
exists there will be a support matrix recording what actually works, per resource
type and per distribution, and entries will only appear in it after the behaviour
exists and is tested.

Most configuration examples are proposed. The `datum: v1alpha1` marker at the top of
every document says the same thing more formally, because the alpha suffix means field
names, defaults and semantics can change without a migration path until the schema
reaches a stable version.

## What is settled so far

Eight decisions are accepted, each with a record explaining what it was weighed against
and what it costs.

| Decision | Record |
| -------- | ------ |
| Desired state is expressed as typed resources | [ADR-0001](../adr/0001-typed-resources.md) |
| Resource types are distribution neutral, providers are not | [ADR-0002](../adr/0002-separate-resources-from-providers.md) |
| Git is the source of desired state | [ADR-0003](../adr/0003-git-as-desired-state-source.md) |
| Host identity is separate from host classification | [ADR-0005](../adr/0005-identity-separate-from-classification.md) |
| Dependencies are declared, never inferred | [ADR-0006](../adr/0006-explicit-dependencies.md) |
| The effective manifest is the engine's only input | [ADR-0007](../adr/0007-effective-manifest-as-input.md) |
| Resource references are separate from target identities | [ADR-0008](../adr/0008-resource-reference-and-target-identity.md) |
| Only declared resources are managed | [ADR-0009](../adr/0009-declared-only-ownership.md) |

The reconciliation model itself is settled. Desired state comes from Git, observed state
comes from the host, the two produce a plan, and the plan is applied and verified. The
five phases and their order are not up for negotiation, because everything else in the
design assumes them.

## What is not settled

The [fleet composition model](../adr/0004-labels-and-matchers.md) is proposed,
not accepted. Labels and matchers are the intended mechanism, and the precedence
and conflict rules are written down, but they have not survived contact with a
real repository yet and are expected to move.

Every resource type schema is proposed. The common behaviour they share is close to
settled and the individual field sets are not.

More than forty questions are recorded as unresolved, and they are collected in [open
questions](../development/open-questions.md). Several of them would otherwise be answered
by accident during implementation, which is the main reason the list exists.

## What has to be true before implementation starts

The documentation phase is finished when an engineer who has never seen Datum can
read this site and come away knowing what a resource is, how identity and
dependencies work, how one repository produces per-host desired state, what an
effective manifest contains, where the provider boundary sits, how drift is
detected, how planning differs from applying, what the
[security model](../security/index.md) trusts and does not defend, and which parts
of the design are still open.

Until that holds, improving the specification is more valuable than starting on
the agent.

## Versioning

There are no releases. When there are, the documentation will carry a version
matcher and this page will be replaced by something that records what shipped
rather than what is intended.
