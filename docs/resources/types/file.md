# File

`File` describes the content and metadata of one file.

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

Target identity is `spec.path` rather than `metadata.name`, so two `File` resources
with different names and the same path are a conflict.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `path` | absolute path | Yes | The file being managed. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether the file should exist. |
| `content` | string | No | Literal content, held in the resource. |
| `source` | path | No | Content taken from a file in the repository. |
| `owner` | string | No | Owning user name. |
| `group` | string | No | Owning group name. |
| `mode` | string | No | Permission bits, quoted. |

`content` and `source` are mutually exclusive, and a resource setting both is
rejected. Setting neither manages metadata only, which is how ownership or
permissions on a file created by a package are corrected without taking over what is
in it.

`source` is resolved relative to the layer directory, so a resource in
`fleet/roles/web/` referring to `files/nginx.conf` means
`fleet/roles/web/files/nginx.conf`.

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

## Verification

The file is re-read and every declared field compared. The most common verification
failure is content that does not match because the provider normalised it on write,
usually by adding or removing a trailing newline, which produces a resource that
appears to change on every pass.

## Open questions

Templating is not part of the design. Rendering content from host facts would make
`File` the most-used part of Datum and would also reintroduce a dependency on
observed state during resolution, which the fleet model currently rules out. The
alternative of one file per variation is verbose and reviewable, and it is not clear
that verbosity is the worse problem.

Symbolic links have no representation. Whether they belong in `File`, in a separate
type, or nowhere is undecided.

Whether `owner` and `group` should accept numeric ids alongside names is undecided.
Names are clearer and depend on the user existing, which is a dependency the manifest
can express, and ids avoid that ordering problem while being harder to read.
