# ADR-0008: Separate resource references from target identities

## Status

Accepted

## Context

A resource needs a name, and the obvious assumption is that one name can do both of the jobs
a name is asked to do here, which turns out not to hold.

The first job is naming the resource inside Datum, so that `requires` can refer to it and so
that two layers contributing to the same resource can be recognised as doing so. The second is
naming what the resource manages on the host, so that two resources fighting over the same
file can be detected.

Using the host target as the only name breaks composition and readability. A `File` would be
named by its path, so every dependency reference would be an absolute path, and a layer
wanting to override one file's mode would have to repeat the path as a name. It also fails
where the same logical file sits at different paths on different distributions, because the
name would change with the path.

Using an arbitrary name as the only identity removes the ability to detect overlap. Two `File`
resources with different names can declare the same path, both would apply, the second would
overwrite the first, and which one won would depend on plan order. The file would then be
rewritten on every pass as the two took turns, which reads as permanent drift with no visible
cause.

## Decision

Every resource has two identities.

The resource reference is `type` and `name` together, written `File[nginx-config]`. It
is unique within an effective manifest, it is what `requires` and `restartOn` refer to, and it
is what identifies a resource across layers during composition.

The target identity is what the resource manages on the host. For `File` and `Directory` it is
`desired.path`, and for `Package`, `Service`, `User`, `Group` and `Sysctl` it is `name`,
because those types have a natural key that reads well as a name.

Two resources sharing a target identity is a conflict, detected by the graph builder before the
host is read.

## Consequences

Dependency references stay short and stable. A reference does not change when a path changes,
and it does not have to be qualified by layer or host.

Layers can override one field of a resource without restating what identifies it on the host.
A host layer contributing `mode` to `File[nginx-config]` does not repeat the path.

Overlap is detectable. Two resources managing the same thing produce an error naming both resources
and the layers that contributed them, rather than a host that quietly contradicts its own manifest.

The mapping is not uniform across types, which is the cost. Knowing what identifies a `File` on the
host means knowing that `File` uses `desired.path` while `Package` uses `name`, so the per-type
table has to be looked up and cannot be derived.

Renaming has two distinct meanings. Changing `name` creates a different resource and breaks every
reference to the old name, which fails loudly. Changing `desired.path` leaves the old path unmanaged
and untouched, which fails silently, so moving a managed file is a two-step change in the
repository, not something Datum works out.

Whether `Package` should gain a `desired.package` field, letting one reference mean `apache2` on
Debian and `httpd` on Fedora, is left open. It would put a distribution difference in the
resource document, and whether that belongs there or inside a provider is not settled.
