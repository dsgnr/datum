# ADR-0002: Separate resources from providers

## Status

Accepted

## Context

Datum is intended to manage several Linux distributions from one repository, and the
distributions disagree about almost everything below the level of intent. Installing a
package means `apt-get install` on Debian, `dnf install` on Fedora, `apk add` on Alpine
and `pacman -S` on Arch, with different version formats, different output to parse and
different failure modes.

The question is where that difference is allowed to live.

The pattern that emerges without a decision is a conditional at the point of use. The
first time a repository has to handle two distributions somebody writes a check against
the operating system, and it works. The checks then multiply, appearing in the
configuration itself, in templates, and in whatever mechanism decides precedence. The
resource model ends up shaped by whichever package manager was implemented first, and
the distribution check becomes something every contributor has to think about.

An alternative is one resource type per packaging system, so `AptPackage` and
`DnfPackage` exist separately. This is honest about the differences and makes every
repository distribution-specific, which defeats the purpose of describing a mixed fleet
from one place.

## Decision

Resource types describe intent and are distribution neutral. Providers implement that
intent for a particular class of system, and a provider is the only component permitted
to know what `apt` is.

A `Package` resource describes package state. It does not mean `apt`, and nothing in a
resource document names a provider. Which provider runs is derived from the host after
desired state has been resolved.

Nothing above the provider boundary reads `/etc/os-release` or branches on a
distribution. That applies to the fleet resolver, the graph builder, the planner and the
reconciler without exception.

## Consequences

The resolver, graph builder, planner and reconciler are written once and behave
identically on every system, which is the property the boundary exists to protect.

The same repository can describe hosts running different distributions, and
adding support for a new one means writing providers rather than touching
configuration.

Provider interfaces have to be wide enough for the awkward cases, and getting one too narrow shows
up as pressure to leak a distribution check upward. A check appearing above the boundary is treated
as evidence that a provider interface is missing, not as a pragmatic shortcut.

Differences that will not fit behind a single field have to surface somewhere. They are
allowed in two places, being inside a provider where they are invisible, or in the
repository where a narrower selector makes them explicit. A field that silently behaves
differently depending on the host is not allowed, because configuration reviewed once
and believed uniform is the failure this decision exists to prevent.

Some differences cannot be absorbed at all. Package naming, version string grammars and
the mechanism for holding a package at a version differ in ways no provider can hide, and
those costs land on whoever maintains the repository.
