# Concepts

This section defines the vocabulary Datum uses. The definitions are normative, because this
documentation is the specification and an implementation that reads a word like "state" loosely will
produce behaviour nobody asked for.

Each concept has exactly one name. Where other configuration management tools offer several
words for the same idea, only one appears here.

## The model

Three things exist at any moment during reconciliation. Desired state is what the repository
says a host should look like. Observed state is what the host looks like. The plan is the
difference between them, as the set of changes that would bring observed state into line.

```text
desired state + observed state -> plan -> reconciliation
```

Reconciliation is carrying out a plan and confirming the result. Drift is any difference between
desired and observed state, whatever caused it. Drift is handled by the normal reconciliation
process.

The rest of the design produces those three things accurately for one host at a time, and
can explain afterwards how each was arrived at.

## Terms defined in this section

| Term | Meaning |
| ---- | ------- |
| [Desired state](desired-state.md) | The resources a repository says should apply to one host, resolved at one repository revision. |
| [Repository revision](desired-state.md#repository-revision) | The exact commit that desired state was resolved from. |
| [Observed state](observed-state.md) | What reading the host reports for the resources in desired state, at a point in time. |
| [Drift](drift.md) | A difference between desired and observed state. |
| [Plan](plan.md) | The ordered set of actions that would resolve the drift found in one pass. |
| [Action](plan.md#actions) | What the plan intends to do to a single resource. |
| [Reconciliation](reconciliation.md) | One complete pass of observe, diff, plan, apply and verify. |
| [Convergence](reconciliation.md#convergence) | The condition of a host whose observed state satisfies its desired state. |

Resource, resource type, provider, host, fleet, label, selector, layer and
effective manifest are also core vocabulary, and each is defined in the section
that owns it rather than being introduced twice.

## Why the phases are named separately

Observe, diff, plan, apply and verify are treated as five distinct things
throughout this documentation, and the separation is structural. Each phase has
a different input, a different output, and a different failure mode, so
collapsing any two of them removes the ability to answer a question that Datum
is required to answer.

Diffing without planning gives a list of differences with no ordering and no
account of dependencies, which is useful for reporting drift and useless for
changing anything. Planning without applying gives an artefact that can be
reviewed before anything happens to a production machine. Applying without
verifying gives a record that commands ran, which is a weaker claim than the
state being correct.
