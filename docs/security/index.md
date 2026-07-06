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

**Blast radius of a merge.** A change matching a thousand hosts reaches a thousand hosts.
There is no staged rollout, no canary, and no mechanism for holding some hosts back, so the
first place a bad change is noticed is production. Recovery is a revert and another pass,
because there is no rollback.

**Fleet-wide configuration disclosure.** Every managed host reads the whole repository, so one
compromised host discloses the configuration of every host, which is the reason secret material has
no place in the repository.

**Secret material.** Configuration files frequently need credentials, and Datum has no way to
supply them. Committing them to the repository would hand them to every managed machine.
There is no half-designed mechanism waiting for a decision, which is why this is recorded as a
gap rather than an open question.

**Agent supply chain.** How the agent binary is obtained, verified and updated is not
designed. An agent that updates itself through a resource describing its own package is a
particular hazard, because a failed update leaves nothing running to retry it.

**Compliance claims.** A host reports its own status, so a converged fleet report is a statement by
the hosts rather than independent evidence about them. The [threat model](threat-model.md) covers
the limits of this.

## The pages

[Threat model](threat-model.md)
:   Attack vectors organised by what the attacker can already do, with the impact of each and
    what the design does about it.

[Handshakes and authenticity](handshake.md)
:   How a host proves which machine it is, how it might be enrolled, and how a manifest could
    be shown to be genuine.
