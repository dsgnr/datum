# Document format

Field reference for the four document kinds. The explanations behind these fields are in
the [fleet](../fleet/repository-layout.md) and [resources](../resources/index.md)
sections, and this page is for looking a field up.

!!! note "Proposed format"

    Every field on this page is proposed. `v1alpha1` means names, defaults and semantics
    can change without a migration path.

## Common structure

Every document has the same three leading keys.

| Key | Type | Required | Notes |
| --- | ---- | -------- | ----- |
| `apiVersion` | string | Yes | Currently only `datum.dev/v1alpha1`. |
| `kind` | string | Yes | `Fleet`, `Host`, `Layer`, or a resource type. |
| `metadata.name` | string | Yes | Unique within its kind. |
| `metadata.labels` | map of string to string | No | Flat strings only. |

An unrecognised `kind` is an error rather than a document to skip, because silently
ignoring a misspelling would mean configuration quietly not applying.

## Fleet

Marks the root of a fleet. One per repository.

```yaml title="fleet/datum.yaml"
apiVersion: datum.dev/v1alpha1
kind: Fleet

metadata:
  name: example

spec:
  exclude:
    - "**/README.md"
```

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `spec.exclude` | list of glob patterns | No | Paths under the fleet root that discovery skips. |

## Host

Declares a machine and its classification.

```yaml title="fleet/hosts/web-001/host.yaml"
apiVersion: datum.dev/v1alpha1
kind: Host

metadata:
  name: web-001
  labels:
    environment: production
    site: london
    role: web
    architecture: amd64
```

`metadata.name` is the identity the agent claims and the value injected as the
`datum.dev/host` label. There is no `spec`, because a host is classification and nothing
else.

Labels under the `datum.dev/` prefix are reserved and setting one is an error.

| Reserved label | Value |
| -------------- | ----- |
| `datum.dev/host` | The host's `metadata.name`, injected during resolution. |

## Layer

Declares a selector and a precedence. Contains no resources.

```yaml title="fleet/roles/web/layer.yaml"
apiVersion: datum.dev/v1alpha1
kind: Layer

metadata:
  name: role-web

spec:
  precedence: 30
  selector:
    matchLabels:
      role: web
```

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `spec.precedence` | integer | No, defaults to `0` | Higher values are applied later and win. |
| `spec.selector` | selector | No | Omitted matches every host in the fleet. |

Resource documents belong to the layer declared by the nearest `Layer` document at or
above them in the directory tree. A resource document with no `Layer` above it is an
error.

Paths referenced by resources, such as `File.spec.source`, resolve relative to the
directory containing the `Layer` document.

### Selector

```yaml
selector:
  matchLabels:
    role: web
    environment: production
  matchExpressions:
    - key: site
      operator: In
      values: [london, frankfurt]
    - key: legacy
      operator: DoesNotExist
```

| Field | Type | Meaning |
| ----- | ---- | ------- |
| `matchLabels` | map of string to string | Each label must be present with exactly that value. |
| `matchExpressions` | list | Each expression must match. |
| `matchExpressions[].key` | string | The label key being tested. |
| `matchExpressions[].operator` | `In`, `NotIn`, `Exists`, `DoesNotExist` | The test applied. |
| `matchExpressions[].values` | list of strings | Required for `In` and `NotIn`, forbidden otherwise. |

| Operator | Matches when |
| -------- | ------------ |
| `In` | The label exists and its value is one of `values`. |
| `NotIn` | The label is absent, or present with a value not in `values`. |
| `Exists` | The label is present, whatever its value. |
| `DoesNotExist` | The label is absent. |

Every term in both parts must match. There is no alternation between whole selectors.

## Resource documents

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

| Key | Type | Required | Meaning |
| --- | ---- | -------- | ------- |
| `dependsOn` | list of resource references | No | Processed before this resource. |
| `spec` | map | Yes | Type-specific desired state. |

`dependsOn` sits outside `spec` because ordering is common to every type. Per-type
`spec` fields are on the [resource type](../resources/types/index.md) pages.

### Resource references

A reference is `Kind[name]`, naming another resource in the same effective manifest.

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
| `dependsOn` | Combined as a set. |
| `Service.spec.restartOn` | Combined as a set. |

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
| Unrecognised `apiVersion` or `kind` | Discovery |
| Resource document with no `Layer` above it | Discovery |
| `Host` setting a `datum.dev/` label | Discovery |
| `matchExpressions` with `values` on `Exists` or `DoesNotExist` | Discovery |
| Equal-precedence field conflict between layers | Fleet resolver |
| Unresolved resource reference in `dependsOn` or `restartOn` | Graph builder |
| Dependency cycle | Graph builder |
| Two resources sharing a target identity | Graph builder |
