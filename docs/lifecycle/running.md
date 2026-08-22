# Running Datum on a host

This page covers installing the binary, configuring the agent and running it as a service.
Everything here works today. What is still missing is listed at the end, and the largest
gap is that nothing fetches from a remote yet, so the repository has to be on disk.

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

Nothing fetches from a remote yet, so the repository has to be on disk already. Clone it,
and pull it on whatever schedule suits until the agent does that itself.

```console
# git clone https://git.example.com/fleet.git /var/lib/datum/fleet
```

Datum reads the fleet directory, which is the one holding the `Fleet` document. In a
repository that holds other things, point at the subdirectory:

```console
# datum plan --host web-001 --repo /var/lib/datum/fleet/infra/fleet
```

## The state directory

Datum keeps the pass lock and its reports in `/var/lib/datum`. The directory must be mode `0700`,
and the agent refuses to run otherwise instead of correcting it, because a readable state directory
discloses plans.

```console
# install -d -m 0700 -o root -g root /var/lib/datum
```

## Configure the agent

The agent reads `/etc/datum/agent.yaml`. Every key and its default is in the
[agent configuration reference](../reference/agent-config.md), and the smallest file that
works names the host and the source.

```yaml title="/etc/datum/agent.yaml"
host: web-001

source:
  url: https://git.example.com/fleet.git

trust:
  require: none

reconciliation:
  mode: observe
  interval: 30m
```

`trust.require: none` is there because signature verification is not implemented, and the
default of `signed-commit` would refuse every revision. It
[reports itself through a metric](../observability/metrics.md#security-controls), which is
the point of it being a setting rather than a silent default.

Starting in `observe` mode suits a machine being adopted, since the first pass then reports what it
would change without changing it.

Check the file before starting anything. The resolved values include the defaults, which is
what catches a key in the wrong place.

```console
# datum config check

/etc/datum/agent.yaml       ok
  host                     web-001
  source.url               https://git.example.com/fleet.git
  source.branch            main
  trust.require            none
  trust.requireDescendant  true
  reconciliation.mode      observe
  reconciliation.interval  30m, offset 06:51
  reconciliation.timeout   15m
  metrics.listen           127.0.0.1:10056
  state                    /var/lib/datum   root 0700   ok
```

The offset is this host's position within the interval, derived from its name. It is the line to
read, because it says when a pass is due and nobody can compute it by hand.

## Run a pass by hand first

Read before writing, since `plan` changes nothing and exits 2 when something
differs.

```console
# datum plan --host web-001 --repo /var/lib/datum/fleet
# datum reconcile --host web-001 --repo /var/lib/datum/fleet
# datum status
```

## Run the agent as a service

The agent is a long-lived process. It schedules its own passes, so there is no timer.

```ini title="/etc/systemd/system/datum.service"
[Unit]
Description=Datum reconciliation agent
Documentation=https://getdatum.sh/
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
ExecStart=/usr/local/bin/datum agent --repo /var/lib/datum/fleet
# The agent stops scheduling on SIGTERM and abandons any pass still running, which
# leaves the host partially applied in the way an interrupted pass always does.
KillSignal=SIGTERM
TimeoutStopSec=30s
Restart=on-failure
RestartSec=30s

[Install]
WantedBy=multi-user.target
```

```console
# systemctl daemon-reload
# systemctl enable --now datum.service
# systemctl status datum.service
# journalctl -u datum.service -f
```

`--repo` is there because fetching is not implemented. It goes away when the agent reads
`source.url` itself.

The agent runs no pass at startup. It waits for its own offset within the first interval,
so a fleet rebooting together does not reconcile all at once.

## Scrape the metrics

The agent serves the last pass on loopback.

```console
$ curl -s http://127.0.0.1:10056/metrics | grep datum_pass_last_success
```

A scrape reads nothing. It returns what the last pass recorded, so scraping often costs nothing on
the host. The full catalogue is under [metrics](../observability/metrics.md), and the first alert to
add is `up{job="datum"} == 0`, which is why the endpoint lives in the process whose health is in
question.

## Read the result

`status` reads the last report from the state directory. It touches neither the repository
nor the host, so it is cheap to call from a monitoring check.

```console
$ datum status
$ datum status --output json | jq .hostState
```

Reports are retained for the twenty most recent passes.

## Exit codes

`reconcile` exits 0 when it converged or changed something, 1 on failure, 2 when it reported drift
it did not apply, and 3 when another pass holds the lock. That matters for a monitoring check, not
for the service, because the agent holds the exit code itself and reports outcomes through metrics.

`agent` exits 0 when it was asked to stop and 1 when it could not start, which is why
`Restart=on-failure` does not fight a deliberate `systemctl stop`.

## What is missing

The agent runs and reconciles on its interval. Three things it is designed to do are not
implemented yet.

| Missing | Designed in |
| ------- | ----------- |
| Fetching from a remote, so `--repo` is required | [Repository fetch](../security/repository-fetch.md) |
| Signature verification of the revision | [Repository trust](../security/repository-trust.md) |
| Falling back to the last good revision | [Last known good](../reconciliation/last-known-good.md) |

Those three are one piece of work rather than three, because a fallback needs something to
fall back from and a verified revision is what makes falling back meaningful. Until they
exist, a host applies whatever is in its checkout, and that is the reason this is not yet a
production deployment.
