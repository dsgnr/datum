---
description: "Build Datum and try one managed file on Linux. Validate desired state, preview a plan, apply changes, and inspect the reconciliation report."
seo_title: "Linux configuration management quickstart - Datum"
---

# Quickstart

Build Datum, inspect the example fleet, then try one managed file on Linux. You do not
need a Git remote or a running service for this walkthrough. To manage a host continuously,
follow [Install and run the agent](installation.md).

## Build and inspect the example

You need Git, Make and the Go toolchain specified in `go.mod`. Clone the source and build it:

```bash
git clone https://github.com/dsgnr/datum.git
cd datum
make build

./bin/datum validate --repo examples/fleet
./bin/datum render --host web-001 --repo examples/fleet
./bin/datum explain 'File[nginx-config]' --host web-001 --repo examples/fleet
```

`validate` checks the repository, `render` shows the resources selected for `web-001`,
and `explain` shows where one resource's values came from. All three read repository
content only and work on a development machine without changing its configuration.

The example fleet manages packages, services and kernel settings. For a first
reconciliation, use the single file below.

## Open a Linux shell

Applying changes requires Linux. On Linux, continue with the binary you just built:

```bash
export PATH="$PWD/bin:$PATH"
```

On macOS or another development platform with Docker, open a disposable Linux container:

```bash
make shell
```

The container already has `datum` on its path. Run the rest of this walkthrough in that
shell. It manages only `/tmp/datum-hello`, with reports in a temporary state directory.

## Describe one file

Create a separate fleet in a temporary directory. `datum init` creates the fleet root
and a base layer, which applies to every host you declare.

```bash
trial_dir=$(mktemp -d)
datum init --repo "$trial_dir" --name quickstart
mkdir -p "$trial_dir/fleet/hosts"

cat > "$trial_dir/fleet/hosts/web-001.yaml" <<'YAML'
datum: v1alpha1
type: Host
name: web-001
YAML

cat > "$trial_dir/fleet/base/hello.yaml" <<'YAML'
datum: v1alpha1
type: File
name: hello
desired:
  path: /tmp/datum-hello
  mode: "0644"
  content: |
    Hello from Datum.
YAML
```

`init` warns that the directory is outside a Git work tree. Local commands can read it
as it is; a continuously running agent needs a committed repository and configured source.

## Preview, apply and check

```bash
datum validate --repo "$trial_dir/fleet"
datum plan --host web-001 --repo "$trial_dir/fleet"
```

The plan should propose creating `File[hello]`. If `/tmp/datum-hello` already exists,
inspect the proposed update before continuing. `plan` exits 2 when it finds drift;
that is an expected result, not a validation failure.

Create a private state directory and record an observation without applying changes:

```bash
mkdir -m 0700 "$trial_dir/state"
datum reconcile --host web-001 --repo "$trial_dir/fleet" --state "$trial_dir/state" --mode observe
datum status --state "$trial_dir/state"
```

An observe pass with drift also exits 2. When the plan matches what you want, apply it:

```bash
datum reconcile --host web-001 --repo "$trial_dir/fleet" --state "$trial_dir/state" --mode enforce
cat /tmp/datum-hello
datum status --state "$trial_dir/state"
datum plan --host web-001 --repo "$trial_dir/fleet"
```

The file now contains `Hello from Datum.`, status reports the pass result, and the final
plan should find no changes and exit 0. Editing the file and planning again demonstrates
drift; another enforce pass restores its declared content.

These commands read a local directory directly and do not fetch or verify Git signatures.
They need permission to write the target and state directory; this temporary-file example
can run as your own user on Linux.

## Continue with your own host

[Install and run the agent](installation.md) covers packages, trusted signers, configuration,
and the systemd service. Start it in observe mode and review drift before enabling enforcement.

For more resources, use the [resource type reference](../resources/types/index.md).
For multiple hosts, read [fleet composition](../fleet/index.md). The
[CLI reference](../reference/cli.md) lists flags and exit codes.
