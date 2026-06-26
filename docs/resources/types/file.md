# File

`File` describes the content and metadata of one file.

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

Target identity is `desired.path` and not `name`, so two `File` resources with
different names and the same path are a conflict.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `path` | absolute path | Yes | The file being managed. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether the file should exist. |
| `content` | string | No | Literal content, held in the resource. |
| `source` | path | No | Content taken from a file in the repository, verbatim. |
| `template` | path | No | Content taken from a file in the repository and [rendered](../../fleet/substitution.md#substituting-into-file-content). |
| `owner` | string | No | Owning user name. |
| `group` | string | No | Owning group name. |
| `mode` | string | No | Permission bits, quoted. |
| `sensitive` | boolean | No, defaults to `false` | Suppresses rendering of the content anywhere. |
| `validate` | string | No | Names a [validator](../validation.md) run against the staged content before it goes live. |

`content`, `source` and `template` are mutually exclusive, and a resource setting more
than one is rejected. Setting none of them manages metadata only, which is how ownership
or permissions on a file created by a package are corrected without taking over what is
in it.

`template` differs from `source` only in that [label values are
substituted](../../fleet/substitution.md) as the content is read. Keeping them as separate fields
rather than one field with a flag means whether a file is rendered is visible on the line that names
it.

`source` is resolved relative to the layer directory, so a resource in `fleet/roles/web/` referring
to `files/nginx.conf` means `fleet/roles/web/files/nginx.conf`. It must be a relative path and must
resolve to somewhere inside the fleet root, which is
[confinement](../../security/provider-safety.md#confining-content-sources), not a convention.

`sensitive: true` stops the content appearing in a diff, a plan or a report, while ownership and
mode are still reported normally. It is not secret management, because the content still sits in the
repository in plain text, and the
[limitation](../../security/provider-safety.md#marking-content-as-sensitive) needs reading before
anything relies on it.

## Quoting mode

`mode` is a string because YAML would otherwise interpret the value.

```yaml
mode: "0640"    # correct
mode: 0640      # wrong
```

Unquoted, `0640` is a number, and depending on the YAML version parsing it is
either octal 416 or decimal 640. Requiring a string removes the ambiguity, and a
`mode` given as a number is rejected instead of guessed at.

## Fields not set are not managed

Only the fields the resource declares are compared. A resource setting `mode` and
not `owner` corrects permissions and leaves ownership alone, whatever it is.

This follows from Datum managing what is declared, and it is what allows a file to be
partially owned. It also means an omission is indistinguishable from an intention, so
a file whose ownership matters needs ownership declared even when it currently looks
right.

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the path exists and is a regular file. |
| `owner` | Owning user name. |
| `group` | Owning group name. |
| `mode` | Permission bits. |
| `content` | A digest of the content, not the content itself. |

Reporting a digest keeps observation cheap on large files and makes the comparison
exact. Rendering a readable difference happens when a plan is displayed, which is a
separate concern from measuring state.

A path that exists but is a directory or a symlink reports `exists: false` for
the purposes of this resource, along with what was actually found, so the plan
can say that the target is occupied by something else rather than reporting a
content mismatch.

## Applying

Content is written to a temporary file in the same directory and renamed into place,
so a process reading the file sees either the previous content or the new content.
Ownership and mode are set on the temporary file before the rename, which avoids a
window where the file exists with the wrong permissions.

Writing to the same directory matters because a rename across filesystems is not
atomic, and `/tmp` is frequently a different filesystem from `/etc`.

!!! note "Security consideration"

    A provider writing as root has to assume the path is hostile. An unprivileged user who can
    create entries in the target directory, which is common under `/var/lib` and `/srv`, can replace
    the target with a symbolic link and have root write somewhere else. The handling that follows
    from this, covering `O_NOFOLLOW`, exclusive temporary file creation, setting metadata on the
    file descriptor and not the path, and hard link counts, is specified in the [threat
    model](../../security/threat-model.md#an-attacker-with-an-unprivileged-account-on-a-managed-host).

## Verification

The file is re-read and every declared field compared. The most common verification
failure is content that does not match because the provider normalised it on write,
usually by adding or removing a trailing newline, which produces a resource that
appears to change on every pass.

## Open questions

Whether `owner` and `group` should accept numeric ids alongside names is undecided.
Names are clearer and depend on the user existing, which is a dependency the manifest
can express, and ids avoid that ordering problem while being harder to read.
