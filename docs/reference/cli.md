# Command line interface

!!! warning "None of these commands exist"

    There is no `datum` binary. This page describes intended behaviour so that the
    command surface can be argued about before it is built, and every command, flag and
    exit code on it is proposed.

The commands map onto the pipeline, so each one stops at a different point and prints
what it produced.

```text
datum render      resolve desired state            no host access
datum explain     why a resource applies           no host access
datum observe     read the host                    read only
datum diff        desired against observed         read only
datum plan        ordered actions                  read only
datum reconcile   apply and verify                 changes the host
datum status      the last pass result             no host access
```

Only `reconcile` changes anything. Everything above it in that list is safe to run on a production
machine in the middle of an incident.

A second group operates on the repository and not on a host, and none of the commands in it read a
managed machine at any point.

```text
datum init        create a new repository          writes local files
datum validate    parse and resolve every host     no host access
datum affected    which hosts a change reaches     no host access
datum migrate     rewrite documents to a schema    writes local files
```

## Global flags

| Flag | Meaning |
| ---- | ------- |
| `--host NAME` | Resolve for a named host instead of the local one. |
| `--revision REV` | Use a specific repository revision instead of the current one. |
| `--repo PATH` | Use a local checkout instead of the configured remote. |
| `--json` | Emit machine-readable output. |

`--host` is what makes the read-only commands useful from a laptop. Resolution reads only
repository content, so rendering another host's desired state needs no access to that
machine.

## datum render

Resolves the effective manifest and prints it.

```text
$ datum render --host web-001

host       web-001
revision   8b91f20
manifest   sha256:3f2a9c4e
resources  14

Package[curl]                base
Package[ca-certificates]     base
Package[nginx]               roles/web
File[nginx-config]           roles/web, hosts/web-001
Service[nginx]               roles/web
Sysctl[net.ipv4.ip_forward]  base, environments/production
User[www-data]               roles/web
```

Runs entirely against the repository. Two renders of the same revision for the same host
produce the same digest, which is what makes the command useful for confirming that a
refactor changed nothing.

## datum explain

Answers why a resource applies to a host, and where each of its field values came from.

```text
$ datum explain File[nginx-config] --host web-001

File[nginx-config]   /etc/nginx/nginx.conf

contributed by
  roles/web        precedence  30   matched role=web
  hosts/web-001    precedence 100   matched datum/host=web-001

fields
  path     /etc/nginx/nginx.conf    roles/web
  owner    root                     roles/web
  group    root                     roles/web
  mode     0600                     hosts/web-001   overrides 0640 from roles/web
  source   files/nginx.conf         roles/web

requires
  Package[nginx]                    roles/web
```

This is the command the fleet model exists to make possible, and the requirement
that provenance survives composition comes from it and not the other way round.

## datum observe

Reads and prints the current state of every resource in the manifest, without comparing
it to anything.

```text
$ datum observe

File[nginx-config]
  exists   true
  owner    root
  group    root
  mode     0644
  content  sha256:91c4de2a

Service[nginx]
  exists   true
  state    running
  enabled  true
```

Useful mainly for working out why a diff says what it says, and for confirming that a
provider reads a field the way it was expected to.

## datum diff

Compares desired against observed and prints the differences, without ordering them or
deciding what to do.

```text
$ datum diff

File[nginx-config]
  mode     0644 -> 0600
  content  differs

1 resource differs, 13 match
```

## datum plan

Builds the plan and prints it. This is `diff` plus ordering, dependency handling and
action selection.

```text
$ datum plan

host       web-001
revision   8b91f20
manifest   sha256:3f2a9c4e

update   File[nginx-config]
         path      /etc/nginx/nginx.conf
         provider  file
         mode      0644 -> 0600
         content   differs
         from      roles/web, hosts/web-001

update   Service[nginx]
         reason    File[nginx-config] changed, restartOn matched

none     Package[nginx]      present, 1.24.0-2

0 to create, 2 to update, 0 to remove, 0 to skip, 12 unchanged
```

