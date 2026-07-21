# Symlink

`Symlink` describes a symbolic link and where it points.

```yaml
datum: v1alpha1
type: Symlink

name: nginx-site-enabled

requires:
  - File[nginx-site]

desired:
  state: present
  path: /etc/nginx/sites-enabled/app.conf
  target: /etc/nginx/sites-available/app.conf
```

Target identity is `desired.path`, the link itself, in the same way it is for
[`File`](file.md) and for the same reason.

## Why not a field on File

A `File` resource's field set assumes content, ownership and permission bits, and a symbolic link has
none of those in any meaningful sense. Its mode is ignored by Linux, it has no content beyond the path it
holds, and the operations that create it are different system calls.

More decisively, `File` and `Symlink` need opposite handling of the same situation. `File` reports a path
occupied by a symlink as [`exists: false`](file.md#observation) and refuses to write through it, because
following a link at a managed path is
[how an unprivileged user redirects a root write](../../security/provider-safety.md#resolving-a-managed-path).
A type that sometimes managed the link and sometimes the file it points at would make that refusal
conditional on a field, which is the wrong shape for a safety control.

Keeping them separate means a path is claimed by exactly one of the two, and a repository declaring both a
`File` and a `Symlink` at one path is a
[duplicate target identity](../identity.md) error raised before the host is read.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `path` | absolute path | Yes | The link being managed. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether the link should exist. |
| `target` | string | Yes when present | What the link points at. |
| `owner` | string | No | Owning user name. |
| `group` | string | No | Owning group name. |

There is no `mode`. Symbolic link permission bits are ignored by the kernel on Linux, so offering the
field would invite a repository to set something with no effect, and a field that does nothing is worse
than an absent one.

`target` may be relative, which is frequently what is wanted, and it is stored exactly as written. A
relative target is interpreted by the kernel against the directory holding the link, so rewriting it as an
absolute path would change what the link means.

## A dangling target is allowed

`target` is not checked for existence. A link pointing at a path that does not exist is created, and the
resource converges.

Dangling links are legitimate and sometimes deliberate. A link into a filesystem mounted later, or into a
directory a service creates at first start, is correct at the moment Datum writes it and broken only if
read too early. Refusing to create one would mean Datum deciding the order the rest of the system comes
up in, which it has no way to know.

Where the target is something Datum also manages, an ordering edge expresses the relationship.

```yaml
requires:
  - File[nginx-site]
```

That is a declared dependency like any other, and Datum
[infers nothing](../dependencies.md) from the fact that a link's target happens to match another
resource's path.

## Observation

| Field | Observed |
| ----- | -------- |
| `exists` | Whether a symbolic link exists at the path. |
| `target` | The path the link holds. |
| `owner` | Owning user name. |
| `group` | Owning group name. |

The link is read with `readlinkat` and stat'd without following it, so everything reported describes
the link, not whatever it points at. A path holding a regular file or a directory reports `exists:
false` along with what was found, mirroring how `File` reports a path occupied by a link.

That symmetry is what makes the two types safe to have side by side. Both operate on the final path
component without following it, so neither can be induced into acting on a different file by something
planted at the path.

## Applying

A link is created at a temporary name in the target directory and renamed into place.

```text
symlinkat(target, parent, ".datum.tmp.XXXX")
renameat(parent, ".datum.tmp.XXXX", parent, "app.conf")
```

The rename is what makes replacing an existing link atomic, since `symlinkat` fails when the name already
exists and removing the old link first would leave a window with nothing there. A reader sees either the
old target or the new one.

Everything in [resolving a managed path](../../security/provider-safety.md#resolving-a-managed-path)
applies unchanged. The parent directory is held as a descriptor, its device and inode are rechecked
before the rename, and the operations are the `at` forms in place of their path-based equivalents.

## Removal

Declaring a link `absent` removes the link and never touches its target.

That follows from target identity being the link's own path. Removing what a link points at would mean one
resource deleting something it does not claim, which
[declared-only ownership](../../adr/0009-declared-only-ownership.md) rules out, and a link is removed with
`unlinkat` which cannot follow it in any case.

A path that exists and is not a symbolic link is left alone and reported, rather than being removed
as though it were the link that was declared.

!!! note "Proposed behaviour"

    This type does not exist. It is specified because symbolic links currently have no
    representation at all while `File` actively refuses them, which leaves a common requirement with
    no expression at all, and not merely an undesigned one.
