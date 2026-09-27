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

Declaring the version per document allows a future agent to support more than one schema version at
once. That would let a repository move between versions gradually, with each document interpreted
according to its own declaration.

[`datum validate`](../reference/cli.md#datum-validate) reports document counts by version. The current
agent supports only `v1alpha1`, so a valid repository cannot yet contain documents from multiple
versions.

## Newer agent, older repository

An agent reading a repository written against an older schema works, and continues to work.

An agent supports a range of schema versions and reads any document declaring one of them, so an agent
that understands `v1alpha1` and `v1beta1` reads a repository containing either or both.

!!! note "Implementation status"

    The agent holds a list of the versions it reads, checks each document against that list, and
    keeps the declared version on the parsed document so later stages can report it. `datum version`
    lists the versions and `datum validate` counts the documents at each.

    Only `v1alpha1` is on the list today, so the range is one version wide. Mixed-version support
    becomes relevant when the agent supports a second schema version.

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

That combination gives a fleet a real upgrade order: agents are upgraded first, then the repository
can adopt the newer schema. Upgrading the repository first leaves agents that do not support the new
version stuck on their last known good revision until they are upgraded.

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