## datum reconcile

Builds a plan and applies it, then verifies the affected resources.

```text
$ datum reconcile

update   File[nginx-config]     ok
update   Service[nginx]         ok
verify   File[nginx-config]     ok
verify   Service[nginx]         ok

changed: 2 updated, 12 unchanged
```

A plan is always built fresh from a current observation. There is no flag to apply a plan
saved earlier, because a stored plan encodes an observation that has since gone out of
date.

## datum status

Reports the result of the last pass on this host.

```text
$ datum status

host       web-001
revision   8b91f20
manifest   sha256:3f2a9c4e
outcome    changed
finished   2 minutes ago

2 updated, 12 unchanged
```

Reads a local report, not the repository or the host, so it is cheap and says
nothing about whether the host has drifted since.

## datum init

Creates the two documents a repository needs in order to be discovered, and stops there.

```text
$ datum init

created fleet/datum.yaml
created fleet/base/layer.yaml

next steps
  add a Host document under fleet/hosts/
  add resources under fleet/base/ or a new layer
```

`--with-examples` adds a commented example host and resource. The reasoning behind the
generated content being this small, and behind the command leaving Git alone, is set out
under [creating a repository](../repository/index.md#datum-init).

## datum validate

Parses every document, checks each against its declared schema version, then resolves and
builds a graph for every host in the fleet.

```text
$ datum validate

fleet      example
revision   9c02ab
hosts      500
layers     14

resolved 500 hosts, 0 errors
```

Errors report how many hosts they affect instead of repeating once per host, and
`--strict` promotes warnings to errors. There is no separate lint command, for
[the reasons given alongside
it](../repository/validating-changes.md#there-is-no-datum-lint).

## datum affected

Resolves every host at two revisions and reports which of them end up with a different
[manifest digest](../fleet/effective-manifests.md#content-addressing).

```text
$ datum affected --from origin/main --to HEAD

42 of 500 hosts affected

web-001    sha256:3f2a9c4e -> sha256:8d10b7f2
web-002    sha256:3f2a9c4e -> sha256:8d10b7f2
...
db-001     unchanged
```

`--show-resources --host NAME` expands one host into a diff between its two
manifests, which reports what that host's desired state becomes, not what
changed in the repository.

## datum migrate

Rewrites documents from one schema version to another, in place, and leaves the result for
a human to review and commit.

```text
$ datum migrate --to v1beta1

rewrote 41 documents in 18 files
  fleet/base/packages.yaml
  fleet/roles/web/nginx.yaml
  ...

review the diff before committing
```

A migration that cannot be performed mechanically stops and names the documents needing a
human, and no agent ever performs one. Both points are covered under
[schema versions](../repository/schema-versions.md#migration-is-a-repository-operation).

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0` | Success, and no differences were found by a read-only command. |
| `1` | Error. Resolution failed, validation failed, or an action failed. |
| `2` | Differences found. Only from `diff` and `plan`. |

Separating code 2 from code 0 is what makes `datum diff` usable as a drift check in
scheduled jobs. Without it, a script cannot distinguish a converged host from a host with
pending changes, and every caller ends up parsing output.

!!! note "Open question"

    Whether `reconcile` should exit non-zero when resources were skipped is undecided.
    Skipping because no provider supports a resource is not a failure of the pass, and a
    host quietly sitting at "mostly converged" indefinitely is worse than a noisy exit
    code.

## Commands deliberately absent

There is no command to run something on a host, and no command that takes a shell
fragment. That follows from Datum's input being a description of state, and adding one
would make every guarantee about planning and verification conditional on whether a
repository used it.

There is no `datum apply`. The name suggests applying a thing that was prepared earlier,
which is exactly the model being avoided, and `reconcile` names what actually happens.

There is no import or adopt command that turns an existing machine into
configuration. Nothing writes to the repository, and generating resources from a
host would produce a description of what is there rather than a decision about
what should be.
