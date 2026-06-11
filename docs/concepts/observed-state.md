# Observed state

Observed state is what reading the host reports for the resources in desired
state, at a point in time. Observed state is measured on every pass.

## Observation is scoped to desired state

Datum reads the host resource by resource. Observation is limited to the targets
named in the effective manifest and does not inventory the machine, enumerate
installed packages or walk the filesystem.

For each resource in the effective manifest, the provider responsible for that
resource type is asked to report the current state of the target it names. A
`File` resource for `/etc/nginx/nginx.conf` causes that one path to be read.
Nothing else in `/etc/nginx` is read, since nothing else is declared.

The cost of observation therefore scales with the size of the manifest and not
with the size of the machine, which matters on hosts with tens of thousands of
packages and a manifest of forty resources.

## Observation is read-only

An observer never changes the host. This is a hard rule, not a convention
providers are encouraged to follow.

The reason is that every useful property of planning depends on it. A plan can
only be trusted before it is applied if producing the plan changed nothing, and
verification only means anything if reading the state after a change does not
itself alter the state. A provider whose only way to determine the installed
version of a package is to run a command that also refreshes package metadata has
a design problem to solve, and hiding it inside observation is not the solution.

## What a provider reports

Each resource type defines the fields that make up its state, and observation
returns values for those fields plus whether the target exists at all.

```text
File[nginx-config]
  exists   true
  path     /etc/nginx/nginx.conf
  owner    root
  group    root
  mode     0644
  content  sha256:91c4de2a
```

Reporting a content digest rather than content itself keeps observation cheap on
large files and keeps the comparison exact. Rendering a human-readable
difference is a separate concern, performed when a plan is displayed and not
when state is measured.

## Fields that cannot be observed

Some state is not readable on some systems, and pretending otherwise would put a
guess into the middle of the comparison.

A provider declares which fields of a resource type it can observe. A field it
cannot observe is reported as unknown, and a field reported as unknown cannot be
diffed, which means it cannot be planned for or verified.

!!! note "Proposed behaviour"

    The intended handling is that a resource with unknown fields appears in the
    plan with those fields marked, and that a resource whose identifying state
    cannot be observed at all is an error instead of something to attempt. This
    keeps the failure visible at plan time instead of at apply time. The exact
    reporting has not been designed.

## Observation is a snapshot

Between observing a host and applying a change to it, the host can change. Datum
holds no lock on the machine, so another process can edit a file between
observation and apply.

This race is accepted, not solved, because solving it would require excluding
everything else on the system from touching anything Datum manages. Verification
exists partly for this reason, since reading the affected resources again after
applying turns an assumption about the outcome into an observation of it. A plan
built from a stale snapshot that no longer matches reality produces a
verification failure and a non-empty plan on the following pass, which is
recoverable.

Observed state is not cached between passes. Every pass measures the host again,
because a cached measurement is a record of the past presented as the present.
