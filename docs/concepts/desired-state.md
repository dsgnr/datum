# Desired state

Desired state is the set of resources a repository says should apply to one host, resolved at
one repository revision. It is specific to both, so a statement about desired state that
names only one is incomplete.

There is no global desired state. The repository describes a fleet, and desired state is the
result of resolving that description for one machine. Two hosts in the same repository at
the same revision normally have different desired states, since different layers match
them.

## Repository revision

Every resolution records the commit it was performed against.

```text
host       web-001
revision   8b91f20
```

Without the revision, a result cannot be reproduced and a change in behaviour cannot be
attributed to a change in configuration. With it, what Datum believed when it acted has a
definite answer.

A revision is a commit, not a branch name. A branch name refers to different content over
time, which makes the record useless for the cases it exists for.

## Absence does not mean removal

A resource that is not in desired state is not managed by Datum. It is not
implicitly scheduled for removal.

This is the most consequential semantic decision in the resource model, so it
needs stating plainly. If `Package[nginx]` is deleted from the repository, the
nginx package stays installed on hosts that already have it, and Datum stops
having an opinion about it. Removing software requires declaring that intent:

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: absent
```

Treating desired state as a complete description of the machine, with anything
undeclared removed, would prevent partial management of a host. Adopting Datum
on an existing machine would then start by deleting most of it.

!!! note "Proposed behaviour"

    Declared-only ownership is the intended model and nothing in the design
    depends on the alternative. Whether a resource type should also support
    reclaiming a directory completely, which is the one case where undeclared
    content does need an opinion, is an open question recorded with the
    `Directory` resource.

## Resolution does not read the host

Desired state is resolved from repository content alone. Matchers match against
the labels in a host's `Host` document, which are written by whoever maintains
the repository, so resolving desired state for any host can be done from a
checkout without access to the machine.

That property is what makes rendering desired state for a host reviewable in
advance and comparable between two revisions. Resolution therefore requires no
access to the machine.

Provider selection does read the host, because choosing between `apt` and `dnf`
requires knowing what the host is running. Provider selection happens after
resolution and does not change which resources apply, only how they are realised.

!!! note "Open question"

    Matching on observed host facts, such as the distribution reported by
    `/etc/os-release`, would make some configuration easier to express and would
    remove the ability to resolve a manifest without the host. The current
    position is that matchers use declared labels only, and that a host needing
    distribution-specific treatment carries a declared label saying so. This has
    not been tested against a real repository.

## Desired state is not a record of what Datum did

Datum does not store what it previously applied and compare against that. Desired
state comes from the repository on every pass, and the comparison is always
against the host as it is now.

A record of past actions treated as authoritative would hide every change made
outside Datum. The cost of not keeping one is that Datum must be able to read
the current state of everything it manages, which constrains what can reasonably
become a resource type.
