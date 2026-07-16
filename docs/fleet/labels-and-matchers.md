# Labels and matchers

Labels classify a host. Matchers decide which layers apply to it. Between them
they are the only mechanism by which configuration reaches a machine, so there is
no inheritance hierarchy, no include directive, and no host list to maintain
alongside them.

## Labels are declared in the repository

A host's labels come from its `Host` document and nowhere else.

```yaml
datum: v1alpha1
type: Host

name: web-001
labels:
  environment: production
  site: london
  role: web
  architecture: amd64
```

Labels are strings on both sides. There are no typed values, no lists as values,
and no nesting, because a label whose value needs structure is a sign that the
structure belongs in a resource rather than in classification.

!!! note "Security consideration"

    A host never supplies its own labels. Reading classification from the machine
    would let a compromised host relabel itself as `environment: production` and
    receive configuration intended for production, which is why labels come from
    the repository and a host only proves which machine it is.

## Reserved labels

The resolver injects one label derived from the `Host` document.

| Label | Value |
| ----- | ----- |
| `datum/host` | The host's `name` |

This exists so that host-specific configuration needs no special mechanism. A
layer targeting one machine is an ordinary layer with an ordinary matcher.

```yaml
datum: v1alpha1
type: Layer

name: host-web-001

precedence: 100
match:
  labels:
    datum/host: web-001
```

The `datum/` prefix is reserved. A `Host` document setting a label under it is
an error, so that injected values cannot be shadowed.

!!! note "Open question"

    Facts observed on the host, such as the distribution and version reported by
    `/etc/os-release`, are not available as labels. Making them available would
    let a layer target Debian hosts without anybody declaring which hosts run
    Debian, and it would also mean desired state could no longer be resolved
    without reaching the machine. The current position is that a host needing
    distribution-specific treatment carries a declared label saying so, and that
    provider selection handles the rest.

## Matcher semantics

A matcher is a `match` block with five optional forms, each testing labels a
different way.

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

| Form | Matches when |
| ---- | ------------ |
| `labels` | Each named label is present with exactly that value. |
| `oneOf` | Each named label is present with one of the listed values. |
| `noneOf` | Each named label is absent, or present with a value not listed. |
| `has` | Each named label is present, whatever its value. |
| `missing` | Each named label is absent. |

Every form present has to hold, and every term within a form has to hold, so a
matcher is one large conjunction. Most layers need only `labels`, and the other
four exist for cases that would otherwise need several near-identical layers.

There is no way to express alternation between whole matchers. A layer that
should apply to web hosts or to cache hosts uses `oneOf` with both values
instead of two matchers combined with an `or`.

That omission is deliberate. Matchers that can be nested arbitrarily become
expressions to debug, and the cases that appear to need them are usually better
served by giving the hosts a label describing what they have in common.

## The empty matcher

A layer with no `match` block matches every host in the fleet.

```yaml
datum: v1alpha1
type: Layer

name: base

precedence: 0
```

Matching everything has to be written out, instead of happening by omission
somewhere else, which is why resource documents are required to sit under a
layer. The one place an empty matcher is normal is a fleet-wide base layer, and
the base layer is exactly the case where applying to everything is the intent.

## What matchers do not do

Matchers choose layers. They do not choose resources within a layer, filter
fields, or apply conditionally at reconciliation time.

A resource that should only exist on some of the hosts a layer matches belongs in a
different layer with a narrower matcher. Pushing conditions down into resources
would put the decision about whether a resource applies into two places at once,
and answering why a resource reached a host would then mean reading both.

## Matching is evaluated once per pass

The set of layers applying to a host is determined during resolution, before the
host is read and before any plan exists. Nothing later in the pipeline can change
it.

That ordering is what makes the question of which layers applied answerable from
the repository alone, and it means a change in a host's observed state never
changes which configuration applies to it.
