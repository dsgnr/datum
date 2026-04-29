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
| **Accepted** | Decided, and recorded in an architecture decision record. Implementation should follow it, and changing it means superseding the record. |
| **Proposed** | A concrete design that has not been accepted. Detailed enough to argue with, likely to change in the detail, and not safe to depend on. |
| **Planned** | Accepted in principle but not specified. It needs to exist and how it behaves has not been worked out. |
| **Open question** | A known gap with no resolution yet. Stated so that it is visible rather than discovered during implementation. |

Nothing on this site is labelled implemented, because nothing is. Once code
exists there will be a support matrix recording what actually works, per resource
type and per distribution, and entries will only appear in it after the behaviour
exists and is tested.

Most configuration examples are proposed. The `datum.dev/v1alpha1` API version in
them says the same thing more formally: the alpha suffix means field names,
defaults and semantics can change without a migration path until the group
reaches a stable version.

## What is settled so far

The reconciliation model is settled. Desired state comes from Git, observed state
comes from the host, the two produce a plan, and the plan is applied and
verified. The five phases and their order are not up for negotiation, because
everything else in the design assumes them.

The separation between resources and providers is settled, as is the decision to
keep host identity separate from host classification. Both are recorded as
accepted decisions.

The fleet composition model is proposed rather than accepted. Labels and
selectors are the intended mechanism, and the precedence and conflict rules are
written down, but they have not survived contact with a real repository yet and
are expected to move.

## What has to be true before implementation starts

The documentation phase is finished when an engineer who has never seen Datum can
read this site and come away knowing what a resource is, how identity and
dependencies work, how one repository produces per-host desired state, what an
effective manifest contains, where the provider boundary sits, how drift is
detected, how planning differs from applying, and which parts of the design are
still open.

Until that holds, improving the specification is more valuable than starting on
the agent.

## Versioning

There are no releases. When there are, the documentation will carry a version
selector and this page will be replaced by something that records what shipped
rather than what is intended.
