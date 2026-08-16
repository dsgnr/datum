# Repository layout

A Datum repository holds four types of document. `Fleet` declares the root of a
fleet, `Host` declares a machine and its classification, `Layer` declares a set of
configuration along with the hosts it applies to, and resource documents declare
the state to be reconciled.

!!! note "Proposed layout"

    Everything on this page is proposed. The directory names in particular are
    convention, not semantics, and Datum attaches no meaning to a directory
    being called `roles` or `environments`.

The layout shown here is the one a growing fleet tends towards, and it is larger than what
Datum requires. [The minimum](../repository/index.md#the-minimum) is four documents in as
few as one file.

## A worked layout

```text
fleet/
├── datum.yaml
├── base/
│   ├── layer.yaml
│   ├── packages.yaml
│   └── sysctl.yaml
├── environments/
│   ├── production/
│   │   ├── layer.yaml
│   │   └── sysctl.yaml
│   └── staging/
│       └── layer.yaml
├── sites/
│   └── london/
│       ├── layer.yaml
│       └── resolv.yaml
├── roles/
│   ├── web/
│   │   ├── layer.yaml
│   │   ├── nginx.yaml
│   │   └── files/
│   │       └── nginx.conf
│   └── database/
│       ├── layer.yaml
│       └── postgresql.yaml
└── hosts/
    ├── web-001/
    │   ├── host.yaml
    │   ├── layer.yaml
    │   └── nginx-tuning.yaml
    └── db-001/
        └── host.yaml
```

The shape carries no privileged meaning. Renaming `roles` to `functions`, or
flattening `environments/production` up a level, changes nothing about how Datum
resolves the repository, because resolution works from document content and not
from paths.

## Fleet

One `Fleet` document marks the root. Its directory and everything below it is the
search area for other documents.

```yaml title="fleet/datum.yaml"
datum: v1alpha1
type: Fleet

name: example

exclude:
  - "**/README.md"
```

The document is thin on purpose. Anything that could live in a `Layer` belongs
in a `Layer`, because settings that apply to a whole fleet are indistinguishable
from a layer whose matcher matches everything, and having two mechanisms for the
same thing would mean explaining which one wins.

## Host

A `Host` document declares that a machine exists and what it is for.

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

The name is the identity Datum uses to resolve desired state, and the labels are
the classification that matchers match against. Adding a machine to the fleet is
this document and nothing else, unless the machine needs configuration that no
existing layer provides.

## Layer

A `Layer` document declares a matcher and a precedence. It contains no resources
itself, because the resources belong to it by position in the tree.

```yaml title="fleet/roles/web/layer.yaml"
datum: v1alpha1
type: Layer

name: role-web

precedence: 30
match:
  labels:
    role: web
```

Every resource document belongs to the layer declared by the nearest `Layer`
document at or above it in the directory tree. `roles/web/nginx.yaml` belongs to
`role-web`, and so would `roles/web/tls/certificates.yaml`, because no closer
`Layer` document sits between them.

A resource document with no `Layer` document above it is an error, not a
resource that applies everywhere. Requiring an explicit layer means every
resource has a matcher, and no resource reaches a host by accident.

## Resource documents

Resource documents hold the state to reconcile. A single file may contain several
resources separated by the YAML document separator, which keeps things that belong
together in one place.

```yaml title="fleet/base/packages.yaml"
datum: v1alpha1
type: Package

name: curl

desired:
  state: present
---
datum: v1alpha1
type: Package

name: ca-certificates

desired:
  state: present
```

Files referenced by resources, such as the `source` of a `File`, are resolved
relative to the layer directory. `roles/web/nginx.yaml` referring to
`files/nginx.conf` means `roles/web/files/nginx.conf`, which keeps a layer
self-contained and movable.

## Conventional precedence values

Precedence is an integer on each `Layer`, and the conventional values live in
the files, not in Datum.

| Layer kind | Conventional precedence |
| ---------- | ----------------------- |
| Fleet-wide defaults | 0 |
| Environment | 10 |
| Site | 20 |
| Role | 30 |
| Single host | 100 |

Nothing enforces this table. A repository that gives sites higher precedence than
roles is expressing a different opinion about which classification should win, and
Datum has no view on whether that opinion is correct. The gaps between the values
exist so that a layer can be inserted between two others without renumbering
anything.

## The fleet directory and the repository

The fleet root is the directory holding the `Fleet` document. It does not have to be the
repository root, so a fleet can live in a subdirectory of a repository that holds other
things.

```text
monorepo/
  services/
  infra/
    fleet/
      datum.yaml
      base/
      hosts/
```

Every command takes the fleet directory, not the repository. `--repo infra/fleet` is what
the example above needs. The revision still comes from the repository the directory
belongs to, since git finds the root itself.

`datum affected` is the one command that reads history. It checks out each revision in a worktree,
which is a copy of the whole repository, and looks for the fleet at the same path below the root.
Walking the worktree root instead would find every fleet in the repository rather than the one asked
for.

## Discovery

Datum walks the fleet directory, reads every YAML document it finds, and dispatches
on `datum` and `type`. Paths matched by `exclude` are skipped. Files outside the fleet
root are not read, so unrelated YAML elsewhere in a repository is not a concern.

A document with an unrecognised `type` is an error and not something ignored,
because silently skipping a misspelled `type` would mean a resource quietly not
applying, which is the failure mode hardest to notice.

!!! note "Open question"

    Whether a repository should be able to contain more than one `Fleet` document,
    and what it would mean for a host to appear in two fleets, is undecided. A
    single fleet per repository is assumed throughout this documentation.
