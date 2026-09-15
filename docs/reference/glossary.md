# Glossary

One entry per concept. Where a term has a fuller treatment elsewhere, the entry links to it rather
than repeating it.

Accepted revision
:   The newest revision a host has verified, resolved and validated. The only state an agent carries
    between passes, serving both downgrade protection and [last known
    good](../reconciliation/last-known-good.md).

Action
:   What a plan intends to do to a single resource. One of `none`, `create`, `update`,
    `remove` or `skip`. See [plan](../concepts/plan.md#actions).

Affected hosts
:   The hosts whose effective manifest digest differs between two repository revisions,
    which is the set a change actually reaches. See [validating
    changes](../repository/validating-changes.md#which-hosts-a-change-would-affect).

Agent
:   The process that runs on a host and reconciles it. Reads desired state, observes,
    plans, applies and verifies.

Apply
:   The phase that carries out the actions in a plan. Performed by providers, one action
    at a time, in plan order.

Assertive resource
:   A resource that claims one target and holds no opinion about others of the same type.
    Every resource type is assertive. See [ownership](../resources/ownership.md).

Authoritative set
:   A proposed resource declaring the complete membership of a category, so that anything
    on the host in that category but not in the set is drift. See
    [ownership](../resources/ownership.md#authoritative-sets).

Capability set
:   The mapping from resource types to the providers that satisfy them on a host. What a
    distribution reduces to once identified. See [capabilities](../providers/capabilities.md).

Classification
:   What a host is for, expressed as labels in its `Host` document. Decided by the
    repository, never by the host. See [host identity](../architecture/host-identity.md).

Configuration validation
:   Checking that an application will accept proposed configuration content, performed on staged
    content inside apply and before the change goes live. Distinct from manifest validation and from
    verification. See [configuration validation](../resources/validation.md).

Convergence
:   The condition of a host whose observed state satisfies its desired state for every
    resource in its effective manifest. Established by observation, not asserted.
    See [reconciliation](../concepts/reconciliation.md#convergence).

Declarative
:   The property that desired state is fully described by the repository at a revision.
    Distinct from reproducible. See [last known
    good](../reconciliation/last-known-good.md#declarative-is-not-reproducible).

Decommissioning
:   Emptying a machine of the resources Datum applied to it, then unenrolling it. Distinct from
    revocation and from unenrolment. See [leaving the
    fleet](../lifecycle/decommissioning.md#decommissioning-a-machine).

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

Enrolment
:   The step in which a machine with no identity acquires one the fleet recognises, along with any
    credential that identity needs. See [enrolment](../lifecycle/enrolment.md).

Extension
:   Code installed on a host supplying provider implementations for resource types Datum does not
    ship. Runs out of process and as root, and is not installed through desired state. See
    [applications](../resources/applications.md#extensions).

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
    [labels and matchers](../fleet/labels-and-matchers.md).

Last known good
:   The most recent revision that resolved and validated cleanly, reconciled when a newer
    revision fails to resolve. A revision identifier, not stored system state. See [last
    known good](../reconciliation/last-known-good.md).

Layer
:   A set of configuration with a matcher saying which hosts it applies to and a
    precedence saying how strongly. See [repository
    layout](../fleet/repository-layout.md).

Manifest digest
:   A digest of an effective manifest's content, used to identify exactly what was
    reconciled.

Manifest validation
:   Checking that repository documents are well formed and internally consistent, performed before the
    host is read. Distinct from configuration validation. See [document
    format](../reference/manifest-format.md#validation-summary).

Matcher
:   The `match` block on a layer deciding which hosts it applies to, made of `labels`,
    `oneOf`, `noneOf`, `has` and `missing`. See [labels and
    matchers](../fleet/labels-and-matchers.md).

Mode
:   Whether an agent applies changes or only reports drift, being `enforce` or `observe`.
    Set on the host. See [reconciliation modes](../concepts/reconciliation-modes.md).

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

Pass lock
:   The exclusive lock every process that reconciles holds from observation through verification, so a
    scheduled pass and an operator's `datum reconcile` cannot overlap. See
    [one pass at a time](../reconciliation/locking.md).

Plan :   The ordered set of actions that would resolve the drift found in one pass. Data, not an
execution. See [plan](../concepts/plan.md).

Planner
:   The component that performs diffing and planning.

Precedence
:   An integer on a layer deciding which layer wins when two set the same field. Higher
    applies later. See [precedence](../fleet/precedence.md).

Provenance
:   The record of which layer contributed a resource or a field value, and which matcher
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

Reproducible
:   The property that the same revision produces the same result on the same host at any
    time. Distinct from declarative, and only as strong as the pinning of the inputs a
    revision refers to. See [last known
    good](../reconciliation/last-known-good.md#declarative-is-not-reproducible).

Resource
:   A typed description of one thing on a host, stating the condition it should be in. See
    [resources](../resources/index.md).

Resource graph
:   Resources as nodes and declared dependencies as edges. Built and validated before the
    host is read, and the only source of ordering.

Resource reference
:   How a resource is named inside Datum, written `Type[name]`, unique within an effective
    manifest. See [resource identity](../resources/identity.md).

Resource type
:   The `type` of a resource, such as `Package` or `File`. See [resource
    types](../resources/types/index.md).

Resource type domain
:   A grouping of resource types by the part of a host they describe, being Core, Identity,
    Runtime and Kernel. A grouping only, absent from documents and from resource
    references. See [domains](../resources/types/index.md#domains).

Revocation
:   Withdrawing a host's ability to obtain desired state, which takes effect when its current
    credential expires. Stops future changes and undoes nothing. See [leaving the
    fleet](../lifecycle/decommissioning.md#revocation-stops-changes).

Ring
:   A set of hosts tracking one Git ref, used to get a change to a few machines before the fleet.
    Membership is agent configuration, not repository content. See [staged
    rollout](../reconciliation/staged-rollout.md).

Schema version
:   The version a single document declares in its `datum` field, deciding how that document
    is interpreted. Declared per document, not per repository. See [schema
    versions](../repository/schema-versions.md).

Secret reference
:   A name in desired state standing for a credential, resolved on the host during apply. The value
    never enters the repository, the manifest, its digest, a plan, a log or a report. See [secret
    references](../resources/secrets.md).

Splay
:   The per-host offset that spreads reconciliation across a fleet, derived from the host identity so
    that a machine's pass times are stable. See
    [scheduling](../reconciliation/scheduling.md#passes-are-spread-deterministically).

Substitution
:   Replacing `{{ labels.NAME }}` or `{{ host }}` in desired state with values declared in a `Host`
    document, during resolution. Declared labels are the only source. See [substituting label
    values](../fleet/substitution.md).

Target identity
:   What a resource manages on the host, such as an absolute path or a package name. Two
    resources sharing one is a conflict. See [resource
    identity](../resources/identity.md).

Unenrolment
:   Retiring a host's identity and removing its `Host` document, so the fleet stops describing the
    machine. Never destructive. See [leaving the
    fleet](../lifecycle/decommissioning.md#unenrolment-is-not-destructive).

Verify
:   The phase that re-reads affected resources after applying and confirms they hold the
    state that was asked for. Not a retry.

## Terms this documentation avoids

Deliberate omissions, recorded so that they do not creep back in.

**Node** is not used for a managed system. Host is the term, and node appears only for a graph node
or a device node.

**Machine** is not a synonym for host and is not avoided either. A host is what the fleet model
describes, meaning a `Host` document and the desired state that resolves for it, and a machine is the
physical or virtual computer that host runs on. A machine can exist before it is a host, which is what
[installation before enrolment](../lifecycle/installation.md) means, and a host can be described in a
repository before any machine claims it.

**Manifest** on its own is avoided where **effective manifest** is meant, because the
resolved artefact is a different thing from the documents in the repository.

**Apply** is not used to mean a whole pass. It names one phase, and `reconcile` is the
pass.

**Module**, **profile**, **overlay** and **class** all describe roughly what a layer is,
and only layer appears here.

**Fact** is not used for repository-declared labels. Labels are declared, and facts would
suggest something measured on the host.

**Selector** is not used for a layer's `match` block. Matcher is the term, chosen so that
the concept and the key share a name.

**Spec**, **kind**, **metadata** and **apiVersion** do not appear as field
names. The document format is not a Kubernetes object, and reusing those names
would imply behaviour Datum does not have, such as a server, a status
subresource or namespaces.
