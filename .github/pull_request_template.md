<!-- CONTRIBUTING.md has the detail. This is the short version. -->

**What this changes**, and why.

**Statements in `docs/` this affects.** A change that makes a statement on the site false is not
finished until the statement is fixed, so name the pages you updated, or say that none needed it.

**Checks run.**

- [ ] `make lint` for a code change
- [ ] `make docs-check` for a documentation change
- [ ] The relevant provider suite, where one applies, such as `make test-apt` or `make test-systemd`

**For a new page**, confirm it is in `nav` in `zensical.toml`. Zensical builds pages that are not
in the navigation without warning, so one can otherwise exist and be unreachable.

**For a new open question**, confirm it appears both as an admonition where the gap is and as an
entry in `docs/development/open-questions.md`.
