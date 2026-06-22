# ADR-0003: Use Git as the source of desired state

## Status

Accepted

## Context

Desired state has to come from somewhere, and the options differ in what they give up.

A database or API as the primary store makes querying and per-host delivery
straightforward, and it needs a server to be available before any host can reconcile. It
also has to grow its own versioning, audit trail and review process, because the
properties that come free with version control do not exist by default.

Files pushed to hosts by some other mechanism works and moves the problem rather
than solving it. Whatever produces and distributes those files becomes the real
source of desired state, and its history is the one that matters.

Git provides several things the design already needs. Content is addressable by commit, so a pass
can name exactly what it acted on. History is immutable and attributable, so the question of who
changed what and when has an answer without building anything. Review before merge is an established
practice, not a feature to design. Reverting is an ordinary operation, which matters because Datum
has no rollback of its own and recovery from a bad change is a new commit.

The cost is that Git is not a query interface. Asking which hosts have a particular
package installed means resolving every host, and there is no index to consult.

## Decision

Git is the source of desired state. A repository holds `Fleet`, `Host`, `Layer` and
resource documents, and reconciliation resolves against a specific revision.

Nothing writes back to the repository. There is no mechanism by which a host records its
state into Git, and no command that imports an existing machine into configuration.

A revision is a commit and not a branch name, because a branch name refers to
different content over time and would make a recorded result useless for the
cases the record exists to serve.

## Consequences

Every pass can name the commit it acted on, which makes results reproducible and
attributable without Datum keeping any state of its own.

The repository becomes the control plane, and its access controls are the real security boundary. A
change merged there reaches every host its selectors match, applied by an agent running as root, so
review on the repository is the mechanism that limits blast radius, not a process nicety.

Reverting in Git is the recovery path for a bad change. That is a deliberate consequence
of having no rollback, and it means the usual Git workflow is also the incident response
workflow.

Fleet-wide questions are expensive. Answering which hosts have a package means resolving
every host in the repository, and there is nothing to query.

Every managed host needs read access to the whole repository, because resolution happens on the
machine and needs every `Layer` and `Host` document. Any secret committed to the repository is
therefore readable by every managed machine, which is why secret material has no place in the
current design instead of being an omission to fill with a convenient mechanism.

The word "initial" in earlier framing of this decision needs correcting. Git is the source of
desired state. What would supersede this decision is desired state originating somewhere other than
version control, and nothing in the design anticipates that.
