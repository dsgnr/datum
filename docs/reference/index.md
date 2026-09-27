---
description: "Find Datum CLI commands, agent configuration, document formats, resource fields, status definitions, provider support and interface stability."
seo_title: "Datum command and configuration reference"
---

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
:   Implemented commands, their flags, output and exit codes, with notes where behaviour is still
    proposed.

[Status](status.md)
:   The implemented per-host status report and the proposed fleet-wide status model.

[Interfaces and stability](stability.md)
:   Which interfaces become contracts, and when. Everything is unstable before 1.0.

[Glossary](glossary.md)
:   One entry per concept, with links to the page that specifies it, and a list of terms this
    documentation avoids.

[State and lifecycle](../concepts/state.md)
:   Resource and host state vocabulary, including the proposed reboot states.

[Provider support matrix](../providers/support-matrix.md)
:   Implemented providers and their platform coverage.

## Per-type field references

Fields specific to each resource type are listed under Resource types in this section.

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
semantics can change without a compatibility or migration promise until the schema reaches a stable
version. See [project status](../introduction/project-status.md) for implementation coverage.
