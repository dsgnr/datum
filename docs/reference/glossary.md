# Glossary

One entry per concept. Where a term has a fuller treatment elsewhere, the entry links to it rather
than repeating it.

Action
:   What a plan intends to do to a single resource. One of `none`, `create`, `update`,
    `remove` or `skip`. See [plan](../concepts/plan.md#actions).

Agent
:   The process that runs on a host and reconciles it. Reads desired state, observes,
    plans, applies and verifies.

Apply
:   The phase that carries out the actions in a plan. Performed by providers, one action
    at a time, in plan order.

Classification
:   What a host is for, expressed as labels in its `Host` document. Decided by the
    repository, never by the host. See [host identity](../architecture/host-identity.md).

Convergence
:   The condition of a host whose observed state satisfies its desired state for every
    resource in its effective manifest. Established by observation, not asserted.
    See [reconciliation](../concepts/reconciliation.md#convergence).

Desired state
:   The resources a repository says should apply to one host, resolved at one repository
    revision. See [desired state](../concepts/desired-state.md).

Diff
:   The phase that compares desired against observed state, field by field. Performed by
    the planner.

Drift
:   A difference between desired and observed state. Recorded per field, and caused by a
    change on the host or a change in the repository. See [drift](../concepts/drift.md).

Effective manifest
:   The resolved desired state for one host at one revision, carrying provenance and
    identified by a digest. The only input to the reconciliation engine. See [effective
    manifests](../fleet/effective-manifests.md).

Fleet
:   The set of hosts one repository describes. See [fleet](../fleet/index.md).

Fleet resolver
:   The component that turns a repository and a host name into an effective manifest. See
    [components](../architecture/components.md).

Graph builder
:   The component that turns an effective manifest into a validated resource graph,
    rejecting cycles, unresolved references and duplicate targets. See [dependency
    graph](../architecture/dependency-graph.md).

Host
:   A machine under management, declared by a `Host` document.

Idempotence
:   The property that reconciling a converged system produces no changes. Guaranteed by planning, not by providers checking their own work. See [resource
    lifecycle](../resources/lifecycle.md#idempotency).

Identity
:   Which machine a host is. Asserted by the host, unlike classification. See [host
    identity](../architecture/host-identity.md).

Label
:   A string key and string value classifying a host. Written in the `Host` document. See
    [labels and selectors](../fleet/labels-and-selectors.md).

Layer
:   A set of configuration with a selector saying which hosts it applies to and a
    precedence saying how strongly. See [repository
    layout](../fleet/repository-layout.md).

Manifest digest
:   A digest of an effective manifest's content, used to identify exactly what was
    reconciled.

Observation
:   Reading the current state of the resources in a manifest from a host. Read-only,
    scoped to the manifest, and never cached between passes. See [observed
    state](../concepts/observed-state.md).

Observed state
:   What observation reports, at a point in time.

Observer
:   The component that performs observation by calling providers.

Pass
:   One complete run of resolve, observe, diff, plan, apply and verify for one host. Used
    interchangeably with reconciliation where no ambiguity arises.

Plan :   The ordered set of actions that would resolve the drift found in one pass. Data, not an
execution. See [plan](../concepts/plan.md).

Planner
:   The component that performs diffing and planning.

Precedence
:   An integer on a layer deciding which layer wins when two set the same field. Higher
    applies later. See [precedence](../fleet/precedence.md).

Provenance
:   The record of which layer contributed a resource or a field value, and which selector
    caused that layer to match. Preserved through composition so that the reason a
    resource applies can be reported.

Provider :   An implementation of one resource type on one class of system. Chosen from the host,
never named in a document. See [providers](../providers/index.md).

Reconciler
:   The component that performs applying and verifying.

Reconciliation
:   One complete pass for one host. See
    [reconciliation](../concepts/reconciliation.md).

Repository revision
:   The exact commit that desired state was resolved from. See [desired
    state](../concepts/desired-state.md#repository-revision).

Resource
:   A typed description of one thing on a host, stating the condition it should be in. See
    [resources](../resources/index.md).

Resource graph
:   Resources as nodes and declared dependencies as edges. Built and validated before the
    host is read, and the only source of ordering.

Resource reference
:   How a resource is named inside Datum, written `Kind[name]`, unique within an effective
    manifest. See [resource identity](../resources/identity.md).

Resource type
:   The `kind` of a resource, such as `Package` or `File`. See [resource
    types](../resources/types/index.md).

Selector
:   The expression on a layer deciding which hosts it applies to, made of `matchLabels`
    and `matchExpressions`. See [labels and
    selectors](../fleet/labels-and-selectors.md).

Target identity
:   What a resource manages on the host, such as an absolute path or a package name. Two
    resources sharing one is a conflict. See [resource
    identity](../resources/identity.md).

Verify
:   The phase that re-reads affected resources after applying and confirms they hold the
    state that was asked for. Not a retry.

## Terms this documentation avoids

Deliberate omissions, recorded so that they do not creep back in.

**Node** and **machine** are not used as synonyms for host. Host is the term.

**Manifest** on its own is avoided where **effective manifest** is meant, because the
resolved artefact is a different thing from the documents in the repository.

**Apply** is not used to mean a whole pass. It names one phase, and `reconcile` is the
pass.

**Module**, **profile**, **overlay** and **class** all describe roughly what a layer is,
and only layer appears here.

**Fact** is not used for repository-declared labels. Labels are declared, and facts would
suggest something measured on the host.
