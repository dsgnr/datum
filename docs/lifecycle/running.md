# Running Datum on a host

Installing the agent, configuring it and running it as a service all work today, including fetching
and verifying a revision. What is still missing is listed at the end.

## Install the agent

There are no releases yet, so build a package and copy it to the host.

```console
$ git clone https://github.com/dsgnr/datum.git
$ cd datum
$ make package
$ scp dist/datum_0.1.0~dev_amd64.deb web-001:/tmp/
```

On the host:

```console
# apt-get install /tmp/datum_0.1.0~dev_amd64.deb
# datum --help
```

`make package` produces a `.deb` and an `.rpm`, each built by its own distribution's tools in
a container, so no packaging toolchain is needed on the machine doing the build. Installing
either one puts down what [installation](installation.md#what-installation-provides)
describes, along with the unit shown below and a default configuration that names no host.

The service is installed and left disabled. A machine that has not been
[enrolled](enrolment.md) has no identity, so an agent enabled at install time would fail every
pass until somebody gave it one.

Copying the binary on its own still works, and then the directories, the configuration and
the unit are the fleet's own to create.

```console
$ make build-linux
$ scp bin/datum-linux-amd64 web-001:/tmp/datum
# install -m 0755 /tmp/datum /usr/bin/datum
```

The binary is statically linked with cgo disabled, so it needs no shared libraries.
Cross-compiling needs no toolchain beyond Go. What it does need on the host is `git`, and
`ssh-keygen` for the default of ssh-signed commits, which is what the packages depend on.

## Give the host a signing key to trust

The agent verifies that a revision was signed by a key in `trust.signers` before it applies
it. The file is in ssh allowed-signers format, and it is
[not something Datum manages](../adr/0010-no-self-managed-trust-anchors.md), so whatever
builds the machine puts it there.

```console
# install -d -m 0755 /etc/datum
# install -m 0644 allowed-signers /etc/datum/allowed-signers
```

```text title="/etc/datum/allowed-signers"
release@example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...
```

A fleet signing commits with ssh keys needs nothing else. A fleet using gpg keeps its trust
root in a keyring instead, which works for verification and means
[`datum_trust_signer_info`](../observability/metrics.md#security-controls) reports nothing.

Set `trust.require: none` to run without any of this, which is a decision that [reports itself
through a metric](../observability/metrics.md#security-controls) rather than being invisible.

## The state directory

Datum keeps the pass lock and its reports in `/var/lib/datum`. The directory must be mode `0700`,
and the agent refuses to run otherwise instead of correcting it, because a readable state directory
discloses plans.

```console
# install -d -m 0700 -o root -g root /var/lib/datum
```

The packages create it already, so the line above is for a host built by copying the binary.

## Configure the agent

The agent reads `/etc/datum/agent.yaml`. Every key and its default is in the
[agent configuration reference](../reference/agent-config.md), and the smallest file that
works names the host and the source.

```yaml title="/etc/datum/agent.yaml"
host: web-001

source:
  url: https://git.example.com/fleet.git

reconciliation:
  mode: observe
  interval: 30m
```

`trust.require` defaults to `signed-commit`, so that file expects every commit on the tracked branch
to be signed by a key in `/etc/datum/allowed-signers`. A fleet signing releases instead of every
commit uses `signed-tag` with a `tagPattern`.

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
  trust.require            signed-commit
  trust.signers            /etc/datum/allowed-signers   1 key
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

These read a checkout rather than fetching, so point them at one. Cloning by hand is the easiest way
to see what a host would do before the service starts.

```console
# git clone https://git.example.com/fleet.git /tmp/fleet
# datum plan --host web-001 --repo /tmp/fleet
# datum reconcile --host web-001 --repo /tmp/fleet
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
ExecStart=/usr/bin/datum agent
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

The agent clones `source.url` into `/var/lib/datum/repository` on its first pass and fetches
after that. Naming a checkout with `--repo` is still possible and skips fetching and
verification entirely, which is refused unless `trust.require` is `none`, so a host cannot end
up applying an unverified tree while its configuration says otherwise.

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

The agent fetches, verifies, falls back, and refuses desired state that targets its own
controls. Three things on the pages this one links to are specified and not implemented.

| Missing | Designed in |
| ------- | ----------- |
| `trust.strictPaths` | [Provider safety](../security/provider-safety.md#untrusted-path-components) |
| `source.maxSourceSize` | [Limits](../security/repository-fetch.md#limits) |
| Secret references | [Secrets](../resources/secrets.md) |

Two limits of what is implemented need stating instead of being left to discover. Verifying a
revision is no use on a host whose clock is wrong in a way that matters for key expiry, which is
[time](../security/time.md), and nothing enforces a freshness bound yet. And the refusal of
resources that target Datum's own files protects Datum's controls from being disabled by desired
state, which is not the same as protecting the host from a repository that is trusted to configure
it. A `File` writing `/etc/sudoers.d/` is root-equivalent and permitted, because that is what a
configuration system is for.
