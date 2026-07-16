# Last known good

A pass begins by resolving a revision from Git. That step can fail before the host is touched at
all, and what an agent does when it cannot obtain a usable revision is a first-class question, not
an error path.

## Two kinds of failure

Resolving and validating a revision can fail in ways that have nothing to do with the host.

```text
syntax error in a document
unknown resource type
unresolved requirement
dependency cycle
conflicting resources at equal precedence
a matcher that does not parse
signature that does not verify
```

Every one of these is caught before observation, by the parts of the pipeline that run against
[repository content alone](../concepts/desired-state.md#resolution-does-not-read-the-host). None of
them says anything about whether the host is healthy, and none of them can be fixed on the host,
because the fault is in the repository.

## Abandoning desired state instead

The obvious implementation abandons desired state when the newest revision does not resolve.

```text
bad commit
     │
     ▼
resolution fails
     │
     ▼
agent has no desired state
     │
     ▼
host stops being managed
```

This makes a single bad commit a fleet-wide outage. A syntax error merged to the tracked branch
would, on the next pass, leave every host with nothing to reconcile against, so drift would go
uncorrected everywhere until someone fixed the commit. The blast radius of a typo would equal the
blast radius of a deliberate change.

## The last-known-good revision

An agent keeps the newest revision it has accepted, meaning one that verified, resolved and passed
validation. When a newer revision fails to resolve, the agent reports the failure and keeps
reconciling that accepted revision.

It is the same stored value that [downgrade
protection](../security/repository-trust.md#verifying-that-a-revision-is-current) compares against,
and not a second pointer kept alongside it, and it advances on a successful resolve and validate
regardless of what the apply that followed did.

```text
new revision
     │
     ▼
resolve and validate
     │
     ├── success --> record as last known good, reconcile it
     │
     └── failure --> report the failure, keep reconciling
                     the last known good
```

A bad commit therefore stops the fleet moving forward and does not stop it working. Hosts continue
enforcing the last good state, drift is still corrected against it, and the failure is visible in
every host's [status](../reference/status.md) as a revision that would not resolve. Fixing it is a
new commit, which every host picks up on its next pass.

## This is not stored system state

Last known good is a revision identifier, not a snapshot of the host and not a record of what Datum
applied.

That distinction matters, because Datum [keeps no record of what it previously
applied](../concepts/desired-state.md#desired-state-is-not-a-record-of-what-datum-did) and has [no
rollback](../concepts/reconciliation.md#there-is-no-rollback). Last known good does not reintroduce
either. The agent still resolves desired state from Git, still reads the host fresh on every pass,
and still holds no observed state between passes. All it retains is which commit to resolve, which
is [the one thing carried between
passes](../architecture/reconciliation-flow.md#what-is-written-down), and when the newest one is
unusable it resolves an earlier one that is known to be usable.

The earlier revision is reconciled exactly as the newest one would be. There is no reverting of
applied changes and no attempt to restore a prior host state, only a choice of which desired state
to resolve.

## Interaction with downgrade protection

Reconciling an older revision looks like a downgrade, and
[downgrade protection](../security/repository-trust.md#verifying-that-a-revision-is-current) exists
to refuse those.

There is no conflict, because the two concern different revisions. Downgrade protection refuses to
move the last-known-good pointer backwards to an older revision presented as new. Last-known-good
recovery continues reconciling the pointer where it already is, because the newer revision failed
to resolve and never became a candidate to move it forward. The pointer only ever advances, to a
revision that both validates and satisfies the descendant requirement, and it never retreats.

## Offline and intermittent hosts

The same mechanism lets a host survive losing contact with the Git remote.

GitOps for operating systems is most attractive for machines that are not permanently connected,
including edge sites, retail, factories, ships, remote offices and developer laptops. A host that
stopped working the moment its source became unreachable would be unusable in exactly those places.

A host that cannot reach its source keeps reconciling the last-known-good revision it already holds.
It is not converging on anything new, because it cannot see anything new, and it continues
correcting drift against the last state it successfully resolved. Reconciliation is local, and a
source is needed to change desired state, not to keep enforcing it.

```text
source unreachable
     │
     ▼
keep reconciling the last known good
     │
     ▼
source returns --> resolve, validate, advance
```

### How long a cached revision stays usable

Continuing indefinitely is not obviously right. A host that has been disconnected for months is
enforcing a policy that may have been changed since, and a signed manifest that is years old is a
replay risk if the disconnection was engineered.

!!! note "Open question"

    Whether a cached revision expires, and what a host does when it does, is undecided. Continuing
    to enforce stale desired state and refusing to enforce anything are both defensible and suit
    different deployments, a factory floor wanting the former and a laptop the latter. The decision
    interacts with [signature freshness](../security/handshake.md), because an expiry measured in
    wall-clock time depends on a clock the host may not have set correctly, which is covered under
    [time](../security/time.md).

## Declarative is not reproducible

Last known good records a revision, and a revision does not fully determine what a pass does,
because desired state depends on more than Git.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: latest
```

Were `latest` permitted, and it is [not](../resources/types/package.md#there-is-no-latest), desired
state would depend on Git plus whatever the package repository serves at the moment of the pass.
Even a pinned version depends on that version still being available to install. A revision is a
declarative source of truth, and it is not a reproducible one, because the surrounding world moves.

Datum distinguishes the two rather than pretending a Git revision captures
everything.

Declarative
:   Desired state is fully described by the repository at a revision. Datum is declarative by
    construction.

Reproducible
:   The same revision produces the same result on the same host at any time. Datum is reproducible
    only to the extent that the resources in a revision are themselves pinned to inputs that do not
    move, which package repositories generally are not.

A fleet that needs reproducibility pins versions and controls its package sources, and a fleet that
values staying current accepts that two hosts reconciling the same revision at different times may
reach different results. Both are supported, and the documentation does not present them as the
same property, because conflating them is how a system comes to be described as reproducible when
it is only declarative.
