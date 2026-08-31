# Agent configuration

`/etc/datum/agent.yaml` is the complete configuration for one agent, and every key in it is
documented below.

The file is a trust anchor. Every setting that determines whether Datum may change a machine is held
here rather than in the repository, and [Datum does not manage this
file](../adr/0010-no-self-managed-trust-anchors.md), so a repository cannot alter it.

!!! note "Implementation status"

    The agent reads this file, and every key and default below is the one it applies. `datum config
    check` reports the resolved values.

    The `source`, `trust` and `reconciliation` blocks are acted on. The agent fetches from
    `source.url`, verifies against `trust.signers` in whichever `trust.require` mode is set, and
    refuses a revision that does not descend from the one it accepted.

    Three keys are read and do nothing. `source.maxSourceSize` does not yet bound a `File` content
    source. `trust.strictPaths` is inert. `secrets` accepts only the `file` provider that
    [secret resolution](../resources/secrets.md) has yet to implement, so a `secretRef` resource
    fails either way.

    `source.credential` works for an ssh identity file and not for an https token.

## The whole file

```yaml title="/etc/datum/agent.yaml"
host: web-001

source:
  url: https://git.example.com/fleet.git
  branch: main
  credential: /etc/datum/credentials/git
  fetchTimeout: 5m
  maxRepositorySize: 1GiB
  maxSourceSize: 16MiB

trust:
  signers: /etc/datum/allowed-signers
  require: signed-tag
  tagPattern: "release-*"
  requireDescendant: true
  strictPaths: false

reconciliation:
  mode: enforce
  interval: 30m
  splay: 30m
  timeout: 15m
  actionTimeout: 5m

secrets:
  provider: file
  path: /etc/datum/secrets

metrics:
  listen: 127.0.0.1:10056
  textfile: /var/lib/node_exporter/textfile/datum.prom

state: /var/lib/datum
```

Only `host` and `source.url` have no usable default. Everything else may be omitted.

## Identity

