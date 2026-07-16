# Directory

`Directory` describes the existence and metadata of one directory.

```yaml
datum: v1alpha1
type: Directory

name: nginx-conf-d

desired:
  path: /etc/nginx/conf.d
  owner: root
  group: root
  mode: "0755"
```

Target identity is `desired.path`. The type says nothing at all about the
directory's contents.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `path` | absolute path | Yes | The directory being managed. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether the directory should exist. |
| `owner` | string | No | Owning user name. |
| `group` | string | No | Owning group name. |
| `mode` | string | No | Permission bits, quoted. |

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the path exists and is a directory. |
| `owner` | Owning user name. |
| `group` | Owning group name. |
| `mode` | Permission bits. |

## Contents are not managed

A `Directory` resource has an opinion about the directory and none about what is
inside it. Files in a managed directory are managed only if they have their own
`File` resources, and anything else is left alone.

Parent directories are also not created implicitly. A `Directory` at
`/opt/app/etc/conf.d` whose parents do not exist fails rather than creating four
levels, because creating a directory nobody declared means choosing ownership
and permissions nobody specified.

Declaring the parents is the alternative, with `requires` giving the order.

```yaml
datum: v1alpha1
type: Directory

name: app-root

desired:
  path: /opt/app
  mode: "0755"
---
datum: v1alpha1
type: Directory

name: app-etc

requires:
  - Directory[app-root]

desired:
  path: /opt/app/etc
  mode: "0755"
```

This is verbose, and the verbosity is the point of disagreement, not something
the design is confident about.

!!! note "Open question"

    Whether `Directory` should create missing parents, and with what ownership and
    mode, is undecided. Creating them with the same metadata as the declared
    directory is the obvious guess and would silently produce directories nobody
    described, which conflicts with the rule that Datum manages what is declared.
    Requiring every level to be declared is consistent and tedious.

## Removal

`state: absent` removes the directory only if it is empty. A non-empty directory
fails instead of being removed recursively.

Recursive deletion as root is the single most destructive operation Datum could
perform, and a typo in a path that produced it would be unrecoverable. Requiring the
directory to be empty means removal is possible while the blast radius of a mistake
stays bounded.

!!! note "Security consideration"

    There is no field that enables recursive removal, and adding one would need a
    stronger argument than convenience. A directory whose contents should also be
    gone can have its contents declared absent as `File` resources, which makes each
    deletion explicit and visible in the plan.

## Purging undeclared contents

The case desired state cannot currently express is a directory that should contain
exactly the declared files and nothing else.

```text
/etc/nginx/conf.d/
  tls.conf        declared
  upstream.conf   declared
  old-site.conf   not declared, left alone
```

Drop-in directories like `conf.d` are where this matters, because a file left behind
after being removed from the repository keeps being loaded by the service. The
general rule that undeclared things are unmanaged produces exactly the wrong outcome
here.

!!! note "Open question"

    This is the one place where the declared-only ownership model is known to be
    insufficient, and it is the strongest argument for some form of reclaim
    behaviour. A `desired.purge` field limited to a single directory level, not
    recursive, and listing what it removed in the plan, is the shape a solution
    would probably take. It has not been designed, and it reintroduces the risk that
    the empty-directory restriction above exists to avoid.
