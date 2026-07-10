# Journey: a bad commit reaches main

A change adding a role layer is merged. It contains a typo in a resource type and matches every
production web host. This journey follows what each of those hosts does.

## The commit

```yaml title="fleet/roles/web/tuning.yaml"
datum: v1alpha1
type: Sysctrl

name: net.core.somaxconn

desired:
  value: "4096"
```

`Sysctrl` is not a resource type. The intended type was `Sysctl`.

The commit passed review and reached `main` as revision `9c02ab`. A single transposed letter in a
type name is the kind of fault review misses.

## What each host does

**Obtaining the revision.** The agent fetches and finds `9c02ab`. Where [signature
verification](../security/repository-trust.md#verifying-that-a-revision-is-genuine) is configured
the commit verifies, since it is a legitimate commit by a trusted signer. Signing establishes who
produced a change and not whether it is correct.

**Resolution.** Discovery reads the documents and rejects the one with the unknown type. An
unrecognised `type` [is an error](../reference/manifest-format.md#common-structure) rather than a
document to skip. Ignoring the misspelling would leave the sysctl unapplied on every host with
nothing reporting it.

```text
error: unrecognised type "Sysctrl"
  fleet/roles/web/tuning.yaml
```

**The pass stops here.** No effective manifest is produced, provider selection does not run, and the
host is not read. This is a whole-pass failure with [nothing to clean
up](../architecture/reconciliation-flow.md#where-failure-stops-the-pass).

**The host keeps working.** The agent reports the failure and continues reconciling
[last known good](../reconciliation/last-known-good.md), which is `7ab21f`. Drift is still detected and
still corrected against that revision. The fleet has stopped moving forward and has not stopped
working.

```text
desired
  revision       9c02ab   (failed to resolve: unrecognised type "Sysctrl")
  lastKnownGood  7ab21f

state
  condition      converged
```

The host reports `converged`, because it is converged on the desired state it is able to resolve. The
failure is visible in the divergence between `revision` and `lastKnownGood` rather than in the host
state, which is the distinction that lets an operator tell "this host is broken" from "this host cannot
move forward".

## What the fleet looks like

Every host the role matched trips the same condition at roughly the same time, so the
[bad-revision alert](../observability/alerting.md#alerts-worth-having) fires across all of them.

```text
datum_revision_timestamp_seconds != datum_last_known_good_timestamp_seconds
```

The alert is grouped by revision rather than by host. Two hundred web servers failing on the same
commit is one problem, and grouping by host would page two hundred times for it.

Hosts the role did not match are unaffected, because the broken document is in a layer whose
[matcher](../fleet/labels-and-matchers.md) does not select them. Their resolution succeeds, they advance
to `9c02ab`, and their `revision` and `lastKnownGood` stay equal. The blast radius of the typo is
exactly the set of hosts the layer was aimed at.

## Abandoning desired state instead

An agent that discarded desired state when resolution failed would leave every matched host with
nothing to reconcile against. Drift would go uncorrected across the web tier until the commit was
fixed.

[Last known good](../reconciliation/last-known-good.md#abandoning-desired-state-instead) exists to
avoid that.

## Recovery

A new commit, `3f81cd`, fixes the type. Nothing was applied, so there is no rollback to perform and
no state to repair.

The next pass on each affected host resolves `3f81cd`, validates it, records it as the new last known
good, and reconciles it. The sysctl applies, `revision` and `lastKnownGood` converge again, and the
alert clears.

Reverting the bad commit in Git would work equally well and produce a revision identical in content to
`7ab21f`. Either way the fix is a commit that moves history forward, which is what
[downgrade protection](../security/repository-trust.md#verifying-that-a-revision-is-current) requires,
since it refuses a revision that is not a descendant of the one last applied. A `git revert` satisfies
that and a `git reset` followed by a force push does not, which is the workflow the control is designed
to encourage.

## The other bad commits

The same sequence handles every validation failure, since all of them fail before the host is read.

| Fault | Caught by |
| ----- | --------- |
| YAML that does not parse | Discovery |
| Unrecognised `type` | Discovery |
| A resource document with no `Layer` above it | Discovery |
| A path that is not absolute, or contains `..` | Discovery |
| Two layers of equal precedence disagreeing on a field | Fleet resolver |
| A `requires` reference that does not resolve | Graph builder |
| A dependency cycle | Graph builder |
| Two resources sharing a target identity | Graph builder |

Each produces the same outcome, being a reported failure, no change to the host, and continued
reconciliation of the last known good.

## What this journey does not cover

A commit that is valid and wrong. A change declaring `Package[nginx]` absent on every web host
resolves and validates cleanly, then removes nginx everywhere, which is what the repository
declared.

Datum does not prevent that. The [repository is the control
plane](../concepts/reconciliation.md#there-is-no-rollback), review limits how many hosts a change
reaches, and recovery is a new commit. Validation catches malformed desired state and not mistaken
desired state.
