# Document format

Field reference for the four document types. The explanations behind these fields are in
the [fleet](../fleet/repository-layout.md) and [resources](../resources/index.md)
sections, and this page is for looking a field up.

!!! note "Proposed format"

    Every field on this page is proposed. `v1alpha1` means names, defaults and semantics
    can change without a migration path.

## Common structure

Every document opens with the same three keys.

| Key | Type | Required | Notes |
| --- | ---- | -------- | ----- |
| `datum` | string | Yes | Schema version. Currently only `v1alpha1`. |
| `type` | string | Yes | `Fleet`, `Host`, `Layer`, or a resource type. |
| `name` | string | Yes | Unique within its type. |

Everything else is top level and depends on the type. There is no wrapper object around identity and
no wrapper object around desired state, because a Datum document is a configuration file, not an
object submitted to an API.

An unrecognised `type` is an error and not a document to skip, because silently
ignoring a misspelling would mean configuration quietly not applying.

## Fleet

Marks the root of a fleet. One per repository.

```yaml title="fleet/datum.yaml"
datum: v1alpha1
type: Fleet

name: example

exclude:
  - "**/README.md"
```

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `exclude` | list of glob patterns | No | Paths under the fleet root that discovery skips. |

## Host

Declares a machine and its classification.

```yaml title="fleet/hosts/web-001/host.yaml"
datum: v1alpha1
type: Host

name: web-001
labels:
  environment: production
  site: london
  role: web
  architecture: amd64
```

`name` is the identity the agent claims and the value injected as the
`datum/host` label. There is no `desired`, because a host is classification and nothing
else.

Labels under the `datum/` prefix are reserved and setting one is an error.

| Reserved label | Value |
| -------------- | ----- |
| `datum/host` | The host's `name`, injected during resolution. |

## Layer

Declares a matcher and a precedence. Contains no resources.

```yaml title="fleet/roles/web/layer.yaml"
datum: v1alpha1
type: Layer

name: role-web

precedence: 30
match:
  labels:
    role: web
```

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `precedence` | integer | No, defaults to `0` | Higher values are applied later and win. |
| `match` | matcher | No | Omitted matches every host in the fleet. |

Resource documents belong to the layer declared by the nearest `Layer` document at or
above them in the directory tree. A resource document with no `Layer` above it is an
error.

Paths referenced by resources, such as `File.desired.source`, resolve relative to the
directory containing the `Layer` document.

### Matcher

```yaml
match:
  labels:
    role: web
    environment: production
  oneOf:
    site: [london, frankfurt]
  noneOf:
    tier: [legacy]
  has:
    - monitoring
  missing:
    - decommissioned
```

| Field | Type | Matches when |
| ----- | ---- | ------------ |
| `match.labels` | map of string to string | Each named label is present with exactly that value. |
| `match.oneOf` | map of string to list of strings | Each named label is present with one of the listed values. |
| `match.noneOf` | map of string to list of strings | Each named label is absent, or present with a value not listed. |
| `match.has` | list of strings | Each named label is present, whatever its value. |
| `match.missing` | list of strings | Each named label is absent. |

All five forms are optional and every form present must hold, so a matcher is a
conjunction. There is no alternation between whole matchers, and `oneOf` covers the
case that would otherwise need one.

An empty or omitted `match` matches every host in the fleet.

## Resource documents

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

| Key | Type | Required | Meaning |
| --- | ---- | -------- | ------- |
| `labels` | map of string to string | No | Resource labels. Not used for matching. |
| `requires` | list of resource references | No | Processed before this resource. |
| `restartOn` | list of resource references | No | Processed before this resource, and a change to any of them updates it. Currently only meaningful on `Service`. |
| `desired` | map | Yes | Type-specific desired state. |

`requires` and `restartOn` sit outside `desired` because both produce edges in the resource graph,
which is behaviour common to every type rather than desired state. Per-type `desired` fields are on
the [resource type](../resources/types/index.md) pages.

### Resource references

A reference is `Type[name]`, naming another resource in the same effective manifest.

```text
Package[nginx]
File[nginx-config]
Service[nginx]
```

A reference that does not resolve within the manifest is an error raised before the host
is read.

## Merge rules

How two layers contributing the same resource reference are combined.

| Field shape | Rule |
| ----------- | ---- |
| Scalar | Replaced by the higher-precedence value. |
| Map | Merged key by key, higher precedence winning per key. |
| List | Replaced entirely. |
| `requires` | Combined as a set. |
| `Service.restartOn` | Combined as a set. |

Two layers of equal precedence setting the same field to different values is an error
and no manifest is produced. Setting it to the same value is not a conflict.

## YAML conventions

Several files in this documentation contain more than one document, separated by `---`,
which keeps resources that belong together in one file.

Values that look numeric but are not must be quoted. `mode: "0640"` and `value:
"1"` are strings, and an unquoted `0640` is a number whose interpretation
depends on the YAML version, so it is rejected instead of guessed at.

## Validation summary

Errors raised before the host is read, in the order they are detected.

| Error | Raised by |
| ----- | --------- |
| Unrecognised `datum` or `type` | Discovery |
| Resource document with no `Layer` above it | Discovery |
| `Host` setting a `datum/` label | Discovery |
| Unrecognised form inside a `match` block | Discovery |
| `restartOn` on a type that does not support it | Discovery |
| Equal-precedence field conflict between layers | Fleet resolver |
| Unresolved resource reference in `requires` or `restartOn` | Graph builder |
| Dependency cycle | Graph builder |
| Two resources sharing a target identity | Graph builder |
