# Installing the agent

Installation puts the agent and its providers on a machine and does nothing else. A freshly
installed machine holds no identity, no credential and no knowledge of any repository, so it
reconciles nothing until it is [enrolled](enrolment.md).

!!! note "Proposed behaviour"

    Nothing on this page exists. The separation it describes is the part that matters, because an
    image built on the assumption that installation includes enrolment cannot be un-built once
    thousands of machines have booted from it.

## What installation provides

Installation provides the agent binary, the providers shipped with it, and the directories the agent
needs at runtime.

```text
/usr/bin/datum                 the agent and command line
/etc/datum/                    configuration, empty apart from defaults
/var/lib/datum/                agent state, empty
```

None of those paths are managed by Datum itself, which is the subject of
[ADR-0010](../adr/0010-no-self-managed-trust-anchors.md), so an installation can be replaced by a
package upgrade without a reconciliation pass being involved.

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

## Verifying an installation

```text
$ datum status

host       (not enrolled)
source     https://git.example.com/fleet.git
providers  package, file, directory, symlink, service, user, group, sysctl

this host has no identity, so nothing has been reconciled
```

Reporting an unenrolled state explicitly, rather than failing to start or reporting a converged host
with nothing in its manifest, is what makes a broken image obvious during a build rather than during
an incident. A machine that silently reports success while managing nothing is the outcome this
design exists to avoid.

!!! note "Open question"

    Whether the agent should refuse to start when it has no identity, or run and report an
    unenrolled state as shown above, is undecided. Running makes the state observable through
    [metrics](../observability/metrics.md) before any enrolment has happened, and it also means a
    misconfigured machine looks like a working one to anything checking only whether the process is
    alive.
