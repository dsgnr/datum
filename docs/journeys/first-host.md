# Journey: one Ubuntu server from Git

The smallest useful thing Datum can do. One machine, one repository, nothing central, starting from an
existing server that is already running nginx.

## The repository

Four documents. A fleet root, a host, a layer, and the resources.

```text
fleet/
├── datum.yaml
├── hosts/
│   └── web-001/
│       └── host.yaml
└── roles/
    └── web/
        ├── layer.yaml
        ├── nginx.yaml
        └── files/
            └── nginx.conf
```

```yaml title="fleet/hosts/web-001/host.yaml"
datum: v1alpha1
type: Host

name: web-001

labels:
  role: web
```

```yaml title="fleet/roles/web/layer.yaml"
datum: v1alpha1
type: Layer

name: role-web

precedence: 30
match:
  labels:
    role: web
```

```yaml title="fleet/roles/web/nginx.yaml"
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
---
datum: v1alpha1
type: File

name: nginx-config

requires:
  - Package[nginx]

desired:
  path: /etc/nginx/nginx.conf
  owner: root
  group: root
  mode: "0644"
  source: files/nginx.conf
---
datum: v1alpha1
type: Service

name: nginx

requires:
  - Package[nginx]
restartOn:
  - File[nginx-config]

desired:
  state: running
  enabled: true
```

Three resources, on a server that already has all three things. [Declared-only
ownership](../adr/0009-declared-only-ownership.md) makes adopting an existing machine safe, since
everything else on the server is unmanaged and untouched.

## The agent

```yaml title="/etc/datum/agent.yaml"
host: web-001

source:
  url: https://git.example.com/fleet.git
  branch: main
  credential: /etc/datum/credentials/git

reconciliation:
  mode: observe

state: /var/lib/datum
```

Starting in [`observe` mode](../concepts/reconciliation-modes.md) is the recommended first step. The
first pass will report what Datum would change without changing anything, which on a machine that
matters is the difference between a safe experiment and a surprise.

## The first pass

**Obtaining a revision.** The agent fetches `main` and gets `7ab21f`. With no recorded revision in
its state directory, this becomes the baseline, and the [trust-on-first-use
gap](../security/repository-trust.md#first-contact) applies, and the host cannot detect a downgrade
on its first pass because it has nothing to compare against.

**Resolution.** The resolver reads `Host[web-001]`, finds its labels, evaluates every layer's matcher
against them, and matches `role-web`. One layer contributes, so there is no merging to do and no
precedence to resolve. The effective manifest holds three resources, each with `roles/web` recorded as
its provenance.

**Provider selection.** The agent reads `/etc/os-release`, finds `ID=ubuntu` with `ID_LIKE=debian`, and
resolves a [capability set](../providers/capabilities.md).

```text
Package  -> apt          (via ID_LIKE=debian)
File     -> posix-file
Service  -> systemd
```

**Graph construction.** Two edges, from `requires` and `restartOn`, both pointing at `Package[nginx]`
and `File[nginx-config]`. No cycles, no unresolved references, no duplicate target identities.

**Observation.** Three reads. The package is installed at `1.24.0-2`. The file exists with mode `0644`
and a content digest. The unit is active and enabled.

**Diff.** The content digest does not match the repository's copy, because the file on the server was
configured by hand months ago and the repository's copy came from somewhere else. Everything else
matches.

**Plan.**

```text
update   File[nginx-config]
         content  differs
         from     roles/web

update   Service[nginx]
         reason   File[nginx-config] changed, restartOn matched

none     Package[nginx]      present, 1.24.0-2

0 to create, 2 to update, 0 to remove, 0 to skip, 1 unchanged
```

**The pass stops.** `observe` mode applies nothing. The host reports `drifted`.

That plan is the useful output of the whole exercise. It says the repository's `nginx.conf` differs from
the server's, which is the thing to resolve before enforcing anything. The right move is almost always
to copy the server's working configuration into the repository, confirm the plan comes back empty, and
only then switch to enforcing.

## Converging

After the repository's `files/nginx.conf` is replaced with the server's actual content, the next pass
observes, diffs, and finds everything matching. The plan is empty, the host reports `converged`, and
nothing is applied.

That empty plan is the signal that enforcing is now safe, because it means enforcing would do nothing.
Changing the mode to `enforce` and reconciling again produces the same empty plan and the same
`converged`.

## Steady state

Passes continue on a schedule, each one resolving, observing, diffing, and finding nothing to do.

```text
$ datum status

host       web-001
revision   7ab21f
manifest   sha256:3f2a9c4e
outcome    converged
finished   2 minutes ago

1 unchanged, 2 unchanged
```

The metrics file is rewritten each pass, so
`datum_pass_last_success_timestamp_seconds` advances and the
[staleness alert](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch) stays quiet.

## What this journey tests

The single-host path needs nothing central, nothing inbound for reconciliation itself, and no identity
beyond a name in a local file, which is the [direct
mode](../architecture/deployment-models.md) claim made concrete. The only listener is the optional
[metrics endpoint](../observability/metrics.md#exposure), bound to loopback.

More importantly it tests that adoption is incremental. A server that was configured by hand can be
brought under management three resources at a time, and the first thing Datum does is tell you where
it disagrees rather than resolving that disagreement on its own.
