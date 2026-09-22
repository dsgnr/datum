---
name: Report a bug
about: Datum did something other than what the documentation says
labels: bug
---

<!-- Not for vulnerabilities. Those go through a private advisory, see SECURITY.md. -->

**What the documentation says.** A link or a quote. The documentation is the specification, so a
difference between it and the behaviour is a defect in one of them.

**What happened instead.**

**How to reproduce it.** The smallest repository content that shows it, and the command you ran.

**Version and host.** The output of `datum version`, plus the distribution and its version.

```text
$ datum version

```

**Anything relevant from the pass.** `datum status --output json`, or the plan from
`datum plan`, with any sensitive values removed.
