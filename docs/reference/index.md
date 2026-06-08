# Reference

The pages in this section are lookup material. They assume the model is already understood and are
arranged for finding a field or a definition instead of reading through.

[Document format](manifest-format.md)
:   Every field of the `Fleet`, `Host` and `Layer` documents, the resource envelope,
    matcher syntax, merge rules, and the validation errors raised before a host is read.

[Command line interface](cli.md)
:   The proposed commands, their output, and their exit codes. No part of it is
    implemented.

[Glossary](glossary.md)
:   One entry per concept, with links to the page that specifies it, and a list of terms this
    documentation avoids.

## Per-type field references

Fields specific to a resource type are documented with that type, not here.

| Type | Reference |
| ---- | --------- |
| `Package` | [resources/types/package](../resources/types/package.md) |
| `File` | [resources/types/file](../resources/types/file.md) |
| `Directory` | [resources/types/directory](../resources/types/directory.md) |
| `Service` | [resources/types/service](../resources/types/service.md) |
| `User` | [resources/types/user](../resources/types/user.md) |
| `Group` | [resources/types/group](../resources/types/group.md) |
| `Sysctl` | [resources/types/sysctl](../resources/types/sysctl.md) |

## Stability

The `v1alpha1` schema carries no compatibility promise. Field names, defaults and
semantics can change without a migration path until the schema reaches a stable version,
and nothing described anywhere on this site is implemented yet.
