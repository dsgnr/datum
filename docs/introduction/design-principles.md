# Design principles

These are the constraints the design is held to. They are written down because
they are meant to settle arguments later, and because a principle that cannot
reject a proposal is not doing any work. Each principle below states what it
requires and what it excludes.

## Declarative

Configuration describes the state a machine should be in. It does not describe
the sequence of commands used to reach that state.

The practical consequence is that Datum has to be able to reach the described
state from any starting point, including a machine that is half-converged
because a previous pass failed. That pushes work into the implementation, and
the alternative is worse, being configuration that is only correct for machines
in the condition its author had in mind.

This rules out a resource type that runs a command, and it rules out ordering
implied by the position of a document in a file. Where order matters it has to
be declared as a dependency, because that is a statement about the world, not
about the text.

## Reconciliatory

Datum compares desired state with observed state continuously, and every pass
begins by reading the host.

Drift is ordinary input to the reconciler. A machine somebody edited by hand is
not an exception to handle, it is a machine that produces a non-empty plan, and
the same code path that handles a first-time install handles it. There is no
record of what Datum previously did to diff against, because such a record is
wrong the moment anything else touches the machine.

Observation is therefore not skipped on the basis that the previous result is
assumed to still hold.

## Idempotent

Reconciling a converged system produces no changes. Running a pass twice has the
same effect as running it once.

That has to hold at the level of individual resources as well as the pass as a
whole. A provider that rewrites a file with identical content is not idempotent
in any useful sense, because it will restart every service watching that file on
every pass. Deciding whether a change is needed is the planner's job, and a
provider is only asked to act once that decision has been made.

## Explainable

Datum has to be able to answer a fixed set of questions about its own behaviour:

```text
what should exist
what currently exists
what differs
why a configuration applies
what Datum intends to change
what Datum changed
whether the resulting state was verified
```

Most of these fall out of separating the phases. The one that constrains the
design hardest is why a configuration applies, because answering it means
carrying provenance through fleet resolution instead of discarding it once the
merge is done. Every resource in an effective manifest therefore records the
layer that contributed it and the selector that matched.

This rules out merge behaviour that cannot be attributed, and it rules out
optimisations in the resolver that lose the trail.

## Multi-distribution

Distribution differences live behind provider boundaries. The resource model is
not shaped around one package manager, one init system or one distribution's
filesystem conventions.

The design targets systems including Debian, Ubuntu, Fedora, RHEL, Rocky Linux,
AlmaLinux, Alpine Linux and Arch Linux. Targeting them does not mean the first
implementation supports all of them, and two providers that work are worth more
than eight that mostly work.

Where distributions genuinely differ, the difference is exposed, not hidden. An
abstraction that silently behaves differently on one distribution is more
expensive than a documented inconsistency, because the documented case can be
planned around and the silent one is found in production.

## Fleet-oriented

One repository describes one machine or several thousand. Configuration is reused
by composition, and never by copying between hosts.

A host is classified by labels, and configuration attaches to labels through
selectors. Adding a machine means adding a `Host` document rather than
duplicating an existing one, and a repository where two hosts have similar
configuration should express that similarity once.

This rules out per-host files as the primary unit of configuration, and it puts
weight on composition being deterministic. A merge whose result depends on
filesystem ordering, or on which machine ran the resolver, is not acceptable.

## Safe

The agent is assumed to run as root on machines that matter.

Changes are explicit, auditable and verifiable. A plan exists before anything is
applied, the plan names every resource it will touch, and the result is checked
by reading the host again afterwards. Failures are contained, not ignored. If a
resource fails to apply, resources depending on it are skipped instead of being
attempted against a state that is known to be wrong.

This rules out actions taken outside the plan, and it rules out silently
resolving ambiguity. Where two layers of equal precedence disagree about a
value, Datum reports the conflict and produces no manifest.

## Understandable

Behaviour should be predictable from the configuration and the documentation
alone, without knowing how Datum is built.

A simple mechanism with an obvious failure mode is worth more than a clever one
that is usually right. Precedence is an integer a reader can compare rather than
a specificity score derived from selector shape. Lists replace instead of
merging, because keyed list merging requires knowing which key the
implementation chose.

This rules out features whose behaviour needs internal knowledge to predict, and
it is the principle most likely to be cited when rejecting something that would
otherwise be convenient.

## Where the principles pull against each other

They are not all compatible, and pretending otherwise would make them useless for
deciding anything.

Reconciliation pulls against safety. A tool that corrects drift automatically
will eventually correct a change somebody made on purpose during an incident,
which is an argument for reconciliation being able to report without applying
and for that mode being easy to reach.

Explainability pulls against understandability. Provenance for every resource
means more output, and output nobody reads is not an explanation. The likely
answer is that the detail is available on request instead of present by default.

Multi-distribution support pulls against both. Honest differences between
providers have to surface somewhere, and wherever they surface is somewhere the
resource model stops being uniform. That cost is better paid in documentation
than in behaviour.
