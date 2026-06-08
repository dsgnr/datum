# Multi-distribution design

Datum aims to manage several Linux distributions from one repository. The way it
intends to stay distribution independent is by separating two things that are easy to
conflate.

Resource semantics
:   What a resource type means. `Package` with `state: present` means the package is
    installed. This is the same statement on every system and is defined once.

Provider implementation
:   How that meaning is achieved and observed on a particular system. Four different
    package managers, four different command sets, four different version string
    formats.

```text
Package resource
      │
      ├── apt
      ├── dnf
      ├── apk
      └── pacman
```

The distributions the design targets are Debian, Ubuntu, Fedora, RHEL, Rocky Linux,
AlmaLinux, Alpine Linux and Arch Linux. Targeting them is not the same as supporting
them, and the [support matrix](support-matrix.md) records what is actually implemented,
which is currently nothing.

## Distributions do not behave identically

The honest position is that a single resource field cannot always mean the same thing
everywhere, and the design would rather say so than paper over it.

**Version strings have no common grammar.** A Debian version such as `1.24.0-2` and a
Fedora version such as `1.24.0-1.fc39` describe comparable software and share no
format. There is no translation, so a layer pinning a version is normally a layer whose
matcher narrows to one distribution.

**Holding a package at a version differs on all four.** The mechanism is not the same
on any two of them, and on some it needs an additional plugin installed before it
exists at all. A uniform `hold` field would be a field that works differently, or not
at all, depending on the host.

**Default dependency behaviour differs.** Some package managers install recommended or
weakly-required packages by default and others do not, so the same `state: present`
produces different sets of installed packages on different distributions. Datum
describes the package that was asked for, and what else arrives with it is the
distribution's decision.

**Package names differ for the same software.** The web server is `apache2` on Debian
and `httpd` on Fedora. Nothing in the current model absorbs that, which is the open
question recorded against resource identity.

**Init systems are not universal.** Alpine uses OpenRC rather than systemd, so
the single proposed `Service` provider does not cover a distribution in the
target list. That is an unresolved tension, not a solved problem.

**File layout conventions differ.** Configuration lives in different places, and a
`File` resource names an absolute path. A layer describing a configuration file is
therefore often distribution-specific even though the `File` type is not.

## Where the difference is allowed to surface

Differences appear in exactly two places, and the choice of where matters.

The first is the provider, where a difference is invisible to the repository. `apt` and
`dnf` needing different commands to install a package is entirely absorbed, and no
repository ever mentions it.

The second is the repository, where a difference is visible and has to be written out.
A package named differently on two distributions means two layers with narrower
matchers, and that is a cost paid by whoever maintains the fleet.

A third option is not allowed, where a field exists on a resource type and
behaves differently depending on the host without saying so. Configuration
written against such a field is reviewed once and then applied to hosts it does
not describe.

## Handling a genuine difference

When a resource needs to differ per distribution, the mechanism is a layer with a
narrower matcher, which requires the hosts to be classified.

```yaml title="fleet/hosts/web-001/host.yaml"
datum: v1alpha1
type: Host

name: web-001
labels:
  environment: production
  role: web
  os: debian
```

```yaml title="fleet/roles/web-debian/layer.yaml"
datum: v1alpha1
type: Layer

name: role-web-debian

precedence: 35
match:
  labels:
    role: web
    os: debian
```

The `os` label is an ordinary declared label with no special meaning to Datum. Using it
this way is a repository's choice, and the effect is that the distribution-specific part
is confined to one layer that says which hosts it is for.

!!! note "Open question"

    Requiring an `os` label to be declared by hand duplicates something the host
    already knows, and getting it wrong means a host receives configuration for a
    distribution it is not running. Letting matchers match observed facts would remove
    the duplication and would also mean desired state could no longer be resolved
    without reaching the machine, which is a property the fleet model currently
    depends on. This is the most consequential unresolved question in the design.

## Testing across distributions

A design that claims multi-distribution support has to be tested that way, and the
number of combinations grows quickly. Seven resource types against four package
managers and eight distributions is not a matrix anybody fills in by hand.

!!! note "Planned"

    Provider behaviour will need testing against real distribution images
    instead of mocks, because the differences that matter are in command output
    formats and failure modes, which are exactly what a mock gets wrong. How
    that is arranged has not been decided, and it is a prerequisite for any
    entry in the support matrix, not something to add afterwards.
