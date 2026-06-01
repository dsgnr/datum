# Resources

A resource is a typed description of one thing on a host. A resource states the
condition that thing should be in, and the steps for reaching it are the
provider's implementation.

```yaml
apiVersion: datum.dev/v1alpha1
kind: File

metadata:
  name: nginx-config

dependsOn:
  - Package[nginx]

spec:
  path: /etc/nginx/nginx.conf
  owner: root
  group: root
  mode: "0640"
  source: files/nginx.conf
```

## The common envelope

Every resource document has the same four top-level keys, and only `spec` varies by
type.

| Key | Meaning |
| --- | ------- |
| `apiVersion` | The API group and version, currently `datum.dev/v1alpha1`. |
| `kind` | The resource type. |
| `metadata` | `name`, which forms the resource reference, and optional `labels`. |
| `dependsOn` | Resource references that must be processed before this one. |
| `spec` | Type-specific desired state. |

`dependsOn` sits outside `spec` because ordering is behaviour shared by every type
rather than something a type defines. Putting it inside `spec` would mean every type's
schema repeated it, and a type could plausibly give it different semantics.

## Settling common behaviour first

Four questions have to be answered the same way for every type, and answering them
badly is expensive because every type added afterwards inherits the answer.

[Resource lifecycle](lifecycle.md)
:   What observe, diff, plan, apply and verify each mean, which of them a provider
    performs, and what idempotency requires.

[Resource identity](identity.md)
:   The difference between how a resource is referred to inside Datum and what it
    manages on the host, and why one identity cannot do both jobs.

[Dependencies](dependencies.md)
:   What `dependsOn` guarantees, how change reaction differs from ordering, and why
    Datum does not infer dependencies from paths or ownership.

[Conflicts](conflicts.md)
:   What happens when two resources manage the same thing, and why none of the
    available automatic resolutions is acceptable.

[Resource types](types/index.md)
:   The seven proposed types, their fields, and the provider differences each one
    cannot hide.

## What is not a resource

Anything whose state cannot be read back from the host. That rules out running a
command, which is the escape hatch configuration management tools usually grow,
and it is the criterion the type set was chosen against, not a rule applied
afterwards.

The practical test is whether the five operations can all be performed. A
command has nothing to observe before running, nothing to diff, and nothing to
verify afterwards beyond its exit status, which says that it ran and not that
the host is now correct.

## What resources do not describe

A resource describes one thing and holds no view of the machine around it. A `File`
resource does not create its parent directory, a `Service` resource does not install
its package, and a `User` resource does not create its home directory.

Each of those is a separate resource with a dependency edge, which makes the
relationship visible in the manifest and in the plan. The alternative of implicit
behaviour would mean the plan contained actions nobody declared, and the ordering would
depend on rules that are not in the repository.

!!! note "Proposed design"

    The common envelope and the lifecycle are the parts of this section closest to
    settled, and identity is recorded as
    [ADR-0008](../adr/0008-resource-reference-and-target-identity.md). The type schemas are
    proposed and are where most of the remaining uncertainty sits.
