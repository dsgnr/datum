# Security model

The agent runs as root and takes its input from a repository that several people can write to. This
section states what is trusted, what is defended and what is not defended.

!!! note "Implementation status"

    Nothing here is implemented. Each item states whether it is a design position the rest of the
    documentation depends on or a requirement recorded for later.

## Principals

| Principal | Capability |
| --------- | ---------- |
| Repository writer | Can propose a change to desired state. |
| Repository approver | Can merge a change, which means executing configuration as root on every matching host. |
| Agent | Runs as root on one host, reads desired state, changes that host. |
| Local root on a host | Controls the agent, its configuration and its credentials. |
| Local unprivileged user on a host | Can create and manipulate filesystem entries in directories it owns. |
| Network position | Sits between an agent and the Git remote. |
| Metrics reader | Can connect to an agent's metrics listener. Every local user on a multi-user host qualifies, because loopback is not a privilege boundary. |

## Assets

The repository is the highest-value asset, since merging to it is equivalent to root execution
across the fleet.

The credentials each host holds come next. Each reads the entire repository, so it is worth more
than the host holding it.

Host integrity is an asset in the ordinary sense. Secret material in configuration would be another,
and Datum has no mechanism for it, which is covered below.

## Trust assumptions

These are stated as assumptions because the design does not defend against their failure.

**Anyone who can merge to the tracked branch can run arbitrary configuration as root on every
host their change matches.** There is no additional control between a merge and a reconciled
host. Review and branch protection on the repository are the mechanism that limits this, and
they are outside Datum.

**Local root on a host owns that host.** The agent binary, its identity file and its credentials are
all readable and writable by root, so a host compromised at root level is lost. The design limits
what that gains an attacker on other hosts.

**Transport authenticity to the Git remote is trusted.** TLS certificate validation or SSH
host key verification is assumed to be working. A proposed control described in
[handshakes](handshake.md) reduces the consequences of that assumption being wrong.

**Package signature verification belongs to the distribution.** When a provider installs a
package, the package manager checks its own signatures against its own configured keys. Datum
does not re-verify, and a repository configured to trust a malicious signing key produces
malicious packages that Datum installed on request.

## A host cannot change what it receives

A host cannot change what configuration it receives.

Classification lives in the repository, not on the machine, so a compromised host cannot
relabel itself `environment: production` and receive production configuration. No label used
for matching originates on the host, the `datum/` label namespace is reserved so injected
values cannot be shadowed, and facts observed on a host are not matcher inputs.

[ADR-0005](../adr/0005-identity-separate-from-classification.md) records this decision. It removes
the class of attack rather than reducing its impact.

## What is not defended

**The health of what a change produces.** A change can be staged across [ring
branches](../reconciliation/staged-rollout.md) so that it reaches five hosts before five hundred,
and nothing holds a promotion until the first ring is known to be healthy. What healthy means for
the service being configured is [outside what Datum
measures](../resources/validation.md#where-datums-responsibility-ends), so the gate belongs to the
fleet. A promoted change affects the whole ring, and recovery is a revert and another pass.

**Fleet-wide configuration disclosure.** Every managed host reads the whole repository, so one
compromised host discloses the configuration of every host. [Secret values are not stored in the
repository](../resources/secrets.md), so the disclosure covers what the fleet is configured to do
and not the credentials it uses.

**Where secret values come from.** Desired state [references a secret rather than containing
one](../adr/0013-secret-references-resolved-on-the-host.md), and the backend that resolves the
reference is not provided by Datum. A fleet distributes secret material to its hosts by its own
means. Datum supplies the interface and keeps the value out of the repository, the manifest and
every report.

**Agent and extension supply chain.** How the agent binary, its bundled providers and any
[extension](../resources/applications.md#extensions) are obtained, verified and updated is not
designed. A tampered agent package grants root across the fleet, which places this second to
repository merge in impact. An agent updating itself through a resource describing its own package
is a further hazard, since a failed update leaves nothing running to retry it.

**Compliance claims.** A host reports its own status, so a converged fleet report is a statement by
the hosts rather than independent evidence about them. The [threat model](threat-model.md) covers
the limits of this.

## The pages

[Threat model](threat-model.md)
:   Attack vectors organised by what the attacker can already do, with the impact of each and
    the control that addresses it.

[Trusting desired state](repository-trust.md) :   Signature verification, downgrade protection, and
the trust anchors Datum keeps outside desired state.

[Obtaining the repository](repository-fetch.md) :   The requirement for complete history, what a
hardened checkout disables, and the limits applied to a fetch.

[Applying state safely](provider-safety.md) :   How a provider running as root handles paths, field
values, temporary files and reports, and the local privilege escalations that handling prevents.

[Handshakes and authenticity](handshake.md)
:   How a host proves which machine it is, how it is enrolled, and how a manifest is shown to be
    genuine and current.

[Time and ordering](time.md) :   How the security controls order events without depending on a host
clock, using monotonic ordering where available.

See [installation](../lifecycle/index.md) for the operational side, covering what an image may
carry, how a repository credential reaches a machine and what leaving the fleet does to it.
