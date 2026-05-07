# Composition

Composition is the step that turns a set of matching layers into one set of
resources. It is the mechanism that lets a repository describe similarity once
rather than copying it between hosts.

```text
base
 +
matching selectors
 +
host-specific configuration
 =
effective desired state
```

## Resources are matched by reference

Two layers contributing a resource with the same kind and the same
`metadata.name` are contributing to the same resource. That pair is the resource
reference, written `File[nginx-config]`, and it is unique within an effective
manifest.

```yaml title="fleet/roles/web/nginx.yaml"
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

```yaml title="fleet/hosts/web-001/nginx-tuning.yaml"
apiVersion: datum.dev/v1alpha1
kind: File

metadata:
  name: nginx-config

spec:
  mode: "0600"
```

The host layer does not repeat the path, the owner or the source. It contributes
one field, and the merged result carries everything from the role layer with the
mode replaced.

```text
File[nginx-config]
  path     /etc/nginx/nginx.conf     roles/web
  owner    root                      roles/web
  group    root                      roles/web
  mode     0600                      hosts/web-001
  source   files/nginx.conf          roles/web
```

A resource reference appearing in only one layer is not merged with anything and
passes through unchanged.

## Merge rules

Layers are folded together in precedence order, lowest first, so a later layer
sees the result of every earlier one.

| Field shape | Rule |
| ----------- | ---- |
| Scalar | Replaced by the higher-precedence value. |
| Map | Merged key by key, with the higher-precedence value winning per key. |
| List | Replaced entirely by the higher-precedence list. |
| `dependsOn` | Combined as a set. |

Lists replace instead of merging because merging them requires a rule for
identifying corresponding entries, and every such rule needs the reader to know
which key the implementation chose. Replacement is blunt and predictable, and a
list that genuinely needs to be assembled from several layers is a sign that its
entries should be separate resources.

## The dependsOn exception

`dependsOn` is a list that does not follow the list rule, and the inconsistency is
deliberate.

If a role layer declares that `Service[nginx]` depends on `Package[nginx]`, and a
host layer adds a dependency on a tuning file, replacement would silently discard
the package dependency. The result would still apply, but in the wrong order, and
nothing in the plan would indicate that an ordering constraint had been dropped.
Combining the references instead makes the failure impossible.

Dependencies are also genuinely set-like, having no order of their own and no
meaning to a duplicate entry, so treating them as a set costs nothing in
expressiveness.

!!! note "Open question"

    There is no way for a higher-precedence layer to remove a dependency declared
    lower down. Whether that is needed is unclear, and adding a removal syntax
    would reintroduce the ability to drop an ordering constraint, which is what
    the exception exists to prevent.

## A worked conflict

Two layers set the same field to different values, and precedence decides.

```yaml title="fleet/base/sysctl.yaml"
apiVersion: datum.dev/v1alpha1
kind: Sysctl

metadata:
  name: net.ipv4.ip_forward

spec:
  value: "0"
```

```yaml title="fleet/environments/production/sysctl.yaml"
apiVersion: datum.dev/v1alpha1
kind: Sysctl

metadata:
  name: net.ipv4.ip_forward

spec:
  value: "1"
```

For a host labelled `environment: production`, the base layer at precedence 0 is
folded first and the environment layer at precedence 10 replaces the value. The
result is `1`, and both contributions are recorded so that the override is
reportable and not merely effective.

For a host labelled `environment: staging`, the production layer does not match at
all, and the value stays `0`.

## Composition does not look at the host

Everything on this page happens against repository content. No part of the merge
depends on what is installed on the machine, which is what allows a composed
result to be produced for any host from a checkout.

The consequence is that composition cannot express "set this value unless
something else already set it on the host". Desired state is a statement about
what should be true, and making it conditional on what is currently true would
turn every field into a question the repository could not answer on its own.
