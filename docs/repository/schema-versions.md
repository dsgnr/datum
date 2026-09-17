# Schema versions and compatibility

A repository outlives any particular agent version, and an agent has to cope with a repository written
against a schema older or newer than the one it knows. The rules for both directions are set out below.

## The document declares its version

Every document opens with `datum: v1alpha1`, and that field is the schema version, which means a
document is interpreted at the version it declares.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
```

Nothing declares a version for the repository as a whole. A `Fleet` document does not state which
schema the repository uses. Adding such a field would create the possibility of it disagreeing with the
documents beneath it, which is a class of confusion with no corresponding benefit.

Declaring the version per document also makes a gradual migration expressible. A repository part-way
through a migration holds documents at two versions with each interpreted at its own, rather than
needing every file changed in a single commit.

[`datum validate`](../reference/cli.md#datum-validate) counts the documents at each version, so a
migration that is part-done reports how much of it is left.

```text
schema     v1alpha1 581, v1beta1 31, migration in progress
```

## Newer agent, older repository

An agent reading a repository written against an older schema works, and continues to work.

An agent supports a range of schema versions and reads any document declaring one of them, so an agent
that understands `v1alpha1` and `v1beta1` reads a repository containing either or both.

!!! note "Implementation status"

    The agent holds a list of the versions it reads, checks each document against that list, and
    keeps the declared version on the parsed document so later stages can report it. `datum version`
    lists the versions and `datum validate` counts the documents at each.

    Only `v1alpha1` is on the list today, so the range is one version wide and no repository can
    yet be part-way through a migration. The second version is what exercises this, and
    `datum migrate` below does not exist.

Support for a version is dropped only in a major agent release, and dropping it is a breaking change
announced as one. An agent that no longer supports a version says so by naming the version instead
of failing to parse the document.

## Older agent, newer repository

An agent reading a repository written against a newer schema fails, which is both deliberate and safe.

An agent encountering `datum: v1beta1` when it only understands `v1alpha1` refuses the document. An
unrecognised schema version is an error in the same way an [unrecognised
`type`](../reference/manifest-format.md#common-structure) is, so resolution fails and the pass ends
before the host is read.

```text
fleet/roles/web/nginx.yaml:1: unsupported schema version "v1beta1", this agent supports v1alpha1
```

The error names the versions this agent does read, so it distinguishes a document written
against a newer schema from a typo. Which versions a binary reads is also reported by
[`datum version`](../reference/cli.md#datum-version), without needing a repository to hand.

Failing closed is the only safe behaviour available here. An agent that guessed at a newer schema would
apply a partial understanding of desired state as root, and that is worse than applying nothing at all.

What keeps this from being an outage is [last known good](../reconciliation/last-known-good.md). The
host keeps reconciling the newest revision it could resolve, so an agent left behind during an
upgrade continues enforcing correct older desired state while reporting that it cannot advance.

That combination gives a fleet a real upgrade order, in which agents are upgraded first and the
repository migrates afterwards. The reverse order leaves every un-upgraded host stuck on its last known
good until it catches up.

## Migration is a repository operation

!!! note "Proposed behaviour"

    `datum migrate` does not exist yet. Its shape matters now because it decides whether migration is
    something a human commits or something an agent performs, and those two possibilities have very
    different consequences.

```text
$ datum migrate --to v1beta1

rewrote 41 documents in 18 files
  fleet/base/packages.yaml
  fleet/roles/web/nginx.yaml
  ...

review the diff before committing
```

Migration rewrites documents in place and then stops, producing a diff for a human to read and commit
in the same way any other change to desired state does.

Migration is never something an agent performs. An agent that rewrote repository content would be
writing to the source of truth it is supposed to be reading, which
[nothing in Datum does](../adr/0003-git-as-desired-state-source.md). It would also mean desired state
changing without a reviewed commit behind it.

A migration that cannot be performed mechanically stops and says which documents need a human. A
field that split into two, or one whose meaning changed and not its name, is not something a
rewriting tool should guess at. A migration that silently got it wrong would do more damage than one
that refused to proceed.

## What a version change is allowed to do

Changes within a version are additive, so a new optional field, a new resource type or a new matcher
form can appear in a patch release of the agent. A repository written before any of them existed keeps
working unchanged.

Renaming a field, removing one, changing a default or changing what a value means requires a new schema
version. That is what `v1alpha1` being alpha permits, and it is why the
[stability page](../reference/stability.md) puts the document schema last in the order things
stabilise.

| Change | Needs a new schema version |
| ------ | -------------------------- |
| A new optional field | No |
| A new resource type | No |
| A new matcher form | No |
| A new required field | Yes |
| A renamed or removed field | Yes |
| A changed default | Yes |
| A changed meaning for an existing value | Yes |

The last row is the dangerous one, because it is the only change that a repository cannot detect for
itself. A field that silently starts meaning something else produces a repository that parses,
validates and then does the wrong thing. That is why it sits with the breaking changes rather than
being treated as a clarification.
