# Host identity

Two separate questions get confused in fleet management, and keeping them apart
is a structural decision, not a naming preference.

Identity
:   Which machine this is. The host answers this.

```text
I am web-001
```

Classification
:   What that machine is for. The repository answers this.

```text
web-001:
  environment: production
  site: london
  role: web
```

A host asserts its identity and nothing else. Everything that follows from being
`web-001`, including which layers apply and therefore which configuration it receives,
is determined by repository content that the host has no part in writing.

## Why the split matters

If a host supplied its own classification, a compromised machine could relabel itself.

```yaml
# If this came from the host, the host decides what it receives
labels:
  environment: production
```

A machine that can claim `environment: production` receives whatever production
configuration exists, which on a real fleet means credentials, certificates and
access that were scoped to production for a reason. The attack requires no
privilege beyond editing a local file, and it leaves no trace in the repository,
so reviewing the repository would show nothing wrong.

Keeping classification in the repository means changing what a host receives requires a commit,
which is reviewable, attributable and revertible. The host's influence is limited to which `Host`
document it claims to be, and that claim is the thing to authenticate.

!!! note "Security consideration"

    This is why no observed fact from the host is a matcher input, and why the
    `datum/` label prefix is reserved and cannot be set by a `Host` document.
    Allowing a host to influence matching, even indirectly through a fact like the
    distribution it reports, moves part of the classification decision onto the
    machine.

## Where identity comes from

!!! note "Proposed behaviour"

    The mechanism below is proposed. Nothing about establishing identity is settled.

The agent reads its host name from local configuration.

```yaml title="/etc/datum/agent.yaml"
host: web-001
```

The system hostname is the obvious alternative and is rejected as the primary
source. Hostnames are assigned by DHCP on some networks, are not reliably unique
across sites, and change for reasons unrelated to a machine's purpose. A rename
would silently re-resolve the machine against a different `Host` document, or
against none, and the failure would look like a configuration problem rather
than an identity one.

An explicit file makes the claim deliberate. A machine whose identity file is missing fails to start
a pass instead of guessing, and a machine claiming a name with no `Host` document in the repository
fails with an error naming the identity it claimed.

How the file comes to be written, and why it is one of the things a golden image may not
carry, is covered under [enrolment](../lifecycle/enrolment.md).

!!! note "Open question"

    Whether the hostname should be usable as a fallback when the file is absent is undecided. It
    would make first boot easier at the cost of reintroducing the failure mode the explicit file
    prevents.

## Identity is not a security control

The agent has the repository. It reads every `Host` document, every `Layer`, and every file
any resource references, because [resolution happens on the
machine](deployment-models.md). A host claiming to be `db-001` instead of `web-001` gains
nothing, since it could already read `db-001`'s configuration either way.

```text
Git
 ↓
Datum agent          (reads the whole repository)
```

Identity selects which configuration to apply. The real boundary is read access to the
repository, granted to the machine as a whole.

## Consequences

Every host having read access to the whole repository is a real limitation, not a temporary one, and
it shapes what belongs in a repository.

Anything committed is available to every managed machine, so a single compromised host
exposes the configuration of the entire fleet. That is why secret material is
referenced rather than committed, instead of being an omission
waiting for a convenient mechanism.

## What a compromised host can do

Being precise about this is more useful than asserting that the design is safe.

It can lie about its identity, which gains nothing. It can lie about
observed state, which causes Datum to apply changes that are not needed or skip changes
that are, affecting only that machine. It can prevent reconciliation entirely, since the
agent runs there.

What it cannot do is change what any other host receives, because that would require committing to
the repository. The blast radius of a compromised machine is bounded by that machine plus whatever
the repository exposed to it, and the second half of that is the part to reduce.

The full set of vectors, including the ones available to an unprivileged local user and not to root,
is in the [security](../security/index.md) section.
