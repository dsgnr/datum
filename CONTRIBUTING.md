# Contributing to Datum

Datum is built specification first, and a complete pass now runs end to end. Contributions to the
specification and to the code are both useful, and the design is still where a problem is cheapest
to fix. The fuller version of this guide is on the documentation site under Development, and this
file covers what is needed to make a change.

## What is useful now

Arguing against a decision, with a reason. Every architecture decision record in `docs/adr/` has a
Consequences section, and an argument that one of them underestimated a cost is worth raising.

Finding a case the model cannot express. The design is narrow, and whether it is too narrow is the
open question behind most of the others. A concrete example of configuration that cannot be
described, or a host that cannot be classified, is directly actionable.

Answering an open question. `docs/development/open-questions.md` lists more than sixty, several of
which need operational experience of a particular distribution rather than further design work.

Finding an inconsistency. Examples across the site describe one fleet and the terminology is meant
to be identical everywhere, so two pages disagreeing is a defect.

Improving an explanation. A page that is correct and hard to follow is not finished.

## What is premature

Nothing central. No server, no APIs, no database.

Providers are welcome, and so is anything that keeps the engine portable. A provider that reaches
around the [boundary](docs/providers/index.md) or writes without following the [safety
rules](docs/security/provider-safety.md) is not. Those rules were written before the code, and a
review holds a provider to them.

A change that makes a statement in `docs/` false is not finished until the statement is fixed. The
documentation is the specification, so the two drifting apart is a defect in both.

## Making a change

```bash
make docs-install    # create .venv and install the pinned toolchain
make docs-serve      # preview the site locally
make docs-check      # build with --strict, which is what CI runs

make build           # build ./bin/datum
make test            # run the Go tests
make lint            # formatting, vet and tests, which is what CI runs
```

`make docs-check` must pass before a documentation change is ready. Strict mode fails on broken internal links and
unknown heading anchors.

Read the rendered page rather than only the source. Tables and admonitions are easy to get subtly
wrong in Markdown and obvious in a browser.

Adding a page means adding it to `nav` in `zensical.toml` in the same change. Zensical builds pages
that are not in the navigation and does not warn about them, so a new page can otherwise exist and
be unreachable.

## Writing

Write as an engineer documenting a system for other engineers. State what something does, what it
costs, and where it breaks. Avoid marketing language, and do not describe unimplemented behaviour
as though it works.

Where behaviour is undecided, mark it rather than guessing.

| Admonition | Used for |
| ---------- | -------- |
| `!!! note "Proposed behaviour"` | A concrete design that has not been accepted |
| `!!! note "Open question"` | A known gap with no decision yet |
| `!!! note "Planned"` | Accepted in principle, not specified |
| `!!! note "Security consideration"` | Something with a security consequence worth stating |
| `!!! note "Important limitation"` | Behaviour that will surprise somebody |

A new open question needs both the admonition where the gap is and an entry in
`docs/development/open-questions.md`. Resolving one means removing the entry.

Keep examples consistent with the rest of the site. Changing the proposed configuration syntax
means updating every example rather than leaving two syntaxes in circulation.

One concept has one home. Where a term is used in several sections, the section that owns it defines
it and the others link to it.

## Changing a decision

Something marked **Accepted** is recorded in an architecture decision record, and changing it means
superseding that record rather than editing it. The original keeps its reasoning and has its status
changed, and the new record explains what changed and why.

Something marked **Proposed** can be edited directly. Proposed designs are expected to move.

Corrections to wording in an accepted record are fine. Changing what it decided is not.

## Commits

One-line [Conventional Commits](https://www.conventionalcommits.org/) subjects, one coherent change
per commit.

```text
docs(fleet): describe host labels and matchers
docs(adr): record typed resource decision
fix(docs): correct matcher precedence example
refactor(docs): simplify fleet terminology
```

Before each commit, inspect the diff, drop unrelated changes, and run `make docs-check` for
documentation or `make lint` for code. The repository
should build at every commit.

## Licence

Contributions are made under the [Apache License 2.0](LICENSE).
