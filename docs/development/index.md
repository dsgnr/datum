# Development

Datum is being built specification first. A complete pass runs end to end for all nine resource
types, and [project status](../introduction/project-status.md) tracks what is and is not done.

The most useful contributions are still the ones that find a problem in the design before anything is
built around it.

## Making a change

Start with the behaviour you want to change, then find the page or code that owns it. The
[project status](../introduction/project-status.md) shows what exists; the
[architecture decisions](../adr/index.md) explain why accepted behaviour works the way it does.
Check [open questions](open-questions.md) before proposing a design change, since the question may
already be recorded.

Make the smallest change that resolves the problem:

1. For a design disagreement, explain the operational case and the cost of the current decision.
   If the decision is accepted, propose a replacement rather than editing the decision in place.
2. For an unclear or incorrect explanation, update the page that owns the topic and check its
   examples and links to related pages.
3. For a code change, include a test for the behaviour and keep the implementation consistent with
   the documented design. If the implementation reveals a design gap, record the decision in the
   docs as part of the same change.

Run the check for the kind of change you made:

```sh
make lint        # Go formatting, vet and tests
make docs-check  # strict documentation build and Mermaid checks
```

For a provider change, also run its integration target from `make help`; those tests exercise real
system tooling and may require Docker or a Linux host. Before finishing, check that the change says
what users can do today, not what is only planned.

## What is useful

Disagreeing with a decision, given a reason. Every architecture decision record has a Consequences
section, and an argument that one of them underestimated a cost is worth raising.

Finding a case the model cannot express. The design is narrow, and whether it is too narrow is the
open question behind most of the others. A concrete example of configuration that cannot be
described, or a host that cannot be classified, is directly actionable.

Answering an [open question](open-questions.md). There are more than sixty recorded, several of
which need somebody with operational experience of a distribution rather than further design work.

Finding an inconsistency. The examples across the site describe one fleet, and the terminology is
meant to be used identically everywhere. Two pages disagreeing is a defect.

Improving an explanation. A page that is technically correct and hard to follow is not finished.

## What is premature

Anything central. Datum resolves on the host and has no server, so a change that assumes one is a
different design, not an addition to this one.

Applying state is where a mistake is expensive, so the rules it has to follow were written before
the code that follows them. A provider that writes without following the [safety
rules](../security/provider-safety.md) is not an early version of one that does.

A decision made incidentally by an implementation is a decision nobody argued about, and the ones
remaining are the expensive kind.

## How the design changes

Small corrections go straight in. An inconsistency, a broken example, or an explanation that
does not work needs no ceremony.

Changing something marked **Accepted** means superseding its architecture decision record. The
original stays readable with its status changed, and the new record explains what changed and
why, because the reason a decision was reversed is usually as useful as the reversal.

Changing something marked **Proposed** means editing the page. Proposed designs are expected to
move, which is what the label is for.

Recording a new open question means adding the admonition where the gap is and an entry in
[open questions](open-questions.md), so the list stays complete.

## How the documentation and the code stay together

The questions marked as blocking in [open questions](open-questions.md) are the ones that would
otherwise be answered by accident during implementation. Several have since been answered on purpose
instead, which is the arrangement working.

Where the implementation finds a gap the specification had not thought through, the answer goes into
the documentation as a decision instead of staying in the code as an accident. The fourth [pass
outcome](../concepts/index.md#pass-outcomes) arrived that way. An observe-mode pass that
found work to do fitted none of the three outcomes that had been written down, and inventing a
fourth in the code alone would have left the site describing a system that no longer existed.

## The pages

[Working on the documentation](documentation.md)
:   Building and previewing the site, the structure it follows, and the writing conventions.

[Open questions](open-questions.md)
:   Every unresolved question on the site, in one list.
