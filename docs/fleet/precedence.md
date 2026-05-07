# Precedence and conflicts

Precedence decides which layer wins when two of them set the same field. It is an
integer on each `Layer`, compared numerically, with higher values applied later and
therefore winning.

```yaml
spec:
  precedence: 30
```

An integer was chosen over the alternatives because a reader can compare two
numbers without knowing anything about Datum. Ranking by selector specificity, by
directory depth, or by the order layers happen to be discovered would all produce
a total order that nobody could predict from looking at two layers side by side.

## Ordering is total and deterministic

Matching layers are sorted by precedence, and layers with equal precedence are
sorted by the path of their `Layer` document relative to the fleet root.

```text
base/layer.yaml                        0
environments/production/layer.yaml    10
sites/london/layer.yaml               20
roles/web/layer.yaml                  30
hosts/web-001/layer.yaml             100
```

The path tie-break gives the fold a defined order in every case. It does not
resolve disagreements, and two layers of equal precedence setting the same field
to different values are a conflict.

Resolution never depends on filesystem enumeration order, on the machine running
the resolver, or on which host is being resolved beyond the selectors that matched
it. The same repository at the same revision produces the same manifest for the
same host every time.

## Conflicts are errors

When two layers of equal precedence set the same field of the same resource to
different values, resolution fails and no manifest is produced.

```text
error: conflicting values for File[nginx-config].mode

  0640  roles/web            precedence 30
  0600  roles/web-tls        precedence 30

both layers match host web-001 at equal precedence
```

Selecting a winner would make the result depend on something not visible in the
repository, applied as root on every host both layers match. Refusing to resolve
reports the ambiguity when it is introduced.

Two layers setting the same field to the same value are not in conflict. The
requirement is an unambiguous outcome, and several layers may mention a field.

Resolving a conflict means changing precedence so one layer is clearly the
authority, or narrowing a selector so the layers no longer overlap.

## Host overrides

A single host is overridden by a layer with a high precedence and a selector
matching that host.

```yaml title="fleet/hosts/web-001/layer.yaml"
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

A host override is an ordinary layer, with no separate mechanism, per-host
section or special syntax. Its contributions are recorded with provenance and it
can conflict with another layer at the same precedence.

The conventional precedence of 100 sits well above the others, so a host
override normally wins. A fleet-wide setting that has to take priority is given
a higher number.

## Traceability

Provenance is recorded per field as well as per resource, since a question is
usually about one value.

!!! note "Proposed command"

    `datum explain` is a proposed command and does not exist. The output below
    shows what the design has to be able to produce, which is the part being
    committed to.

```text
$ datum explain File[nginx-config] --host web-001

File[nginx-config]   /etc/nginx/nginx.conf

contributed by
  roles/web        precedence  30   selector role=web
  hosts/web-001    precedence 100   selector datum.dev/host=web-001

fields
  path     /etc/nginx/nginx.conf    roles/web
  owner    root                     roles/web
  group    root                     roles/web
  mode     0600                     hosts/web-001   overrides 0640 from roles/web
  source   files/nginx.conf         roles/web

dependsOn
  Package[nginx]                    roles/web
```

Two things have to survive resolution for this to be answerable. Every field needs
the layer that set its final value and the values it displaced, and every layer
needs the selector terms that caused it to match. Discarding either after the merge
would make the question unanswerable, so the resolver is not permitted to discard
them as an optimisation.

## Why not order by specificity

Ranking layers by how specific their selectors are is the obvious alternative and
was rejected.

A selector matching three labels is not reliably more authoritative than one
matching two, since `role: web` plus `site: london` plus `architecture: amd64` is
more specific than `datum.dev/host: web-001` by any counting rule while clearly
being less targeted. Making specificity work would require weighting labels against
each other, which means the repository ends up encoding a ranking of label keys
somewhere, and at that point an integer on each layer is the same mechanism with
less machinery.
