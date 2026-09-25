# Command line interface

!!! warning "Some of these commands do not exist yet"

    `render`, `explain`, `observe`, `diff`, `plan`, `reconcile`, `status`, `validate`,
    `affected`, `init`, `agent`, `config check`, `revision` and `version` are implemented.

    Every resource type has a provider. Which one serves a host depends on what it runs, so a type
    with none here is reported as skipped, which makes a host
    [degraded](../concepts/state.md#host-state-across-passes) rather than converged.

    Applying is Linux-only. Reading a host works anywhere, since the safety rules the writing path
    depends on have no portable equivalent.

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
datum agent       reconcile on the interval        changes the host
```

Only `reconcile` changes anything. Everything above it in that list is safe to run on a production
machine in the middle of an incident.

A second group operates on the repository and not on a host, and none of the commands in it read a
managed machine at any point.

```text
datum init        scaffold Datum documents          writes local files
datum validate    parse and resolve every host     no host access
datum affected    which hosts a change reaches     no host access
```

Two more read and write the agent's own state, not a host or a repository.

```text
datum config check   the resolved configuration    reads local files
datum revision       the accepted revision         reads and writes local state
```

## Common flags

These are spelled the same way wherever they appear. Each belongs to the commands that take
it rather than to `datum` itself, so it goes after the command name and not before it.

| Flag | Meaning |
| ---- | ------- |
| `--host NAME` | Resolve for a named host instead of the local one. |
| `--revision REV` | Use a specific repository revision instead of the current one. |
| `--repo PATH` | Use a local checkout instead of the configured remote. |
| `--json` | Emit machine-readable output. |
| `--version` | Report the version, the revision it was built from and the platform. |

`datum --host web-001 render` is not accepted, and `--version` is the one exception, since it
is answered before any command is chosen. On `datum affected`, `--host` narrows the report to
one host rather than selecting which host to resolve for.

`--host` is what makes the read-only commands useful from a laptop. Resolution reads only
repository content, so rendering another host's desired state needs no access to that
machine.

!!! note "Implementation status"

    Of the five, `--host`, `--repo` and `--version` are the ones that work. `--host` is required on
    the commands that take it, because nothing works out which host the local machine is, and that is
    [enrolment](../lifecycle/enrolment.md), not a missing default. `datum agent` is the exception,
    since it reads `host` from [its configuration](agent-config.md#identity) and fetches from
    `source.url` instead of needing `--repo`. `--revision` and `--json` are not implemented, so
    every command other than the agent reads a checkout named with `--repo`. `datum status` has its
    own `--output json` because it is the one command whose structured form is already
    [designed](status.md#machine-readable-output).

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

host       web-002
revision   d47726f
manifest   sha256:8448ab72
mode       enforce

converged  File[motd]           update content
converged  File[nginx-config]   create /etc/nginx/nginx.conf
skipped    Package[nginx]       no provider for Package on this host
skipped    Service[nginx]       no provider for Service on this host

outcome    changed
host state degraded
resources  14 total, 5 converged, 0 drifted, 0 failed, 0 blocked, 8 skipped
duration   21ms
```

Resources with nothing to do are left out, since they are the majority on a healthy host and listing
them buries the ones that matter.

A plan is always built fresh from a current observation. There is no flag to apply a plan
saved earlier, because a stored plan encodes an observation that has since gone out of
date.

Each resource is verified by reading the target again, not by trusting what the provider returned.
An action that reported success and did not take is exactly the failure to catch, and a provider
cannot be the judge of its own work. A resource that fails blocks everything downstream of it,
transitively, so a dependent is never attempted against a prerequisite that is not in place.
Unrelated resources are still reconciled, because leaving the rest of a machine unmanaged turns one
fault into many.

The [pass lock](../reconciliation/locking.md) is taken before anything is read, so two passes cannot
observe one host while one of them is partway through changing it. A second invocation exits `3`
rather than waiting, because a command that blocks without saying so is indistinguishable from one
that has hung. `--wait` asks for the other behaviour, which is what a scheduled job wants and not an
operator at a terminal.

| Flag | Meaning |
| ---- | ------- |
| `--mode MODE` | `enforce` applies the plan, `observe` reports what differs and changes nothing. |
| `--state DIR` | Directory for the pass lock and reports. Defaults to `/var/lib/datum`. |
| `--wait` | Wait for another pass to finish instead of exiting `3`. |

`--mode observe` runs the same observation and the same diff as `enforce`, so the drift it
reports is exactly the drift `enforce` would act on. A pass that found work to do in that
mode ends with the outcome `drifted` and exits `2`.

## datum status

Reports the result of the last pass on this host.

```text
$ datum status

host  web-002

desired
  revisionAttempted  d47726f
  revisionApplied    d47726f
  manifest           sha256:8448ab72

state
  condition  degraded
  mode       enforce

last pass
  outcome   changed
  finished  2026-02-08T09:14:22Z (2m14s ago)
  duration  21ms

resources
  total      14
  converged  5
  drifted    0
  failed     0
  blocked    0
  skipped    8

skipped    Package[nginx]   no provider for Package on this host
skipped    Service[nginx]   no provider for Service on this host
```

Reads a local report, not the repository or the host, so it is cheap and says
nothing about whether the host has drifted since.

| Flag | Meaning |
| ---- | ------- |
| `--state DIR` | Directory the reports were written to. Defaults to `/var/lib/datum`. |
| `--output FORMAT` | `text` or `json`. |
| `--resources` | List every resource, including the converged ones. |

The resources printed under the counts are the ones that are not converged, which is the
part somebody reading status during an incident is looking for. `--resources` prints the whole list
for the cases where the absence of an entry is itself the question.

A host that has never run says so and exits `0`, because never having reported is not the
same as having reported a problem.

## datum agent

Runs as a service, reconciling on the interval in
[the configuration file](agent-config.md#reconciliation).

```console
# datum agent
2026-02-08T09:00:00Z serving metrics on 127.0.0.1:10056
2026-02-08T09:00:00Z host web-001, interval 30m, offset 6m51s, next pass 2026-02-08T09:06:51Z
2026-02-08T09:06:51Z pass converged, 14 of 14 resources converged, next pass 2026-02-08T09:36:51Z
```

| Flag | Meaning |
| ---- | ------- |
| `--config PATH` | Configuration file. Defaults to `/etc/datum/agent.yaml`. |
| `--repo PATH` | Reconcile a local checkout instead of fetching, which verifies nothing. |
| `--prefix PATH` | Fleet directory within the repository, for a monorepo. |

Each pass clones or fetches `source.url`, selects a revision according to
[`trust.require`](../security/repository-trust.md#verifying-that-a-revision-is-genuine), checks
it descends from the one this host accepted, and reconciles it. A revision that is refused
leaves the host on [the last one that worked](../reconciliation/last-known-good.md).

`--repo` skips all of that, so it is refused unless `trust.require` is `none`. That keeps a
host from applying an unverified tree while its own configuration says it should not.

There is no pass at startup. The agent waits for its own
[offset](../reconciliation/scheduling.md#passes-are-spread-deterministically) within the first
interval, so a fleet rebooting together does not reconcile at once.

It exits `0` when asked to stop and `1` when it could not start. `SIGTERM` and `SIGINT` both
stop scheduling and end a pass that is still running, which leaves the host partially applied
in the way any [interrupted pass](../reconciliation/failure-handling.md#a-pass-is-bounded) does.
[Running Datum on a host](../lifecycle/running.md#run-the-agent-as-a-service) has the unit file.

## datum config check

Reports the resolved configuration, including the defaults.

```console
# datum config check

/etc/datum/agent.yaml       ok
  host                     web-001
  source.url               https://git.example.com/fleet.git
  source.branch            main
  trust.require            none
  trust.requireDescendant  true
  reconciliation.mode      enforce
  reconciliation.interval  30m, offset 06:51
  reconciliation.timeout   15m
  metrics.listen           127.0.0.1:10056
  state                    /var/lib/datum   root 0700   ok
```

| Flag | Meaning |
| ---- | ------- |
| `--config PATH` | Configuration file. Defaults to `/etc/datum/agent.yaml`. |

Printing the resolved values instead of echoing the file is what the command is for, since a key in
the wrong place looks correct when a file is read back verbatim. An unrecognised key is reported as
a warning after the values, and does not stop the agent.

## datum revision

Shows or clears the revision this host has accepted.

```console
# datum revision show
8b91f2036f4e6b0f5a7c1d2e3f4a5b6c7d8e9f01
```

| Subcommand | Meaning |
| ---------- | ------- |
| `show` | Print the accepted revision, or say the host is at first contact. |
| `clear --yes` | Forget it, so the next signed revision becomes the baseline. |

Clearing is the documented recovery from
[a history that has been rewritten](../security/repository-fetch.md#when-the-recorded-revision-is-absent),
where the host cannot tell whether a candidate moves forward or backward and refuses every
pass until somebody says which baseline to trust.

It needs `--yes` because it disarms downgrade protection until the next pass completes. That is a
one-shot action and not a setting, since a control that can be switched off during an incident and
left off is a suggestion.

```console
# datum revision clear --yes
cleared 8b91f2036f4e6b0f5a7c1d2e3f4a5b6c7d8e9f01
the next signed revision this host sees becomes its baseline
```

## datum version

Reports what this binary is.

```console
$ datum version

datum 0.1.0~dev
revision  87c303c0399fd34ab377c7dbd827e739eaf822e1, committed 2026-09-23T14:36:12Z
platform  linux/amd64, go1.25.5
schema    v1alpha1
```

The schema line lists every [schema version](../repository/schema-versions.md) this binary
reads, newest last. It answers whether a given binary can be pointed at a given repository
without having to run it and read the parse error.

`datum --version` prints the same thing, because a binary is asked its version both ways and
guessing wrong should not be an error.

The revision line is stamped by the Go toolchain from the checkout the binary was built in, so
a build carries its own commit without anything being passed in. A build from a tree with
uncommitted changes says so on that line, and a build with no version passed in reports
`unknown` instead of a number it was never given. A build from a checkout rather than a release
carries the version `0.1.0~dev`, which sorts below any released version.

## Verifying a release

Each release publishes `SHA256SUMS`, a detached signature over it as `SHA256SUMS.asc`, and the
public half of the signing key as `signing-key.asc`.

```console
$ gpg --import signing-key.asc
$ gpg --verify SHA256SUMS.asc SHA256SUMS
$ sha256sum --check SHA256SUMS
```

The signature covers the checksum file, which covers every artefact listed in it. Altering an
artefact fails the checksum and altering the checksum file fails the signature.

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

| Flag | Meaning |
| ---- | ------- |
| `--repo DIR` | Directory in which to create Datum files. Defaults to the current directory. |
| `--name NAME` | Name for the `Fleet` document. Defaults to `example`. |
| `--with-examples` | Also write a commented example host and resource. |

The documents are written with the schema version this agent understands. If any generated-document
path already exists, `init` stops before writing anything, so a second run reports the clash and
leaves the first result alone. If writing a document fails, `init` removes the documents it created
before reporting the error.

The command warns when the directory is not inside a Git work tree and creates the repository
anyway. The reasoning behind the created files being this few, and behind the command leaving Git
alone, is set out under [creating a repository](../repository/index.md#datum-init).

## datum validate

Parses every document, checks each against its declared schema version, then resolves and
builds a graph for every host in the fleet.

```text
$ datum validate

fleet      example
revision   9c02ab
hosts      500
layers     14
schema     v1alpha1 (612 documents)

resolved 500 hosts, 0 errors
```

The schema line counts the documents declaring each version. A repository part-way through
a migration lists both, which is where the remaining work shows up.

```text
schema     v1alpha1 581, v1beta1 31, migration in progress
```

Errors report how many hosts they affect instead of repeating once per host. Everything
`validate` reports is an error, so there is nothing to promote and no `--strict`. There is
no separate lint command either, for [the reasons given alongside
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

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0` | Success, and no differences were found by a read-only command. |
| `1` | Error. Resolution failed, validation failed, or an action failed. |
| `2` | Differences found. From `diff`, `plan`, an observe-mode `reconcile`, and `status` on a drifted host. |
| `3` | Could not acquire the [pass lock](../reconciliation/locking.md). Another pass is running. |

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
