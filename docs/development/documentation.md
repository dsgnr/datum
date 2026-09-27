# Working on the documentation

The site is built with [Zensical](https://zensical.org/) and configured through
`zensical.toml`.

## Building and previewing

```bash
make docs-install    # create .venv and install the pinned toolchain
make docs-serve      # preview the site locally with live reload
make docs-build      # build into ./site
make docs-check      # build with --strict, which is what CI runs
```

The `Makefile` targets wrap the toolchain and add nothing to it. Running Zensical directly works the
same way.

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements-docs.txt
zensical serve
```

The version in `requirements-docs.txt` is pinned so that local builds and CI produce the same
output. Bumping it belongs in its own commit, after checking the rendered result.

## Strict mode

`zensical build --strict` fails on any warning, and link validation is enabled in
`zensical.toml`.

```toml
[project.validation]
invalid_links = true
invalid_link_anchors = true
unresolved_references = true
unresolved_footnotes = true
```

A link to a page that does not exist, or to a heading anchor that does not exist, fails the
build. This catches the most common way documentation rots, which is a heading being renamed
while links to it are left behind.

One thing strict mode does not catch is a page missing from the navigation. Zensical builds
pages that are not in `nav` and does not warn, so a new page can exist and be unreachable.
Adding a page means adding it to `nav` in the same change.

## Structure

Navigation is organised around four reader tasks, with Home as a separate link. It is
explicit in `zensical.toml`; directory names describe subject areas and do not determine
the sidebar hierarchy.

| Navigation group | Holds |
| ---------------- | ----- |
| Getting started | Quickstart, installation, introduction and project status |
| User guide | Core concepts, operating modes, fleet and repository authoring, resource behaviour, reconciliation and monitoring |
| Reference | Commands, configuration, document and resource fields, status, supported providers and glossary |
| Design | Principles, architecture, provider internals, security, scenario journeys, development and decisions |

Keep short definitions together in `concepts/index.md`. Detailed operating modes and state
vocabulary have their own pages. Resource behaviour belongs in the user guide, while the
per-type field pages in `resources/types/` appear under Reference. Journeys are design
scenarios; hands-on instructions belong in Getting started.

Subject directories generally have an `index.md` that introduces and links to their pages.
Keep those links useful even when related pages appear in different navigation groups.

One concept has one home. Where a term is used in several sections, the section that owns it
defines it and the others link to it, because a definition in two places becomes two
definitions.

## Writing conventions

Write as an engineer documenting a system for other engineers. State what something does,
what it costs, and where it breaks.

Say what is not decided. Where behaviour is unresolved, mark it instead of describing a guess as
though it were settled.

| Admonition | Used for |
| ---------- | -------- |
| `!!! note "Proposed behaviour"` | A concrete design that has not been accepted |
| `!!! note "Open question"` | A known gap with no decision yet |
| `!!! note "Planned"` | Accepted in principle, not specified |
| `!!! note "Security consideration"` | Something with a security consequence |
| `!!! note "Important limitation"` | Behaviour that will surprise somebody |
| `!!! note "Implementation status"` | A reminder that something does not exist |

Admonitions are for those cases. A page covered in coloured boxes is harder to read than one
with none, so ordinary explanation stays in prose.

Every new open question gets an entry in [open questions](open-questions.md), and resolving one
means removing the entry.

Keep examples consistent. The configuration examples across the site describe one fleet, with
`web-001` as the example host and nginx as the example service. Changing the proposed syntax means
updating every example rather than leaving two syntaxes in circulation.

Avoid marketing language, and avoid asserting that something works when nothing is
implemented.

## Search metadata and moved pages

Give entry pages and reference pages a concise `description` in YAML frontmatter that
summarises their actual content. Use `seo_title` when the search title needs more context
than the sidebar label, for example `Install and run the Linux agent - Datum`. Keep titles
distinct and avoid claims beyond the current implementation.

`overrides/main.html` uses these fields for search titles and social previews, with the
site description as a fallback. It also adds website structured data to the homepage.
Canonical URLs and the sitemap come from Zensical; `docs/robots.txt` advertises the sitemap.

When merging or moving a published page, preserve its old URL with a static HTML redirect,
as in `concepts/desired-state/index.html`. Use an immediate meta refresh, a canonical URL
for the destination page, and a visible link. Static redirects stay outside the navigation
and sitemap. Check the generated redirect and its destination anchor after a clean build.

## Diagrams

Mermaid is available through the fenced block configuration in `zensical.toml`.

````text
```mermaid
graph TD
  a[Repository] --> b[Fleet resolver]
```
````

A diagram should communicate a relationship or a sequence that prose handles badly. Diagrams that
restate the surrounding paragraph should be deleted, and every diagram needs enough prose around it
that the page still makes sense without it.

Plain text diagrams in fenced blocks are often clearer than Mermaid for short pipelines, and
they diff readably.

Mermaid is rendered in the browser, not at build time, so a syntax error produces a diagram that
silently fails to draw and a build that reports nothing. `make docs-mermaid` parses every block with
the real parser and is part of `make docs-check`.

The common mistake is a node identifier that is also a keyword. A `graph TD` diagram cannot
have a node called `graph`, so name it `rgraph` or similar.

## Before committing

Run `make docs-check`, which builds with `--strict` and parses every diagram. Read the rendered page
and not only the source, because tables and admonitions are easy to get subtly wrong in Markdown and
obvious in the browser.

Keep commits small and coherent, one improvement each, with a one-line [Conventional
Commits](https://www.conventionalcommits.org/) subject.

```text
docs(fleet): describe host labels and matchers
docs(adr): record typed resource decision
fix(docs): correct matcher precedence example
refactor(docs): simplify fleet terminology
```

The repository should build at every commit, so a change that adds a page adds its navigation
entry and any links to it in the same commit.
