# Enrolment

Enrolment is the step where a machine with no identity acquires one the fleet recognises. It happens
once per machine per installation of the operating system, and it is the only point at which
something has to be delivered to a machine out of band.

!!! note "Proposed behaviour"

    None of this exists, and the mechanisms are less settled than the constraints. What each way of
    placing a credential costs is set out under
    [handshakes](../security/handshake.md#getting-the-repository-credential-onto-a-host).

## What enrolment produces

There is no authority to enrol with. The agent reads Git directly, so enrolment is three local facts
and a commit.

```text
1. the machine is given a host name, a repository credential and the trusted signer set
2. the baseline revision is written into the state directory
3. a Host document for that name is committed to the repository
4. the next pass resolves and reconciles
```

| Produced | Chosen by | Lives where |
| -------- | --------- | ----------- |
| A host name | Whoever provisions the machine, never the machine | `/etc/datum/agent.yaml` and a `Host` document |
| A repository credential | The Git server | On the machine |
| A baseline revision | Whoever provisions the machine | `/var/lib/datum/accepted-revision` |

The credential is a long-lived read-only Git credential, not something Datum issues or renews.
Nothing authenticates the host to Datum, so there is no key material of Datum's own.

The ordering matters. A machine configured with a name that has no `Host` document fails its pass
with an error naming the identity it claimed, which is the correct outcome and looks like a failure
instead of a machine waiting.

## The host name is not a security boundary

The name in `agent.yaml` is an unauthenticated local assertion, and nothing verifies that it is the
name somebody issued. That is safe because every agent can read the whole repository anyway, so a
machine claiming a different name gains nothing it did not already have. Identity [selects
configuration and does not grant
access](../architecture/host-identity.md#identity-is-not-a-security-control).

It still matters who chooses the name. A machine that supplies its own name during provisioning can
classify itself, and a fleet where hosts pick their own labels has no classification anything can
rely on. The name comes from whatever provisions the machine, and the labels come from the `Host`
document.

## The baseline revision

A host with no recorded revision cannot detect a downgrade, because
[the descendant check](../security/repository-trust.md#verifying-that-a-revision-is-current) has
nothing to compare against. Provisioning closes that by writing the revision the machine should start
from into the state directory before the first pass.

```text
/var/lib/datum/accepted-revision      8b91f20
```

This is the one host-specific thing written at enrolment that is neither a name nor a credential,
and it is kept out of the image, because an image is built once and the baseline has to be current
at the moment a machine boots. Whatever provisions the machine writes it, in the same step that
writes the host name.

A machine provisioned without it is not broken and is weaker. The agent records the first signed
revision it sees and proceeds, which is trust on first use, and from the second pass onwards the
control works normally.

!!! note "Proposed behaviour"

    Whether the agent should refuse to run without a baseline, rather than falling back to trust on
    first use, is undecided. Refusing makes a provisioning mistake loud at the moment it happens,
    and it also means every machine built by a pipeline that predates this control stops reconciling
    the moment the agent is upgraded. Reporting the fallback through [a
    metric](../observability/metrics.md#security-controls) is the compromise currently specified.

## Reinstallation

A machine that has its operating system reinstalled has lost its state directory, including the
accepted revision. It is provisioned again the same way, and the `Host` document stays where it is.

Nothing has to be retired, because nothing was issued. The name is reusable and there is no key bound
to it. What does have to be redone is the baseline revision, and a rebuild that skips it leaves the
machine on trust on first use.

## Machines that never enrol

A machine that is installed and never given a name and credential does nothing. It resolves no
desired state and applies no changes, which is the correct behaviour for a machine nobody has yet
decided anything about.

An offline machine is the same case with different timing. Fetching needs to reach the Git remote,
so a machine with no network path stays unconfigured until it has one, instead of reconciling
against a cached or assumed identity.

!!! note "Open question"

    Whether unenrolled machines should be discoverable at all is undecided. A fleet cannot tell the
    difference between a machine that was never meant to be managed and one whose provisioning
    failed, and closing that gap would mean something outside Datum knowing which machines are
    supposed to exist. That is inventory, not configuration, and putting it in Datum would make
    Datum responsible for a list it has no way to verify.

## What an image must not contain

A golden image, a container base layer or a machine template may carry the agent. It may not carry
anything that identifies a particular machine or grants access to a fleet.

| In an image | Allowed |
| ----------- | ------- |
| The agent and its providers | Yes |
| Default configuration with no host name | Yes |
| The repository URL in `source.url` | Yes, with the caveat below |
| Trusted signing keys for the repository | Yes |
| A host name in `/etc/datum/agent.yaml` | No |
| A repository credential | No |
| A baseline revision in `/var/lib/datum/` | No |

Two of those prohibitions need stating plainly instead of being left as a table row.

A host name in the image means every machine booted from it claims to be the same host, so a set of
machines all apply one host's configuration and all report status under one name.

A credential in the image means the credential is as widely distributed as the image, which is
usually more widely than anyone intends. It cannot be scoped to one machine, it is readable by
anyone who can obtain the image or a snapshot of a disk built from it, and revoking it requires
rebuilding every machine that ever booted from it. That is the weakest of the
[enrolment approaches](../security/handshake.md#getting-the-repository-credential-onto-a-host) for exactly this reason.

The [baseline revision](enrolment.md#the-baseline-revision) is excluded for a different reason. It
is not a secret and it is not host-specific, and it goes stale, because an image built in March
would give a machine booted in September a baseline six months behind the repository. Every revision
between the two would then satisfy the descendant check, which is most of what the control exists to
refuse, so the baseline is written at enrolment and not baked in.

## The repository URL is configuration, not discovery

The agent learns where its repository is from explicit local configuration, and it
does not discover that location from the network.

```yaml title="/etc/datum/agent.yaml"
source:
  url: https://git.example.com/fleet.git
```

Every other setting is in the [agent configuration reference](../reference/agent-config.md), and the
one line above is the only part an image may carry.

DNS service records and DHCP options are the conventional way to make this self-configuring, and
both are rejected. The agent runs as root and applies whatever the resolved location gives it, so
allowing the network to nominate that location hands the network's owner the ability to redirect a
root process on every machine that boots. An attacker who controls DHCP on one segment would then
control the configuration of everything on it.

Signature verification limits the damage without removing the problem. An attacker who redirects an
agent to a repository it cannot verify causes that agent to stop reconciling instead of applying
attacker content, which is a denial of service across the segment instead of a compromise. Carrying
the location in the image avoids both outcomes and costs one line of configuration.

## How this fits existing provisioning

The agent has no opinion about how it arrives, which is what lets each of these remain the fleet's
own arrangement.

| Mechanism | Installation | Enrolment |
| --------- | ------------ | --------- |
| Golden image or template | Baked into the image | On first boot |
| cloud-init or ignition | Package install from user data | Credential or platform attestation from user data |
| Network installation, such as PXE or kickstart | Package install in the installer | Credential placed by the installer, or first boot |
| Container base image | Baked into the layer | Rarely appropriate, see below |
| Autoscaling group | Baked into the image | Platform attestation, because no human is present |

Autoscaling is the case that decides whether enrolment can require a human. A group that adds
machines at three in the morning cannot wait for an operator to approve each one, so a fleet using
autoscaling needs either platform attestation or short-lived credentials issued by whatever launches
the instances. Manual approval remains available and is a choice a fleet makes about its own risk,
not a default.

## Containers are mostly the wrong fit

A container that reconciles its own contents is describing a machine that should have been built by
an image pipeline. Datum manages long-lived hosts whose state drifts, and a container's state is
replaced rather than corrected.

The one exception is a container used as a host, meaning a long-lived system container with an init
system, its own package database and a lifetime measured in months. That is a host in everything but
virtualisation mechanism, and nothing about Datum's model objects to it.
