# Obtaining the repository

Every control in [trusting desired state](repository-trust.md) reasons about Git history. A shallow
clone leaves two of those controls unenforceable without reporting anything, and a checkout
performed with default settings can execute code before any manifest has been read.

!!! note "Implementation status"

    The agent keeps a full clone with tags at `/var/lib/datum/repository`, fetches rather than
    re-cloning, refuses a shallow clone, and refuses a pass whose recorded revision is absent. The
    four checkout mechanisms below are disabled, and the tests for that run against real
    repositories.

    Two limits are not enforced yet. `maxSourceSize` is read and does nothing, so a `File` pointing
    at a very large blob is not bounded. `source.credential` works for an ssh identity file and not
    for an https token. A token would have to reach git either on a command line every local user
    can read or through a credential helper, and a helper is a command Datum will not run.

## History has to be complete

The agent keeps a full clone, including tags, and fetches instead of re-cloning on each pass.

```text
git fetch --tags        origin
```

Shallow clones are refused, since ancestry cannot be computed from them. The [descendant
check](repository-trust.md#verifying-that-a-revision-is-current) asks whether a candidate revision
descends from the recorded one, and [signed-tag
selection](repository-trust.md#choosing-among-signed-tags) asks which of several tags descends from
the rest. Both questions return the wrong answer, or no answer, when the commits they concern are
absent from the local object store.

A shallow clone fails in a way that is hard to notice. Ancestry between two commits that are both
present still resolves correctly, so the control works on a fleet whose history is short, and begins
passing everything once history grows past the clone depth.

Tags matter for the same reason under `signed-tag`. A tag that was not fetched cannot be a
candidate, and an agent holding only some tags selects a different revision from one holding all of
them.

## When the recorded revision is absent

An agent whose [recorded revision](repository-trust.md#verifying-that-a-revision-is-current) is not
present in the local object store refuses the pass and reports why.

That happens when a repository has been force-pushed, when history has been rewritten, or when the
state directory and the clone have diverged. In each case the agent cannot establish whether the
candidate revision moves forward or backward, so it refuses the pass.

```text
error: cannot verify revision ordering
  recorded revision  8b91f20 is not present in the repository
  candidate          a41c9d3

  history appears to have been rewritten
  clear the recorded revision explicitly to accept a new baseline
```

A missing recorded revision is not treated as first contact. An attacker who can force-push would
otherwise remove the commit a host is pinned to and have that host accept any revision. Recovery is
the same [one-shot operator action](repository-trust.md#the-cases-this-makes-awkward) as any other
baseline reset.

## Checkout is hardened

A Git checkout can run commands. Several standard mechanisms cause the client to execute programs or
reach the network on the repository's instruction, and all of them are disabled.

| Mechanism | Why it is disabled |
| --------- | ------------------ |
| Submodules | A submodule is a second repository fetched from an address the first one chooses. |
| `.gitattributes` filters and `textconv` | Both name commands the client runs while checking files out. |
| Repository-local `config` | A cloned repository's own config would otherwise configure the client reading it. |
| Hooks | A hook in a fetched repository is a script the client runs. |

The rest of the design assumes that reading desired state runs nothing and that execution begins
when a provider acts. A checkout performed with default settings breaks that assumption before the
first document is parsed. A repository declaring a `.gitattributes` filter would reach arbitrary
root execution without a single resource document, which ADR-0011 forbids.

Symbolic links inside the working tree are checked out as links and never followed when resolving a
`desired.source`, which is the same containment rule
[source confinement](provider-safety.md#confining-content-sources) already applies.

## Limits

An unbounded fetch is a denial of service against every host that performs it. A single commit can
make the repository expensive to fetch for the whole fleet.

```yaml title="/etc/datum/agent.yaml"
source:
  fetchTimeout: 5m
  maxRepositorySize: 1GiB
  maxSourceSize: 16MiB
```

`fetchTimeout` bounds the network operation. `maxRepositorySize` bounds what the agent accepts into
its object store, which covers a repository that has grown large as well as one made large
deliberately. `maxSourceSize` bounds a single `File` content source. Without it, a resource pointing
at a very large blob would be read into memory and written to a host that may not have room for it.

Exceeding any of them refuses the revision and leaves the host on
[last known good](../reconciliation/last-known-good.md), which is the same outcome as any other
revision that cannot be used.

Resolution is bounded separately. A repository that parses quickly and resolves slowly, through very
many layers or very many hosts, costs whatever runs [`datum
validate`](../repository/validating-changes.md) as much as it costs an agent. The limits for that
are not specified.

!!! note "Open question"

    What bounds resolution itself is undecided. A single commit adding thousands of layers would
    slow every agent's pass without exceeding any of the limits above. A bound on resolved manifest
    size and layer count looks more useful than one on time, since a host that exceeds a time limit
    stops reconciling altogether.

## The client itself

The agent uses one Git implementation and treats the remote as untrusted input to it.

Transport authentication establishes which server answered. It says nothing about whether the
content that server sent is well formed. A malicious remote is attacking a parser, and the
mitigations are the ones any parser handling hostile input gets, being a current implementation and
the limits above on what it is asked to process.

!!! note "Important limitation"

    A vulnerability in the Git implementation is a vulnerability in Datum. Signature verification
    does not help, since parsing happens first. The exposure is bounded by the remote being a
    configured address, which is why [the repository URL is configuration and not
    discovery](../lifecycle/installation.md#the-repository-url-is-configuration-not-discovery).
