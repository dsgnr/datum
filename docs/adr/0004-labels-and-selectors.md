# ADR-0004: Use labels and selectors for fleet composition

## Status

Proposed

## Context

One repository has to describe hosts that are mostly similar and never identical. Twenty
web servers share a package list, differ by site in resolver settings, differ by
environment in kernel tuning, and one of them carries a change somebody made during an
incident. The mechanism has to express the shared parts once.

Several approaches exist.

A file per host is the simplest to understand and duplicates everything shared, so a
change to the common package list is twenty edits and the twentieth gets forgotten.

An inheritance hierarchy, where a host inherits from a role which inherits from a base,
handles the common case well and forces every host into a single chain. A host that is
both a web server and a London machine and a production machine does not fit one chain,
and the usual escape is multiple inheritance with a precedence rule nobody can predict.

Include directives, where a host's file lists the fragments it wants, are explicit and
put the decision in the wrong place. Adding a fleet-wide setting means editing every host
file, and the question of which hosts have a setting requires reading all of them.

Labels with selectors invert the direction. Configuration declares which hosts it applies
to, so adding a fleet-wide setting is one new layer and adding a host is one document with
the right labels. A host can match any number of layers without any of them being its
parent.

The inversion has a cost. Reading a host's file no longer tells you what it receives,
because the information lives in the layers rather than in the host, and answering what a
host gets requires resolving it.

## Decision

Hosts carry labels in their `Host` document. Layers carry a selector saying which hosts
they apply to and a precedence saying how strongly. Resolution evaluates every selector,
sorts the matching layers by precedence, and merges them.

Labels are declared in the repository and never supplied by the host, for the reasons in
[ADR-0005](0005-identity-separate-from-classification.md).

Precedence is an explicit integer rather than being derived from selector specificity or
directory depth, so that two layers can be compared by reading them.

Directory names carry no meaning. `base`, `environments`, `roles`, `sites` and `hosts` are
convention, and the conventional precedence values live in the layer documents rather
than in Datum.

## Consequences

Shared configuration exists once. Adding a host is a `Host` document, and adding a
fleet-wide setting is one layer.

Answering what a host receives requires resolution rather than reading a file, which is
why rendering a manifest for any host from a checkout is a first-class operation rather
than a debugging aid.

Answering why a host receives something requires provenance to survive composition. That
is the constraint this decision imposes on the resolver, and it is the reason provenance
is recorded per field and may not be dropped as an optimisation.

Overlapping layers can disagree. Where two layers of equal precedence set the same field
differently, resolution fails rather than choosing, because a silent choice applied as
root across every matching host is worse than a refusal.

Mistakes are quiet in one specific way. A label typo means a layer does not match, and the
result is configuration that is simply absent rather than an error, since Datum cannot
know which layers were meant to match.

## Why this is proposed rather than accepted

The mechanism is specified and has not been used. Composition models tend to fail in one
of two directions, either being too rigid for cases that arrive later or too expressive to
predict, and neither failure is visible until a repository has real size and several
people editing it.

The parts most likely to move are whether precedence as a single integer is enough, whether
selectors need any form of alternation, and whether requiring distribution labels to be
declared by hand is tolerable.
