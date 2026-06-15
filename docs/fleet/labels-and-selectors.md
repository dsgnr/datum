# Labels and selectors

Labels classify a host. Selectors decide which layers apply to it. Between them
they are the only mechanism by which configuration reaches a machine, so there is
no inheritance hierarchy, no include directive, and no host list to maintain
alongside them.

## Labels are declared in the repository

A host's labels come from its `Host` document and nowhere else.

```yaml
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
| `datum.dev/host` | The host's `metadata.name` |

This exists so that host-specific configuration needs no special mechanism. A
layer targeting one machine is an ordinary layer with an ordinary selector.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Layer

metadata:
  name: host-web-001

spec:
  precedence: 100
  selector:
    matchLabels:
      datum.dev/host: web-001
```

The `datum.dev/` prefix is reserved. A `Host` document setting a label under it is
an error, so that injected values cannot be shadowed.

!!! note "Open question"

    Facts observed on the host, such as the distribution and version reported by
    `/etc/os-release`, are not available as labels. Making them available would
    let a layer target Debian hosts without anybody declaring which hosts run
    Debian, and it would also mean desired state could no longer be resolved
    without reaching the machine. The current position is that a host needing
    distribution-specific treatment carries a declared label saying so, and that
    provider selection handles the rest.

## Selector semantics

A selector has two optional parts.

```yaml
spec:
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

`matchLabels` requires each named label to be present with exactly that value.
`matchExpressions` supports four operators.

| Operator | Matches when |
| -------- | ------------ |
| `In` | The label exists and its value is one of `values`. |
| `NotIn` | The label does not exist, or exists with a value not in `values`. |
| `Exists` | The label is present, whatever its value. |
| `DoesNotExist` | The label is absent. |

Every term in both parts has to match. A selector is a conjunction with no way to
express alternation between whole selectors, so a layer that should apply to web
hosts or to cache hosts uses `matchExpressions` with `In` rather than two
selectors.

The absence of a top-level `or` is deliberate. Selectors that can be arbitrarily
nested become expressions to debug, and the cases needing them are usually better
served by giving the hosts a label that says what they have in common.

## The empty selector

A layer with no `selector` matches every host in the fleet.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Layer

metadata:
  name: base

spec:
  precedence: 0
```

Matching everything has to be written deliberately rather than happening by
omission somewhere else, which is why resource documents are required to sit under
a layer. The one place an empty selector is normal is a fleet-wide base layer, and
the base layer is exactly the case where applying to everything is the intent.

## What selectors do not do

Selectors choose layers. They do not choose resources within a layer, filter
fields, or apply conditionally at reconciliation time.

A resource that should only exist on some of the hosts a layer matches belongs in a
different layer with a narrower selector. Pushing conditions down into resources
would put the decision about whether a resource applies into two places at once,
and answering why a resource reached a host would then mean reading both.

## Matching is evaluated once per pass

The set of layers applying to a host is determined during resolution, before the
host is read and before any plan exists. Nothing later in the pipeline can change
it.

That ordering is what makes the question of which layers applied answerable from
the repository alone, and it means a change in a host's observed state never
changes which configuration applies to it.
