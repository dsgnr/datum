# ADR-0005: Keep host identity separate from classification

## Status

Accepted

## Context

Resolution needs two things about a host. Which machine it is, and what that machine is
for. It is tempting to treat these as one question, because the machine appears to know
both.

If a host supplied its own classification, the labels used for matcher matching would
come from a local file.

```yaml
labels:
  environment: production
```

A machine that can write that file can decide what configuration it receives. On a real fleet,
production configuration includes credentials, certificates and access that were scoped to
production for a reason, so the escalation path is editing a local file. It requires no privilege
beyond what an attacker on the machine already has, and it leaves no trace in the repository, which
means reviewing the repository would show nothing wrong.

Self-classification is also operationally poor even without an attacker. The repository
would no longer be the description of the fleet, because what a host actually receives
would depend on files scattered across the machines.

## Decision

A host asserts its identity and nothing more.

```text
I am web-001
```

The repository decides what that identity means.

```text
web-001:
  environment: production
  site: london
  role: web
```

Classification lives in the `Host` document. No label used for matcher matching
originates on the machine, and the `datum/` label prefix is reserved so that injected
values cannot be shadowed by a `Host` document.

Observed facts about a host are not matcher inputs either. The distribution a machine
reports is something it could lie about, and allowing it to influence matching would move
part of the classification decision onto the machine.

## Consequences

Changing what a host receives requires a commit, which is reviewable, attributable and
revertible. A compromised machine cannot reclassify itself into a more privileged part of
the fleet.

This decision buys less than it appears to. An agent reading Git has the whole repository, so a host
claiming to be `db-001` instead of `web-001` gains nothing it could not already read. Identity there
selects which configuration to apply and does not act as a security control, and the real boundary
is read access to the repository.

Distribution-specific configuration needs a declared label. Since observed facts cannot
drive selection, a host needing different treatment because of what it runs carries a label
saying so, which duplicates something the machine already knows and can be wrong. This is
the most frequently felt cost of the decision and is recorded as an open question in the
provider documentation.

The host name has to come from somewhere deliberate. The system hostname is rejected as
the primary source, because hostnames are assigned by DHCP on some networks, are not
reliably unique across sites, and change for reasons unrelated to a machine's purpose, any
of which would silently re-resolve a machine against a different `Host` document.
