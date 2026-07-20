# Trusting desired state

Two questions have to be answered before an agent acts on a revision. Was this desired state
produced by someone trusted, and is it the current one. Transport authentication answers neither. A
commit pushed from a compromised account on the Git server arrives over a valid connection.

!!! note "Proposed behaviour"

    The mechanisms below are specified rather than merely intended, and none of them are
    implemented. They are written at this level of detail because they constrain the agent's
    configuration format and its startup sequence, which are cheap to design now.

## Agent configuration

Everything on this page is configured locally, outside the repository, for the reason given in
[trust anchors](#trust-anchors-are-never-managed-by-datum).

```yaml title="/etc/datum/agent.yaml"
host: web-001

source:
  url: https://git.example.com/fleet.git
  branch: main
  credential: /etc/datum/credentials/git

trust:
  signers: /etc/datum/allowed-signers
  require: signed-tag
  tagPattern: "release-*"
  requireDescendant: true
  strictPaths: false

state: /var/lib/datum
```

## Verifying that a revision is genuine

`trust.require` selects what has to carry a valid signature from a key in `trust.signers`.

| Value | The agent applies |
| ----- | ----------------- |
| `signed-commit` | The branch tip, and only if that commit is signed by a trusted key. |
| `signed-tag` | The revision named by a signed tag matching `tagPattern`, not the branch tip. |
| `none` | The branch tip, unverified. |

Both signing modes are offered because requiring a signature on every commit has an operational
cost. It constrains automation that generates commits, and it makes key rotation a fleet-wide event.
`signed-tag` moves the requirement to a release step, so day-to-day commits need no signature and
the fleet acts on a signed statement that a revision is fit to deploy.

`none` lets a fleet run before signing is set up. An agent configured with `none` records that in
every report, so an unverified fleet is visible.

### Choosing among signed tags

With `signed-tag` there can be several candidates, and picking the wrong one is a downgrade.

A candidate is a tag meeting all four of the following, and anything else is ignored and not treated
as an error.

| Requirement | Reason |
| ----------- | ------ |
| An annotated tag object | A lightweight tag is a bare reference with nothing to sign, so it cannot carry a signature. |
| Signed by a key in `trust.signers` | The signature is checked on the tag object itself, not on the commit it points at. |
| Name matches `tagPattern` | The pattern is what separates release tags from every other tag in the repository. |
| Reachable from `source.branch` | A tag on a branch nobody deploys is not a deployment candidate. |
| A descendant of the recorded revision | This bounds the search, and a tag older than the baseline could never be applied anyway. |

From that set the agent selects the unique candidate that is a descendant of every other candidate.
Two tags pointing at the same commit count as one candidate, because neither is a strict descendant
of the other and refusing that case would make a repository unable to tag a release twice.

If no unique candidate exists, because two signed tags sit on genuinely diverged histories, the
agent refuses and reports both instead of choosing.

```text
error: ambiguous signed tag
  release-2026.02.03   a41c9d3   signed by D065FC86
  release-2026.02.04   7b2e918   signed by D065FC86
  neither descends from the other

  the host remains on 8b91f20
```

Bounding candidates by the recorded revision keeps selection cheap on a long-lived repository.
Considering every signed tag in history would make the cost of selection grow with the age of the
fleet, and an old release tag that is never deleted would stay in the candidate set.

Selecting by tag name or by date was rejected. Name ordering depends on the collation applied, and
dates in a tag are metadata the tagger sets. Ancestry is a property of the history.

Every one of these checks depends on the local clone holding complete history and all tags, which is
why [obtaining the repository](repository-fetch.md) specifies that it does.

## Verifying that a revision is current

A signed old revision is still validly signed, so signature verification does not stop a
downgrade. `trust.requireDescendant` closes that.

The agent records the newest revision it has accepted in its state directory. On the next pass, the
candidate revision has to be a descendant of that recorded revision, which Git can answer
directly, and anything else is refused.

Accepted means the revision carried a valid signature, satisfied the descendant requirement,
resolved into an effective manifest and passed manifest validation. A pass that failed to apply
still counts as accepted, so the pointer advances. Holding the pointer back would pin a host with
one persistently failing resource at an old revision, and the commit that fixes that resource could
not reach it.

```text
signature valid, descendant, resolved, validated   pointer advances
apply failed, verification failed                  pointer still advances
signature invalid, not a descendant, unresolvable  pointer unchanged
```

One value is stored and Git supplies the ordering, so the same pointer serves both this control and
[last known good](../reconciliation/last-known-good.md). A revert is expressed as a new commit, and
the branch is not rewritten.

### The cases this makes awkward

A repository that force-pushes, or a host that has been off long enough for its recorded
revision to have been removed from history, will fail the descendant check and keep failing it.

Recovery is a one-shot operation. An operator clears the recorded revision with a documented command
on that host. There is no configuration flag for it, since a flag set once during an incident tends
to stay set.

### First contact

A host with no recorded revision cannot detect a downgrade, because there is nothing to compare
against. Whatever signed revision it first sees becomes its baseline.

This is a trust-on-first-use gap, and the host cannot close it. Provisioning writes the initial
revision into the state directory when the machine is built, which gives the first pass a baseline
set from outside.

## Trust anchors are never managed by Datum

The agent's identity, its credentials and its signer list are not managed by Datum resources,
and Datum refuses to reconcile a resource whose target is one of them.

The case this prevents is a bootstrapping attack. With `allowed-signers` as an ordinary `File`
resource, one malicious commit replaces it with an attacker's key, and every subsequent commit
verifies against that key.

The refusal names specific targets and does not exclude `/etc` as a whole. A resource targeting the
agent's configuration file, its state directory, its credential files or its signer list is rejected
when the manifest is validated, so it fails before the host is touched and the error names the file
and the control it protects.

| Path | Protects |
| ---- | -------- |
| `/etc/datum/agent.yaml` | Host identity, source, and the trust settings themselves |
| `/etc/datum/allowed-signers` | The set of keys that can authorise desired state |
| `/etc/datum/credentials/` | Repository credentials |
| `/etc/datum/secrets/` | Locally held [secret material](../resources/secrets.md#where-the-value-comes-from) |
| `/var/lib/datum/` | The recorded revision, the pass lock and pass reports |
| `/usr/bin/datum` | The agent binary itself |
| The agent's package and service unit | The process that enforces everything above |

This is recorded as [ADR-0010](../adr/0010-no-self-managed-trust-anchors.md).

The cost is that rotating a signer key or moving a fleet to a new remote falls to whatever builds
machines. That cost is accepted in exchange for a control that desired state cannot disable.

## Repository credentials

The credential a host uses to read Git is read-only, because an agent never writes to the
repository, and it is stored at `source.credential` owned by root with mode `0600`.

Per-host credentials are strongly preferred over one shared credential. A shared credential cannot
be revoked without touching every machine, so the first compromise becomes a fleet-wide rotation. A
per-host credential is revoked on its own.

!!! note "Important limitation"

    Every host can still read the whole repository, including every other host's configuration.
    Per-host credentials limit who can use a stolen credential and not what it grants. Narrowing
    what a host can read would require resolving manifests away from the host, which [Datum does not
    do](../architecture/deployment-models.md).
