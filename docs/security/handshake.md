# Handshakes and authenticity

Three separate questions arise whenever an agent talks to something. Who is this host, is this
desired state genuine, and is it current. Each has its own mechanism.

!!! note "Implementation status"

    The three sections below carry their own status. What follows the first one describes what is
    assumed today and what has to be added.

## There is no Datum handshake

An agent reading Git authenticates to the Git remote and to nothing else. There is no Datum
protocol for desired state, nothing that serves it, and no identity exchange.

```text
agent  --- SSH key or token --->  Git remote
```

The credential is the whole of the authentication, and it should be read-only, since an agent never
writes to the repository. The security properties are the Git server's.

The agent does open one inbound socket, the optional [metrics
endpoint](../observability/metrics.md#binding-and-exposure) bound to loopback by default. Nothing
reaching it can change what the host applies, so it discloses rather than controls, and it can be
turned off.

Identity is local. The agent reads the host name it claims from its own configuration, and since it
can read the entire repository anyway, that claim grants it nothing it did not already have.
Identity selects which configuration to apply. It is not a security control, which [host
identity](../architecture/host-identity.md) sets out in full.

## Verifying that a revision is genuine

Transport authentication proves the connection reached the right server. It says nothing about who
produced the content, since a commit pushed by a compromised account on the Git server arrives over
a valid connection.

!!! note "Implementation status"

    Implemented. The agent holds a set of trusted public keys and refuses a revision whose commit or
    tag is not signed by one of them. Trust rests on a key rather than on a server, so an attacker
    holding the remote cannot produce desired state the agent accepts.

    [`signed-tag`](repository-trust.md#verifying-that-a-revision-is-genuine) exists because that has
    an operational cost. Requiring a signature on every commit on the tracked branch constrains
    automation and makes key rotation a fleet-wide operation. Moving the requirement to a release
    step avoids both.

## Verifying that a revision is current

A signed old commit is still a valid signed commit, so signature verification does not stop a
downgrade.

!!! note "Implementation status"

    Implemented. The agent records the revision it accepted and requires the next one to descend
    from it, refusing anything that is not a fast-forward. Git history supplies the ordering, so
    nothing is stored beyond one revision identifier. A revert is expressed as a new commit and the
    branch is not rewritten.

    The awkward case is a host that has been off long enough for history to have been rewritten, or
    a repository that force-pushes. Both refuse every pass until an operator runs [`datum revision
    clear`](../reference/cli.md#datum-revision). That is a one-shot action rather than a setting,
    since a setting turned on during an incident tends to stay on.

## Getting the repository credential onto a host

A host with no credential cannot fetch, so something has to place one. The four approaches below
trade off differently, and [enrolment](../lifecycle/enrolment.md) covers the shape they share.

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

Revocation happens on the Git server, with
[no Datum involvement](../lifecycle/decommissioning.md#revocation-stops-changes).

!!! note "Important limitation"

    Revocation stops future changes and does not undo past ones. A compromised host that has lost
    its credential is still running whatever it last applied, and still holds whatever that
    configuration contained. Cutting a host off leaves it to be contained separately.

## The limit of what a handshake covers

Authentication establishes which host is talking. It says nothing about how that host is behaving,
so an authenticated machine holding a valid credential and compromised at root reports whatever its
operator chooses.

A handshake bounds what a host can obtain. It does not bound what a host can claim about itself, so
an agent report is not compliance evidence.
