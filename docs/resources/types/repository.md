# Repository

`Repository` describes a package source the host may install from.

```yaml
datum: v1alpha1
type: Repository

name: nodesource

desired:
  state: present
  id: nodesource
  url: https://deb.nodesource.com/node_20.x
  suite: nodistro
  components: [main]
  signingKey: files/nodesource.asc
```

Target identity is `desired.id`, which is the identifier the package manager knows the source by, so two
`Repository` resources claiming one `id` on a host are a
[duplicate target](../identity.md).

## A package source is its own type

A package source can be added with a `File` resource, and this type exists to give it a declaration
of its own.

A sources list written by a `File` appears in review as an ordinary configuration change. It grants
root execution on that host to whoever serves the URL, since everything installed from that source
runs installation scripts as root, and the diff shows none of that. It is [one
case](../../security/threat-model.md#an-attacker-who-controls-upstream-content) of a change whose
effect is not visible in review.

A dedicated type states the grant. The signing key is a required field, so the key being trusted is
named where a reviewer is looking, and a source with no key has to declare that.

The type also keeps the format out of the repository. A sources list is written differently for apt,
dnf, zypper and apk, and a `File` resource would put that [distribution difference into fleet
configuration](../../providers/multi-distribution.md).

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `id` | string | Yes | The source's identifier on the host. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether the source should be configured. |
| `url` | string | Yes when present | Base URL the packages are served from. |
| `signingKey` | path or secret reference | Yes unless `unsigned` | The key packages from this source must be signed by. |
| `unsigned` | boolean | No, defaults to `false` | Accept a source with no signing key. |
| `enabled` | boolean | No, defaults to `true` | Whether the source is used for installs. |
| `priority` | integer | No | Preference relative to other sources, where the provider supports one. |
| `suite` | string | No | The apt suite, for providers that need one. |
| `components` | list of strings | No | The apt components, for providers that need them. |

`signingKey` takes a repository path in the same way
[`File.source`](file.md) does, and it may instead be a
[secret reference](../secrets.md) for a source whose key a fleet would rather not commit.

`suite` and `components` are the one place this type carries fields not every provider uses. An apt
source needs them and a dnf source has no equivalent. A provider that cannot express them [reports
that](../lifecycle.md#when-a-provider-cannot-do-something), so a field set on the wrong kind of
source is an error and not a dropped value.

## Unsigned sources are declared, never implied

```yaml
desired:
  state: present
  id: internal-mirror
  url: http://packages.internal/debian
  unsigned: true
```

Omitting `signingKey` is an error. Accepting an unsigned source requires `unsigned: true`, so the
choice appears in a diff as a word.

A missing field is hard to see in a diff and a present one is not. `unsigned: true` exists to put
the choice in front of a reviewer scanning a change for risk.

!!! note "Security consideration"

    A trusted package source is root execution on every host that has it, deferred until something
    is installed from it. The explicit field puts the grant in the diff, and review policy is what
    decides who may approve one.

    Datum verifies nothing about package content. Signature checking belongs to the package manager,
    against the key this resource configures. This type controls which key is trusted.

## Observation

| Field | Observed |
| ----- | -------- |
| `exists` | Whether the source is configured. |
| `url` | The base URL currently configured. |
| `signingKey` | A digest of the key currently trusted. |
| `enabled` | Whether it is used for installs. |

The key is observed as a digest, so a rotated or replaced key shows as drift on a field nobody would
compare by hand.

## Dependencies

A `Package` installed from a source declared this way needs an edge to it, and Datum
[infers none](../dependencies.md).

```yaml
datum: v1alpha1
type: Package

name: nodejs

requires:
  - Repository[nodesource]

desired:
  state: present
```

Without the edge the install works on a host that already has the source and fails on a fresh one.
This is the [ordering mistake](../dependencies.md#where-dependencies-come-from) the design leaves to
the repository rather than inferring.

## Removal

Declaring a source `absent` removes the source and does not remove packages installed from it.

Those packages stay installed, keep working, and stop receiving updates. Removing them would mean
treating anything traceable to one source as unwanted. That decision is expressed in the repository
as `Package` resources declared absent.

!!! note "Proposed behaviour"

    This type does not exist. It is specified ahead of the others because it is the type most likely to
    be needed first, given that installing anything outside a distribution's default sources currently
    has no representation, and because the security argument for it is independent of the convenience
    one.
