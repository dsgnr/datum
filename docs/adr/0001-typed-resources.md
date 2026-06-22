# ADR-0001: Use typed resources

## Status

Accepted

## Context

Desired state has to be expressed in some form, and three were available.

Scripts or command sequences are the easiest to write and the hardest to reason about.
A script has no state to read before running and nothing to compare afterwards, so
nothing built on top of it can report what differs or confirm that a change worked.

Freeform data applied through templates, where the repository holds arbitrary key and value
structures rendered into configuration files, covers a lot of ground quickly. The problem is that
the data has no meaning to the tool. Rendering `port: 8080` into a template gives no way to ask
whether the port is currently 8080 on the host, since nothing records that the value corresponds to
observable state.

Typed resources, where each unit of configuration is an instance of a type with a known set of
fields, make the tool aware of what it is managing. A `File` type gives the implementation a defined
way to read a file's mode, so comparison, planning and verification are written once rather than per
piece of configuration.

The requirement that forces the choice is that Datum has to be able to report what
differs, what it intends to change, and whether the result was verified. All three need
state to be readable and comparable, which only the third option provides.

## Decision

Desired state is expressed as typed resources. Every resource has the same envelope,
being `apiVersion`, `kind`, `metadata`, `dependsOn` and `spec`, and each type defines
the fields its `spec` accepts.

A type is only admitted if its state can be read back from the host. Observability is the admission
criterion, not a quality to aim for, which rules out any type whose effect cannot be measured after
the fact.

## Consequences

Diffing, planning, ordering and verification are written once against the type
system rather than per piece of configuration. Adding a type inherits all of it.

Every capability needs a type before it can be used. Managing something Datum
has no type for means writing a provider and a type definition instead of a
script, which is a much higher barrier and is the main cost of this decision.

There is no escape hatch. A `Command` or `Exec` type would let anybody bypass
the type system, and because such a resource could not be diffed or verified,
its presence in a repository would weaken every guarantee for that host. The
barrier above is therefore permanent, not something to relieve later.

The set of manageable things is bounded by what providers can observe. Configuration
held in a database, in a service's runtime state, or in a format nothing can parse is
outside what Datum can manage, and recognising that early is better than discovering it
after a type has been designed.
