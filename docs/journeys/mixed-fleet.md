# Journey: 500 mixed hosts

Ubuntu web servers, RHEL database servers, and Alpine edge nodes, in two sites and two environments,
from one repository. This journey is about composition and about where distribution differences are
allowed to surface.

## The repository

```text
fleet/
├── datum.yaml
├── base/
│   ├── layer.yaml
│   └── packages.yaml
├── environments/
│   ├── production/
│   └── staging/
├── sites/
│   ├── london/
│   └── frankfurt/
├── roles/
│   ├── web/
│   ├── web-apt/
│   ├── database/
│   └── edge/
└── hosts/
    ├── web-001/
    └── ...
```

Five hundred `Host` documents, each carrying labels and nothing else.

```yaml title="fleet/hosts/web-001/host.yaml"
datum: v1alpha1
type: Host

name: web-001

labels:
  environment: production
  site: london
  role: web
  architecture: amd64
  os: ubuntu
```

Adding the five hundred and first host is one document. Nothing is copied, because configuration is
[reused by matching](../fleet/index.md).

## Resolution differs per host

Each host resolves independently, and the layers that match differ.

```text
web-001    base(0)  environments/production(10)  sites/london(20)  roles/web(30)  roles/web-apt(35)
db-001     base(0)  environments/production(10)  sites/london(20)  roles/database(30)
edge-014   base(0)  environments/production(10)  sites/frankfurt(20)  roles/edge(30)
```

Layers fold in [precedence](../fleet/precedence.md) order, lowest first. A base package list applies
everywhere, production kernel tuning applies to production, a site's resolver configuration applies to
that site, and a role's service configuration applies to that role.

The composed result is three different effective manifests with three different digests, produced from
one repository by one mechanism.

## Where the distributions differ

Most resources are distribution neutral and need no special handling. `Package[curl]` with
`state: present` resolves on all three, and the [capability
set](../providers/capabilities.md) differs beneath it.

```text
                Ubuntu          RHEL            Alpine
Package    ->   apt             dnf             apk
Service    ->   systemd         systemd         openrc
File       ->   posix-file      posix-file      posix-file
Sysctl     ->   proc-sys        proc-sys        proc-sys
```

Three of the four capabilities are identical across all three distributions, and only `Package` and
`Service` differ. The unit of difference is the capability, not the distribution, which is why the
provider boundary contains most of it.

What the boundary cannot contain is a package that is named differently. The web servers need a
version-pinned nginx, and an apt version string means nothing to `dnf`, so that pinning lives in a
layer whose matcher narrows to the apt-family hosts.

```yaml title="fleet/roles/web-apt/layer.yaml"
datum: v1alpha1
type: Layer

name: role-web-apt

precedence: 35
match:
  labels:
    role: web
  oneOf:
    os: [debian, ubuntu]
```

Matching two values with `oneOf` rather than writing one layer per distribution is what keeps the
pinning in a single place, since Debian and Ubuntu take the same version string even though they are
different distributions.

The `os` label is an [ordinary declared
label](../providers/multi-distribution.md#handling-a-genuine-difference) with no special meaning.
The difference is visible in the repository, in one layer, and not hidden in a conditional, which is
the [second of the two
places](../providers/multi-distribution.md#where-the-difference-is-allowed-to-surface) a difference
is allowed to appear.

!!! note "Open question"

    Declaring `os: ubuntu` by hand duplicates something the machine already knows and can be wrong. This
    is the [most consequential unresolved
    question](../development/open-questions.md) in the design, because letting matchers read observed
    facts would remove the duplication and would break offline resolution.

## Alpine has no Service provider

Alpine is in the target distribution list and does not use systemd. The `Service` capability on those
hosts resolves to `openrc`, and no such provider exists.

Until it does, `Service` resources on the edge nodes are
[skipped](../providers/selection.md#when-no-provider-matches) with the reason recorded, and those
hosts report [`degraded`](../concepts/state.md#host-state-across-passes) instead of `converged`.

```text
skip   Service[node-agent]
       reason   no Service provider supports ID=alpine
```

That is the correct behaviour and it is not a good outcome. The hosts reconcile everything else
successfully, the gap is visible in `datum_resources{state="skipped"}` and in the host state rather
than being silently absent, and a fleet dashboard shows nine degraded hosts instead of pretending
five hundred are fine.

## Scaling properties

Resolution is per host and independent, so five hundred hosts are five hundred independent resolutions
with no shared state and no coordination. A host resolving does not wait for or affect any other.

Each host reads the whole repository, which is a [security
limitation](../architecture/host-identity.md) and not a scaling one, because every host can read
every other host's configuration. At five hundred hosts that has to be weighed against what the
arrangement buys.

Five hundred agents polling one Git remote is a load question the design has not answered. The
[reconciliation interval](../concepts/reconciliation.md#retry) is undecided precisely because it has to
be short enough that drift does not persist and long enough that a fleet does not overwhelm a remote,
and that trade-off is exactly what this journey exposes.

## Observability at this size

Per-host inspection stops being viable, so the [metrics](../observability/metrics.md) are what make the
fleet legible.

```text
hosts by state       converged 486  degraded 9  failed 3  awaiting-reboot 2
worst revision lag   4h 12m
hosts behind newest  11
```

The revision lag query works without anything knowing what the newest revision is, because
`max(datum_revision_applied_timestamp_seconds)` across the fleet supplies it. The
[limitation](../observability/metrics.md#answering-is-this-host-up-to-date) is that a fleet where every
host is equally behind reports no lag at all.

## What this journey tests

Composition has to produce different manifests for different hosts from one repository without
copying, which labels and matchers do. Distribution differences have to land either inside a
provider or in a narrowly-matched layer, and never in a silently-varying field, which the capability
model and the `web-apt` layer between them achieve. And a partially-supported distribution has to be
visibly partial instead of quietly incomplete, which `degraded` and `skipped` provide.
