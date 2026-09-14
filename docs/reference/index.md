# Reference

The pages in this section are lookup material. They assume the model is already understood and are
arranged for finding a field or a definition instead of reading through.

[Document format](manifest-format.md)
:   Every field of the `Fleet`, `Host` and `Layer` documents, the resource envelope,
    matcher syntax, merge rules, and the validation errors raised before a host is read.

[Agent configuration](agent-config.md)
:   Every key in `/etc/datum/agent.yaml`, with its default, and the one file that decides how much a
    machine trusts Datum.

[Command line interface](cli.md)
:   The proposed commands, their output, and their exit codes. No part of it is
    implemented.

[Status](status.md)
:   The host and fleet status model, designed before implementation so the architecture
    preserves the right information.

[Interfaces and stability](stability.md)
:   Which interfaces become contracts, and when. Everything is unstable before 1.0.

[Glossary](glossary.md)
:   One entry per concept, with links to the page that specifies it, and a list of terms this
    documentation avoids.

## Per-type field references

Fields specific to a resource type are documented with that type, not here.

| Domain | Type | Reference |
| ------ | ---- | --------- |
| Core | `Package` | [resources/types/package](../resources/types/package.md) |
| Core | `Repository` | [resources/types/repository](../resources/types/repository.md) |
| Core | `File` | [resources/types/file](../resources/types/file.md) |
| Core | `Directory` | [resources/types/directory](../resources/types/directory.md) |
| Core | `Symlink` | [resources/types/symlink](../resources/types/symlink.md) |
| Identity | `User` | [resources/types/user](../resources/types/user.md) |
| Identity | `Group` | [resources/types/group](../resources/types/group.md) |
| Runtime | `Service` | [resources/types/service](../resources/types/service.md) |
| Kernel | `Sysctl` | [resources/types/sysctl](../resources/types/sysctl.md) |

## Stability

The `v1alpha1` schema carries no compatibility promise. Field names, defaults and
semantics can change without a migration path until the schema reaches a stable version,
and nothing described anywhere on this site is implemented yet.
