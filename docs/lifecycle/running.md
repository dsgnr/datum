# Running Datum on a host

This page covers installing the binary and running passes on a schedule. Everything here
works today. The resident agent described under [installation](installation.md) and
[agent configuration](../reference/agent-config.md) does not exist yet, so a systemd timer
takes its place.

## Install the binary

There are no releases. Build from source and copy the result to the host.

```console
$ git clone https://github.com/dsgnr/datum.git
$ cd datum
$ make build-linux
$ scp bin/datum-linux-amd64 web-001:/tmp/datum
```

On the host:

```console
# install -m 0755 /tmp/datum /usr/local/bin/datum
# datum --help
```

The binary is statically linked with cgo disabled, so it needs nothing else installed.
Cross-compiling needs no toolchain beyond Go.

## Put the fleet on the host

Nothing fetches from a remote yet, so the repository has to be on disk already. Clone it
and pull it before each pass.

```console
# git clone https://git.example.com/fleet.git /var/lib/datum/fleet
```

Datum reads the fleet directory, which is the one holding the `Fleet` document. In a
repository that holds other things, point at the subdirectory:

```console
# datum plan --host web-001 --repo /var/lib/datum/fleet/infra/fleet
```

!!! warning "The revision is not verified"

    Signature verification is not implemented. Whatever is in the working tree is what
    gets applied, so the clone has to be somewhere only root can write.
    [Repository trust](../security/repository-trust.md) describes the checks that are
    designed and not yet built.

## The state directory

Datum keeps the pass lock and its reports in `/var/lib/datum`. The directory must be mode
`0700`, and a pass refuses to run otherwise rather than correcting it, because a readable
state directory discloses plans.

```console
# install -d -m 0700 -o root -g root /var/lib/datum
```

## Run a pass by hand first

Read before writing, since `plan` changes nothing and exits 2 when something
differs.

```console
# datum plan --host web-001 --repo /var/lib/datum/fleet
# datum reconcile --host web-001 --repo /var/lib/datum/fleet
# datum status
```

`--host` is required. Nothing works out which host a machine is yet, which is the part
[enrolment](enrolment.md) describes.

## Run passes on a timer

A unit and a timer give the interval and the splay the
[scheduler](../reconciliation/scheduling.md) is designed to provide. `Type=oneshot` because
a pass runs to completion and exits.

```ini title="/etc/systemd/system/datum.service"
[Unit]
Description=Datum reconciliation pass
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStartPre=/usr/bin/git -C /var/lib/datum/fleet pull --ff-only --quiet
ExecStart=/usr/local/bin/datum reconcile --host %H --repo /var/lib/datum/fleet
# A pass that cannot finish has to be abandoned rather than left holding the lock.
TimeoutStartSec=15m
```

```ini title="/etc/systemd/system/datum.timer"
[Unit]
Description=Run a Datum pass every 30 minutes

[Timer]
OnBootSec=5m
OnUnitActiveSec=30m
# Spreads passes across the fleet so several hundred hosts do not fetch at once.
RandomizedDelaySec=5m
Persistent=true

[Install]
WantedBy=timers.target
```

```console
# systemctl daemon-reload
# systemctl enable --now datum.timer
# systemctl list-timers datum.timer
```

`%H` expands to the hostname, which only works where the hostname matches the `Host`
document name. Write the name out otherwise.

## Exit codes in a timer

`reconcile` exits 0 when it converged or changed something, 1 on failure, 2 when it
reported drift it did not apply, and 3 when another pass holds the lock. systemd treats
anything non-zero as a failed unit, so a host reporting drift shows up as a failed timer.
Use `--wait` if a queued pass is preferable to exit 3.

To alert on failures rather than on drift:

```ini
ExecStart=/usr/local/bin/datum reconcile --host %H --repo /var/lib/datum/fleet
SuccessExitStatus=2 3
```

## Read the result

`status` reads the last report from the state directory. It touches neither the repository
nor the host, so it is cheap to call from a monitoring check.

```console
$ datum status
$ datum status --output json | jq .hostState
```

Reports are retained for the twenty most recent passes. There is no metrics endpoint yet,
so [metrics](../observability/metrics.md) is a design rather than something to scrape.

## What is missing

Nothing here is a substitute for the designed agent. A timer running `git pull` has none
of the properties that matter for a fleet:

| Missing | Designed in |
| ------- | ----------- |
| Signature verification of the revision | [Repository trust](../security/repository-trust.md) |
| Falling back to the last good revision | [Last known good](../reconciliation/last-known-good.md) |
| Host identity rather than `--host` | [Enrolment](enrolment.md) |
| Backing off when the upstream fails | [Scheduling](../reconciliation/scheduling.md) |
| Metrics on a port | [Metrics](../observability/metrics.md) |

Use this to try Datum on a machine you can rebuild. It is not a production deployment.
