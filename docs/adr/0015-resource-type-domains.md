# ADR-0015: Group resource types into domains

## Status

Accepted

## Context

Nine resource types exist, and nothing ordered them. The type pages sat in one flat
navigation section, the index listed them in the order the nginx example introduces them,
the support matrix used the order they were implemented in, and the capability table used a
third order. Two of those lists had drifted far enough to omit a type that exists.

A flat list works at nine types and stops working before twenty. The types left out of the
current set include `Mount`, `Firewall`, `Hostname` and `Timezone`, and adding any of them
to a flat list makes the ordering question worse rather than answering it.

The available groupings are not equivalent. Grouping by provider would tie the taxonomy to
how each distribution happens to implement a type, which is the coupling the [provider
boundary](../providers/index.md) exists to prevent. Grouping by implementation status would
change as work lands. Grouping by what a type manages on the host is a property of the type
itself and does not move.

A grouping can also be made to carry behaviour. Domains could gate which types an operator
may declare, appear in a resource reference as `Core/Package[nginx]`, or become a `domain`
key on a document. Each of those makes the taxonomy load-bearing, which means a grouping
decision taken for navigation would become a compatibility surface. `Package[nginx]` is
already the reference form recorded in
[ADR-0008](0008-resource-reference-and-target-identity.md), and a qualified form would
either replace it or exist alongside it.

## Decision

Resource types are grouped into four domains, by what a type manages on the host.

```text
Core        Package, Repository, File, Directory, Symlink
Identity    User, Group
Runtime     Service
Kernel      Sysctl
```

A domain is a grouping and carries nothing else. It is absent from documents, absent from
resource references, and no reconciliation behaviour reads it. A resource is written
`type: Package` and referred to as `Package[nginx]` whichever domain its type belongs to.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
```

The table lives in `internal/document` beside the set of recognised type names, and a test
fails the build when a type has no domain or a domain has no types. The documentation
orders every list of types by domain.

A type belongs to one domain. A type that appears to belong to two is treated as evidence
that it should be two types.

## Consequences

The type pages nest one level deeper in the navigation, and every list of types in the
documentation uses one order. The two lists that had lost a type were corrected while being
regrouped, and a test now makes that class of omission a build failure rather than something
to notice by reading.

The grouping does not cover everything, and that is visible rather than hidden. `Mount` sits
between Core and Kernel, and `Firewall`, `Hostname` and `Timezone` have no domain. A fifth
domain is expected before any of those types is added. Stretching one of the four to fit
would leave the taxonomy describing nothing in particular, which is the state it replaced.

Later work can attach per-domain behaviour without first having to invent the taxonomy. A
per-domain metrics label, a `datum status` grouped by domain, and documentation generated
from the type tables are all reachable from here. None of them is decided by this record.

Keeping domains out of the reference form means a domain can be renamed or a type moved
between domains without breaking a repository. It also means a domain cannot be used to
disambiguate two types with the same name, so type names stay globally unique, which they
already were.

The cost is a second axis over the same nine types, alongside the [capability
set](../providers/capabilities.md) that maps a type to the provider serving it on a host.
The two are independent. A domain is a property of a type and is the same everywhere, and a
capability is a property of a host. A reader meeting both in the same table has to hold two
ideas at once, which is the main argument against having done this.
