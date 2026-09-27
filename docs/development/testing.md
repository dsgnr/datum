# Testing

Datum changes operating systems as root. Its tests cover both the logic that decides what to change
and providers running against real system tools. This page describes the checks that exist today
and the coverage still to build. The [support matrix](../providers/support-matrix.md) records which
providers and distributions are supported.

## Running checks locally

From the repository root, with the Go version specified in `go.mod`:

```sh
make lint        # Go formatting, vet and tests
make test-build  # rebuild with two versions and check the binary reports each one
```

`make test` runs just `go test ./...`. For a focused change, run the affected package first, then
run the full suite before committing:

```sh
go test ./internal/metrics -count=1
go test ./... -count=1
```

The ordinary Go suite runs on Linux and macOS. Tests that apply Linux state skip on other operating
systems; a successful macOS run therefore does not establish Linux provider coverage. With Docker
running, `make test-linux` runs the ordinary suite in a Linux container.

For documentation changes, run `make docs-check`. It installs the Python documentation toolchain
in `.venv`, builds the site in strict mode, and uses a Node container to parse Mermaid diagrams.
See [working on the documentation](documentation.md) for preview and build instructions.

## Integration targets

These targets require Docker. Run the target for the provider or subsystem you changed. The
`integration` build tag enables tests that install packages, change accounts or modify system
state, so use the container targets instead of running the tagged suite on your workstation.

| Command | What it exercises |
| ------- | ----------------- |
| `make test-apt` | The apt provider against Debian's real package tools |
| `make test-dnf` | The dnf provider against Fedora 41's real package tools |
| `make test-user` | User and group operations against a real account database |
| `make test-sysctl` | Runtime kernel parameters and persistence files in a privileged container |
| `make test-systemd` | The systemd provider and agent service in a container booted with systemd as PID 1 |
| `make test-git` | Git operations and signature verification against real signed repositories |
| `make test-source` | Revision selection and fallback against a real signed repository |
| `make test-package` | Building and installing the Debian and RPM packages, then checking the installed files |
| `make test-prometheus` | Parsing the example Prometheus configuration and testing the alert rules with promtool |

The Debian suites use the `golang:1.25` image, and the dnf suite uses `fedora:41`. Package installation
checks use `debian:trixie` and `fedora:41`. These are the images selected by the Makefile, not a claim
that every Debian or Fedora release has been tested. Package suites need access to their
repositories to install dependencies.

### Container coverage and its limits

A container provides real package tools, account databases and filesystem operations. The systemd
suite additionally boots systemd as PID 1 with cgroup access; it is a privileged Docker container,
not an ordinary container running `systemctl` without a service manager.

The sysctl suite changes `net.ipv4.ip_forward` in the container's own network namespace. It checks
writes to procfs, invalid-value rejection and persistence-file contents. This provides useful
kernel interaction coverage, but does not establish that arbitrary sysctls are isolated from the
host or that persisted values survive a reboot. New sysctl tests must choose parameters with the
same care.

There is currently no VM test matrix. Host reboots, kernel upgrades, recovery after power loss and
network changes that can disconnect a host need separate coverage. A container test cannot establish
those behaviours.

## What CI runs

Pull requests and pushes to `main` run the same Go workflow:

- Formatting, `go vet` and the ordinary Go suite on Linux.
- The ordinary Go suite on macOS.
- The build-version regression test and cross-compilation for Linux `amd64` and `arm64`.
- Debian and RPM package installation checks and Prometheus rule tests.
- All seven integration targets from `test-apt` through `test-source` in the table above.

The `All checks` job collects the Go workflow results for branch protection. A separate
documentation workflow parses Mermaid diagrams and runs a clean, strict site build. Successful
pushes to `main` also deploy the documentation.

Cross-compiling for both architectures checks that the binaries can be built. It does not run the
provider suites on both architectures; integration tests use the runner's container architecture.

Tags starting with `v` trigger the release workflow. It reruns formatting, vet, the Go tests and all
seven integration suites, builds binaries and packages for both Linux architectures, installs and
checks packages for the runner's architecture, and checks the native binary's version. It then
signs checksums and publishes the release artefacts. There is no nightly workflow or additional VM
release gate.

The workflow files in `.github/workflows/` and the root `Makefile` are the executable definitions of
these checks. Update this page when their coverage changes.

## Writing useful regression tests

Start with the user-visible failure and make the test fail before changing the implementation.
Parsing and resolution tests should assert the resolved result or the error returned for bad input.
Filesystem tests should inspect the resulting files and check that unrelated paths remain untouched.
Provider integration tests should assert real observed state after applying a change.

For providers, cover convergence with a second pass, deliberately introduced drift, failures and
verification mismatches. Test unsupported operations as explicit refusals. Follow the
[provider safety rules](../security/provider-safety.md) when constructing hostile filesystem cases.

`internal/provider/providertest` supplies an in-memory provider for engine tests. It is not a shared
conformance suite against every real provider. A reusable conformance suite remains future work;
its useful requirements include observation without mutation, idempotence, truthful failure
reporting and explicit treatment of fields a provider cannot observe.

## Keeping the matrix affordable

Broader distribution, architecture and VM coverage is planned, not currently running. Add rows in
response to provider behaviour that needs testing, and keep fast parsing, resolution and engine
tests independent of that infrastructure. Use containers where real userspace tools are enough;
reserve VMs for behaviour that needs a host kernel, reboot or full machine lifecycle.

A future matrix should test named distribution versions and both supported architectures before
extending the [support claims](../providers/support-matrix.md). Nightly and release VM runs are
possible ways to control cost, but neither is implemented.

!!! note "Open question"

    Where the VM matrix runs is undecided. Hosted runners with nested virtualisation, dedicated
    hardware and disposable cloud instances have different cost and maintenance requirements. The
    choice will determine how frequently the broader matrix can run.
