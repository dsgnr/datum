# Resources

A resource is a typed description of one thing on a host. A resource states the
condition that thing should be in, and the steps for reaching it are the
provider's implementation.

```yaml
datum: v1alpha1
type: File

name: nginx-config

requires:
  - Package[nginx]

desired:
  path: /etc/nginx/nginx.conf
  owner: root
  group: root
  mode: "0640"
  source: files/nginx.conf
```

## The common envelope

Every document shares the same three opening keys, and only `desired` varies by type.

| Key | Meaning |
| --- | ------- |
| `datum` | Schema version, currently `v1alpha1`. |
| `type` | The resource type. |
| `name` | Combined with `type`, forms the resource reference. |
| `requires` | Resource references processed before this one. |
| `restartOn` | Resource references processed before this one, whose change also restarts it. |
| `reloadOn` | Resource references processed before this one, whose change also reloads it. |
| `desired` | Type-specific desired state. |

The envelope is flat on purpose. A Datum document is a configuration file, not an object submitted
to an API, so there is no wrapper around identity and no wrapper around desired state. Anything not
inside `desired` is behaviour every type shares.

`requires` and `restartOn` therefore sit outside `desired`, because both produce edges in
the resource graph and neither describes a condition the host should be in. Putting them
inside `desired` would mean every type's schema repeated them, and a type could plausibly
give them different meanings.

## Settling common behaviour first

Four questions have to be answered the same way for every type, and answering them
badly is expensive because every type added afterwards inherits the answer.

[Resource lifecycle](lifecycle.md)
:   What observe, diff, plan, apply and verify each mean, which of them a provider
    performs, and what idempotency requires.

[Ownership](ownership.md)
:   What a declaration claims about targets it does not mention, and the difference between an
    assertive resource and an authoritative set.

[Resource identity](identity.md)
:   The difference between how a resource is referred to inside Datum and what it
    manages on the host, and why one identity cannot do both jobs.

[Dependencies](dependencies.md)
:   What `requires` guarantees, how change reaction differs from ordering, and why dependencies
    are declared rather than inferred.

[Conflicts](conflicts.md)
:   What happens when two resources manage the same thing, and why none of the
    available automatic resolutions is acceptable.

[Secret references](secrets.md)
:   How desired state names a credential without containing one, and where the value comes from.

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
