# Resource identity

Every resource has two identities, and confusing them causes problems that only
appear once a repository has grown.

Resource reference
:   How the resource is named inside Datum. It is the kind and `name`
    together, written `File[nginx-config]`, and it is unique within an effective
    manifest.

Target identity
:   What the resource manages on the host. For a `File` it is the absolute path, for
    a `Package` it is the package name, and it is what determines whether two
    resources are fighting over the same thing.

## Why two are needed

A single identity cannot do both jobs.

If the reference were the target, `File` resources would have to be named by their
path, which makes every `requires` entry an absolute path and makes a layer unable
to override a file's mode without repeating the path as a name. If the target were
the reference, two differently named `File` resources could write the same path and
Datum would have no way to notice.

```yaml
datum: v1alpha1
type: File

name: nginx-config

desired:
  path: /etc/nginx/nginx.conf
```

```yaml
datum: v1alpha1
type: File

name: nginx-tls-settings

desired:
  path: /etc/nginx/nginx.conf
```

Those two resources have different references and the same target. Both would
apply, the second would overwrite the first, and which one won would depend on plan
order. The graph builder rejects the manifest instead.

## Target identity by type

| Type | Target identity | Source |
| ---- | --------------- | ------ |
| `Package` | Package name | `name` |
| `File` | Absolute path | `desired.path` |
| `Directory` | Absolute path | `desired.path` |
| `Symlink` | Absolute path | `desired.path` |
| `Repository` | Source identifier | `desired.id` |
| `Service` | Unit name | `name` |
| `User` | User name | `name` |
| `Group` | Group name | `name` |
| `Sysctl` | Parameter key | `name` |

Where a type's natural key is a single string that reads well as a name,
`name` supplies it, which keeps the common case short. `File`, `Directory` and
`Symlink` are exceptions because paths make poor names and because the same
logical file often lives at different paths on different distributions.
`Repository` is an exception for a different reason, which is that the identifier a
package manager knows a source by is not something a reader would recognise as the
source's name.

!!! note "Open question"

    Whether `Package` should gain a `desired.package` field so that
    `name` can be a logical name is unresolved. It would allow one
    reference to mean `apache2` on Debian and `httpd` on Fedora, which is exactly
    the kind of difference the provider boundary is supposed to absorb, and it is
    not yet clear whether the provider or the resource should carry it.

## References are scoped to the manifest

A resource reference is unique within one host's effective manifest and means
nothing outside it. Two hosts can both have `File[nginx-config]` pointing at
different content, and neither has any relationship to the other.

That scope is what makes `requires` simple. A dependency is a reference to another
resource in the same manifest, with no need to qualify it by layer, by host, or by
path, and a reference that does not resolve within the manifest is an error the
graph builder raises before the host is read.

The same scope is what allows layers to merge. A role layer and a host layer
both contributing `File[nginx-config]` are contributing to one resource, and
that is only coherent because the reference identifies the resource, not the
file.

## Renaming

Changing `name` changes the reference and therefore creates a different
resource, so every `requires` pointing at the old name stops resolving and the
manifest is rejected until they are updated.

Changing `desired.path` changes the target. The old path is no longer managed and is
left exactly as it is, because nothing in the manifest describes it any more and
undeclared paths are not touched. Moving a managed file therefore needs two
resources for one pass, one declaring the new path and one declaring the old path
absent, after which the second can be deleted.

!!! note "Important limitation"

    There is no rename operation and no way to express that one target replaces
    another. This follows from desired state being a description instead of a
    sequence of changes, and it means migrations that move files are a two-step
    change in the repository, not something Datum works out.
