# Handshakes and authenticity

Three separate questions arise whenever an agent talks to something. Who is this host, is this
desired state genuine, and is it current. Each has its own mechanism.

!!! note "Implementation status"

    The three sections below carry their own status. What follows the first one describes what is
    assumed today and what has to be added.

## There is no Datum handshake

An agent reading Git authenticates to the Git remote and to nothing else. There is no Datum
protocol, no server, and no identity exchange.

```text
agent  --- SSH key or token --->  Git remote
```

The credential is the whole of the authentication, and it should be read-only, since an agent never
writes to the repository. The security properties are the Git server's.

Identity is local. The agent reads the host name it claims from its own configuration, and since it
can read the entire repository anyway, that claim grants it nothing it did not already have.
Identity selects which configuration to apply. It is not a security control, which [host
identity](../architecture/host-identity.md) sets out in full.

### Verifying that a revision is genuine

Transport authentication proves the connection reached the right server. It says nothing about who
produced the content, since a commit pushed by a compromised account on the Git server arrives over
a valid connection.

!!! note "Proposed behaviour"

    The agent holds a set of trusted public keys and refuses to resolve a revision whose commit
    or tag is not signed by one of them. Trust then rests on a key rather than on a server, and
    an attacker who takes over the remote cannot produce desired state the agent will accept.

    The operational cost is real. Every commit that reaches the tracked branch has to be signed
    by a key the fleet trusts, which constrains automation and means key rotation becomes a
    fleet-wide operation.

## Verifying that a revision is current

A signed old commit is still a valid signed commit, so signature verification does not stop a
downgrade.

!!! note "Proposed behaviour"

    The agent records the revision it last applied and requires the next one to be a descendant of
    it, refusing anything that is not a fast-forward. Git history supplies the ordering, so nothing
    extra has to be stored beyond one revision identifier, and a legitimate revert is a new commit
    rather than a rewritten branch.

    The awkward case is a host that has been off long enough for history to have been rewritten,
    or a repository that force-pushes. Both would require an explicit override, and what that
    looks like has not been designed.

## Getting the repository credential onto a host

A host with no credential cannot fetch, so something has to place one. The approaches below make
different trade-offs.

**A credential baked into an image.** Simple and works offline. It cannot be per-host, it leaks
to anyone who obtains the image, and revoking it means rebuilding every machine.

**Cloud instance user data.** The credential arrives at first boot without being in the image.
User data becomes the thing to protect, and it is readable by anything on the instance that can
reach the metadata service.

**A workload identity the Git server accepts.** A platform attestation or OIDC token exchanged for
repository access, so no long-lived secret is pre-placed. Nothing is stored on the host to steal,
and it works only where both the platform and the Git server support it.

**Manual placement.** An operator installs the credential. Auditable, and it does not scale past
a certain fleet size.

!!! note "Proposed behaviour"

    Datum does not issue credentials and has no opinion about which of these a fleet uses. The
    credential is agent configuration, and [Datum does not manage
    it](../adr/0010-no-self-managed-trust-anchors.md).

    A credential baked into an image is documented as the weakest option rather than left out, since
    a fleet with no other route would otherwise have nothing to follow.

Revocation means removing that credential on the Git server. Datum has no part in it.

!!! note "Important limitation"

    Revocation stops future changes and does not undo past ones. A compromised host that has lost
    its credential is still running whatever it last applied, and still holds whatever that
    configuration contained. Cutting a host off leaves it to be contained separately.

## What a handshake cannot fix

Authentication establishes which host is talking. It says nothing about whether that host is
behaving, so an authenticated, enrolled, credential-holding machine that has been compromised at
root level is still reporting whatever it likes.

The boundary a handshake protects is what a host can obtain, not what it can claim about itself.
Confusing the two would lead to treating agent reports as compliance evidence, which they are
not.
