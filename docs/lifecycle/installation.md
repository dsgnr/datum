---
description: "Build and install Datum, configure a signed Git source, and run the systemd agent in observe mode before enabling configuration enforcement."
seo_title: "Install and run the Linux agent - Datum"
---

# Install and run the agent

Build and install Datum on a Linux host, configure a signed Git source, then move from
observing drift to applying changes. For a first look without installing a service,
start with the [quickstart](index.md).

Installing, configuring and running the agent are implemented. Automated enrolment and
decommissioning remain design work; see [enrolment](enrolment.md) and
[leaving the fleet](decommissioning.md) for those boundaries.

## What installation provides

Installation provides the agent binary, the providers shipped with it, and the directories the agent
needs at runtime.

```text
/usr/bin/datum                 the agent and command line
/etc/datum/                    configuration, empty apart from defaults
/var/lib/datum/                agent state, empty
```

None of those paths are managed by Datum itself, which is the subject of
[ADR-0010](../adr/0010-no-self-managed-trust-anchors.md), so an installation can be replaced by a
package upgrade without a reconciliation pass being involved.

## Building from source

The commands below need Git, Make and the Go toolchain specified in `go.mod`. Building packages
additionally needs a running Docker daemon, because each package is built by its own distribution's
tools in a container.

```console
$ git clone https://github.com/dsgnr/datum.git
$ cd datum
$ make build          # ./bin/datum for this machine
$ make build-linux    # linux/amd64 and linux/arm64 into ./bin
$ make package        # a .deb and an .rpm into ./dist
```

Applying state needs Linux, so `make build` on macOS or Windows produces a binary that reads a host
without being able to change one. The cross-compiled binaries are statically linked with cgo
disabled, and `make package` names its output for the architecture Docker reports, so an x86 machine
writes `datum_0.1.0~dev_amd64.deb` and `datum-0.1.0~dev-1.x86_64.rpm`.

`make test-package` installs both packages in throwaway containers and checks the layout above, the
directory modes and that the service was left disabled. CI runs the same target, so a package that
installs wrongly fails there rather than on a host.

The version in those file names is a placeholder that sorts below any real release, and
[`datum version`](../reference/cli.md#datum-version) reports it along with the commit the binary was
built from.

## Install the agent

There are no releases yet, so [build the packages](#building-from-source), then copy
the one the host needs.

```console
$ scp dist/datum_0.1.0~dev_amd64.deb web-001:/tmp/
```

On the host, install it with the matching package manager.

```console
# apt-get install /tmp/datum_0.1.0~dev_amd64.deb    # Debian and Ubuntu
# dnf install /tmp/datum-0.1.0~dev-1.x86_64.rpm     # Fedora and RHEL
```

[`datum version`](../reference/cli.md#datum-version) then reports which build landed. Installing
either package puts down what [the installation layout](#what-installation-provides) describes,
along with the unit shown below and a default configuration that names no host.

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

Replace the illustrative key with the public key used to sign your repository commits.

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

For a private repository, provision read access for the service account as well. The
implemented `source.credential` accepts an SSH identity file, not an HTTPS token; use an
SSH source URL with it. See [repository credentials](../security/repository-trust.md#repository-credentials)
and the [source configuration](../reference/agent-config.md) for details.

Starting in `observe` mode lets the first pass report what it would change without applying it.

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

The offset is this host's position within the interval, derived from its name; it tells you
when a pass is due.

## Preview the host

The source repository must contain a `Host` named `web-001` and the resources it should
manage. Use the [quickstart](index.md) to learn the repository layout, then commit and push
your desired state with a signature from a trusted key.

To preview a local checkout on the target host:

```console
# git clone https://git.example.com/fleet.git /tmp/fleet
# datum validate --repo /tmp/fleet
# datum plan --host web-001 --repo /tmp/fleet
```

`plan` changes nothing and exits 2 when it finds drift. These commands read the checkout
directly; they do not perform the agent's signature verification. Start the service in
`observe` mode below to verify the remote and record a pass before enabling changes.

The standalone `reconcile` command defaults to `enforce`; it does not inherit the
agent's `reconciliation.mode`. Use `--mode observe` explicitly when trying it by hand.

## Run the agent as a service

The packages already install the unit below. Create it only if you copied the binary
manually. The agent schedules its own passes, so there is no timer.

```ini title="/etc/systemd/system/datum.service"
--8<-- "datum.service"
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

## Enable enforcement

After the first scheduled pass, read `datum status` and review the reported drift. Adjust
the repository until the planned changes are the ones you intend, then set
`reconciliation.mode: enforce` in `/etc/datum/agent.yaml` and restart the agent:

```console
# datum config check
# systemctl restart datum.service
```

The next scheduled pass applies changes. Check `datum status` afterwards; see the
[status reference](../reference/status.md) for failures and per-resource outcomes.
To pause changes, return to `observe` and restart, or stop the service with
`systemctl stop datum.service`.

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
nor the host, so it is cheap to call from a monitoring check. Run it as root for the installed
agent: `/var/lib/datum` is private to root. The quickstart's user-owned state directory can be
read by the user who created it with `--state`.

```console
# datum status
# datum status --output json | jq .hostState
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

## Verifying an installation

An installed machine that has not been enrolled says which part is missing, and it says it by name
instead of failing somewhere later.

```console
# datum config check

/etc/datum/agent.yaml: host is required, because a machine has to claim a name a Host document matches
```

The agent itself refuses to start for the same reason and exits 1, so a machine built from a broken
image is a unit that will not come up during the build, not a converged-looking host during an
incident. A machine that silently reports success while managing nothing is the outcome this design
exists to avoid.

`datum status` still answers, because reading a state directory needs no identity, and on a
freshly installed machine it reports that no pass has run.

!!! note "Open question"

    Refusing to start means an unenrolled machine publishes no
    [metrics](../observability/metrics.md), so the fleet's own monitoring cannot see it waiting.
    Whether the agent should instead run and report an unenrolled state is still open, against the
    risk that a misconfigured machine then looks like a working one to anything checking only
    whether the process is alive.
