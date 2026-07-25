# Development

Datum is being built specification first. The parts that resolve desired state exist, the parts that
change a machine do not, and [project status](../introduction/project-status.md) tracks which is
which.

The most useful contributions are still the ones that find a problem in the design before anything is
built around it.

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

Anything that changes a host. That means providers, the reconciler, the agent's scheduling and
locking.

Resolution is implemented because it reads nothing and breaks nothing. Applying state is where a
mistake is expensive, so the rules it has to follow are written down before the code that follows
them.

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

## Before implementation can start

The documentation phase finishes when somebody who has never seen Datum can read this site and
come away knowing what a resource is, how identity and dependencies work, how one repository
produces per-host desired state, what an effective manifest contains, where the provider
boundary sits, how drift is detected, how planning differs from applying, where the trust
boundaries are, and which parts are settled rather than proposed.

The questions marked as blocking in [open questions](open-questions.md) are the ones that would
otherwise be answered by accident during implementation.

## The pages

[Working on the documentation](documentation.md)
:   Building and previewing the site, the structure it follows, and the writing conventions.

[Open questions](open-questions.md)
:   Every unresolved question on the site, in one list.
