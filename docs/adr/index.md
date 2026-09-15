# Decisions

Architecture decision records for choices where preserving the reasoning will be useful later.
Each one records why a decision was needed, what was decided, and what the decision costs.

| ADR | Decision | Status |
| --- | -------- | ------ |
| [0001](0001-typed-resources.md) | Use typed resources | Accepted |
| [0002](0002-separate-resources-from-providers.md) | Separate resources from providers | Accepted |
| [0003](0003-git-as-desired-state-source.md) | Use Git as the source of desired state | Accepted |
| [0004](0004-labels-and-matchers.md) | Use labels and matchers for fleet composition | Proposed |
| [0005](0005-identity-separate-from-classification.md) | Keep host identity separate from classification | Accepted |
| [0006](0006-explicit-dependencies.md) | Represent resource dependencies explicitly | Accepted |
| [0007](0007-effective-manifest-as-input.md) | Use effective manifests as the reconciliation input | Accepted |
| [0008](0008-resource-reference-and-target-identity.md) | Separate resource references from target identities | Accepted |
| [0009](0009-declared-only-ownership.md) | Manage only declared resources | Accepted |
| [0010](0010-no-self-managed-trust-anchors.md) | Datum does not manage its own trust anchors | Accepted |
| [0011](0011-no-command-execution-from-desired-state.md) | No command execution from desired state | Accepted |
| [0012](0012-substitution-from-declared-labels.md) | Substitute declared label values, and nothing else | Accepted |
| [0013](0013-secret-references-resolved-on-the-host.md) | Secret references, resolved on the host | Accepted |
| [0014](0014-go-as-the-implementation-language.md) | Go as the implementation language | Accepted |
| [0015](0015-resource-type-domains.md) | Group resource types into domains | Accepted |

## What gets an ADR

A decision qualifies when the reasoning behind it is not obvious from the result, and when
somebody is likely to want to revisit it without knowing what was already considered.

Most of the design does not qualify. The fact that `File` has a `mode` field needs no record,
because nothing was weighed up. The fact that `File` is identified by its path while `Package` is
identified by its name does qualify, because the asymmetry looks like an oversight until the
reasoning is available.

An ADR is not written because something exists. Documenting the design is what the rest of this
site is for.

## Statuses

| Status | Meaning |
| ------ | ------- |
| Proposed | Written down and not agreed. Safe to argue with, not safe to depend on. |
| Accepted | Agreed. Implementation should follow it, and changing it means superseding the record. |
| Superseded | Replaced by a later ADR, which is named in this one. |

An accepted ADR is not edited to change its decision. A new ADR is written, the old one is marked
superseded with a link to the replacement, and the original reasoning stays readable, because the
reason a decision was reversed is usually as useful as the reversal.

Correcting a typo or clarifying wording in an accepted ADR is fine. Changing what it decided is
not.

## Format

```text
# ADR-NNNN: Title

## Status

Proposed | Accepted | Superseded

## Context

Why is a decision required?

## Decision

What was decided?

## Consequences

What becomes easier, harder or constrained?
```

Alternatives that were considered and rejected belong in Context, where they show what the decision
was weighed against. Consequences covers costs as well as benefits, and an ADR whose Consequences
section lists only benefits has not been thought through.