| Key | Default | Meaning |
| --- | ------- | ------- |
| `host` | None, required | The [host name this machine claims](../architecture/host-identity.md#where-identity-comes-from). |

The name has to match a `Host` document in the repository, and a machine claiming a name with no such
document fails its pass with an error naming what it claimed. There is no fallback to the system
hostname, because a hostname assigned by DHCP would silently re-resolve a machine against different
desired state.

## Source

| Key | Default | Meaning |
| --- | ------- | ------- |
| `source.url` | None, required | The Git remote desired state is read from. |
| `source.branch` | `main` | The ref tracked, which is also [which rollout ring this host is in](../reconciliation/staged-rollout.md#rings-are-branches). |
| `source.credential` | None | Path to a read-only credential for the remote. |
| `source.fetchTimeout` | `5m` | Bound on the [fetch](../security/repository-fetch.md#limits). |
| `source.maxRepositorySize` | `1GiB` | Largest repository the agent will accept. |
| `source.maxSourceSize` | `16MiB` | Largest single content source a `File` may read. |

The URL is configuration, not something discovered from the network, because [letting DNS or DHCP
nominate it](../lifecycle/installation.md#the-repository-url-is-configuration-not-discovery) would
hand whoever runs the network the ability to redirect a root process.

`source.branch` doing double duty as ring membership is deliberate. A ring is a control over how far
a change spreads, so it belongs on the machine instead of in the repository the change arrives
through.

## Trust

| Key | Default | Meaning |
| --- | ------- | ------- |
| `trust.signers` | `/etc/datum/allowed-signers` | The [keys](../security/repository-trust.md#verifying-that-a-revision-is-genuine) that may authorise desired state. |
| `trust.require` | `signed-commit` | One of `signed-commit`, `signed-tag` or `none`. |
| `trust.tagPattern` | None | Which tags are candidates under `signed-tag`. |
| `trust.requireDescendant` | `true` | Refuse a revision that is not a descendant of the [accepted revision](../security/repository-trust.md#verifying-that-a-revision-is-current). |
| `trust.strictPaths` | `false` | Refuse rather than warn when a managed path passes through a [directory a non-root user can write](../security/provider-safety.md#untrusted-path-components). |

`require` defaults to `signed-commit`, so an unconfigured fleet gets an error rather than applying
unverified desired state. Setting `none` is [reported through a
metric](../observability/metrics.md#security-controls).

`requireDescendant` defaults on. Recovery from a rewritten history is [a one-shot operator
command](../security/repository-trust.md#the-cases-this-makes-awkward) rather than a setting, so the
control cannot be left disabled.

## Reconciliation

| Key | Default | Meaning |
| --- | ------- | ------- |
| `reconciliation.mode` | `enforce` | `enforce` applies changes, `observe` [only reports them](../concepts/reconciliation-modes.md). |
| `reconciliation.interval` | `30m` | How often [a pass runs](../reconciliation/scheduling.md#the-interval). |
| `reconciliation.splay` | The interval | How widely passes are [spread across the fleet](../reconciliation/scheduling.md#passes-are-spread-deterministically). |
| `reconciliation.timeout` | `15m` | Bound on [one pass](../reconciliation/failure-handling.md#a-pass-is-bounded). |
| `reconciliation.actionTimeout` | `5m` | Bound on one provider action. |

`timeout` defaults shorter than `interval` so that a pass always finishes or gives up before its
successor is due.

`mode` defaults to `enforce`. Starting a host in `observe` is [the recommended first
step](../journeys/first-host.md) when adopting an existing machine.

## Secrets

| Key | Default | Meaning |
| --- | ------- | ------- |
| `secrets.provider` | None | Which backend resolves [secret references](../resources/secrets.md). |
| `secrets.path` | None | Directory the `file` provider reads from. |

With the block omitted, a resource carrying a `secretRef` fails rather than resolving the reference
to an empty value.

Backends are named, and none is configured by supplying a command to run. See
[ADR-0011](../adr/0011-no-command-execution-from-desired-state.md).

## Metrics

| Key | Default | Meaning |
| --- | ------- | ------- |
| `metrics.listen` | `127.0.0.1:10056` | Address the [endpoint](../observability/metrics.md#binding-and-exposure) binds to, or `none` to disable it. |
| `metrics.textfile` | None | Path to write a textfile-collector file, in addition to or instead of the listener. |

The listener binds loopback by default rather than all interfaces, since it runs inside a root
process holding repository credentials.

## State

| Key | Default | Meaning |
| --- | ------- | ------- |
| `state` | `/var/lib/datum` | Directory for the accepted revision, the pass lock and pass reports. |

The directory is root-owned at mode `0700` with files at `0600`, and the agent exits at startup if
the permissions differ. A readable state directory discloses plans, and a writable one [allows a
local attacker to reset downgrade
protection](../security/threat-model.md#an-attacker-with-root-on-one-managed-host).

```text
/var/lib/datum/accepted-revision      the one thing carried between passes
/var/lib/datum/pass.lock              held for the duration of a pass
/var/lib/datum/reports/               the result of recent passes
```

Reports are named after the time the pass finished and the twenty most recent are retained, which is
enough history to tell a new failure from a recurring one. The count is fixed and not configurable.

## What is deliberately not configurable here

Resource types, providers and desired state of any kind. This file says how the agent reaches its
desired state and how much it trusts what it finds, and never what that desired state is.

Provider selection is also absent, because a provider is [chosen from the host, never
named](../providers/selection.md). A fleet wanting to force one has [no way
to](../development/open-questions.md), which is an open question instead of an omission from this
file.

## Validating the file

```text
$ datum config check

/etc/datum/agent.yaml       ok
  host                      web-001
  source.url                https://git.example.com/fleet.git
  source.branch             main
  trust.require             signed-tag
  trust.signers             4 keys
  reconciliation.mode       enforce
  reconciliation.interval   30m, offset 00:07
  secrets.provider          file
  metrics.listen            127.0.0.1:10056
  state                     /var/lib/datum   root 0700   ok
```

Reporting the resolved values including defaults, instead of echoing the file, is what the command
is for. Most mistakes in a configuration file of this shape are a key in the wrong place or a
setting somebody believes is on, and both look correct when the file is read back verbatim.

Reporting the schedule offset matters for the same reason, because a
[deterministic offset](../reconciliation/scheduling.md#passes-are-spread-deterministically) is
predictable and nobody can compute it by hand.

!!! note "Open question"

    Whether an unrecognised key is an error or a warning is undecided. Refusing makes a typo loud at
    the moment it is introduced, and it also means a host running an older agent stops reconciling the
    moment somebody adds a setting a newer agent understands, which is the same
    [fail-closed trade](../repository/schema-versions.md#older-agent-newer-repository) the document
    schema makes and has a worse blast radius here, because this file is not reviewed in a pull
    request.
