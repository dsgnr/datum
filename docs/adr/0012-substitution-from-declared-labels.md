# ADR-0012: Substitute declared label values, and nothing else

Status: Accepted

## Context

Hosts differ in ways too small to justify separate files. Forty web servers can share one nginx
configuration apart from a `server_name`, and a resolver configuration can differ between sites only in
the addresses it lists.

With no substitution at all there are two ways to express that, and both are bad at fleet scale. One
`File` resource per host, each pointing at a near-identical source file, means forty copies in Git where
a reviewer cannot see that thirty-nine of them are meant to be identical. A fragment per host dropped
into a `conf.d` directory means relying on directory contents that
[nothing purges](0009-declared-only-ownership.md), so a stale fragment keeps being loaded after the
resource that placed it is gone.

The position taken previously was that templating had no place in the design, on the grounds that
rendering content from host facts would reintroduce a dependency on observed state during resolution.
That reasoning conflated two different sources of values.

Rendering from **observed host facts** would indeed break the model. Resolution is a pure function of
repository content and a host name, which is what lets a manifest be produced from a checkout on a
laptop, lets `datum validate` resolve every host in CI, and lets a
[manifest digest](../fleet/effective-manifests.md#content-addressing) identify desired state.

Rendering from **declared labels** breaks none of it. Labels are repository content. Resolution already
reads them in order to evaluate matchers, so substituting their values changes nothing about what
resolution depends on.

## Decision

Values declared in a `Host` document's labels may be substituted into desired state. Nothing else may.

```text
{{ labels.site }}        a declared label value
{{ host }}               the host name
```

The following are all refused, and the refusals are the substance of this decision, not omissions
from a first version.

No expressions, arithmetic, comparisons or function calls, and no conditionals or loops. No values
read from the host. No values read from the environment, another file, or anything outside the
document being resolved. No nesting, so a substituted value is used literally and is never itself
rendered.

A reference to a label the host does not declare is an error raised during manifest validation, before
the host is read.

Matchers are not substituted. A `match` block determines which hosts a layer applies to, and
rendering it against a host's labels to establish whether it matches would be circular.

## Consequences

Resolution stays a pure function of the repository and a host name, so everything built on that property
continues to hold unchanged.

Manifest digests become more specific, which is correct. Two hosts differing only in a substituted value
now have different manifests and therefore different digests, and a
[change affecting one site](../repository/validating-changes.md#which-hosts-a-change-would-affect) shows
as affecting exactly the hosts in that site.

The absence of conditionals is the constraint that will be argued with. A fleet wanting a block of
configuration present for some hosts and absent for others cannot express it by rendering, and has
to express it as a separate resource in a [narrowly matched layer](../fleet/labels-and-matchers.md),
which is more verbose and visible in the repository rather than hidden inside a file.

That verbosity is the intended trade. Every configuration language that started with substitution and
added conditionals arrived at a programming language embedded in configuration, at which point
predicting what a host will receive requires executing it, and
[behaviour derivable by reading](../introduction/design-principles.md#understandable) stops being true.
The related refusal in [ADR-0011](0011-no-command-execution-from-desired-state.md) exists for the same
reason, and a substitution language with control flow would reintroduce through rendering what that
decision removed from execution.

Reviewers gain something and lose something. A rendered file is one file in review instead of forty,
and the value a particular host will receive is no longer literally present in the repository. That
is why `datum explain` reports the rendered result alongside the source of every value.
