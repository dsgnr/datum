# Drift

Drift is a difference between desired state and observed state for a resource in
the effective manifest. Drift is handled through the normal reconciliation
process, and the code path that installs a package for the first time is the one
that corrects a package removed by hand.

## Drift is per field

Drift is recorded at the level of individual fields, not whole resources.

```text
File[nginx-config]
  mode     0644 -> 0600        drift
  owner    root                match
  content  sha256:91c4de2a     match
```

Field-level detail lets a plan state which fields change and lets a provider
make a narrow change. Correcting a mode does not rewrite content, which matters
for files that other services watch.

## Drift no provider will correct

Almost all drift is corrected on the pass that finds it. The exception is a
[uid](../resources/types/user.md#identifiers) or
[gid](../resources/types/group.md#identifiers) that does not match what the
manifest declares. Changing either would leave every file owned by the old
number belonging to nobody, and the manifest does not say which files to
reassign.

A provider declares such a field uncorrectable. The difference is compared and reported in the diff
and in the plan without being acted on, and the resource ends the pass as `drifted`. The plan marks
the field on its own line so that an uncorrectable difference is distinguishable from a failed
change.

```text
User[deploy]
  uid  4242 -> 4343  (reported, not corrected)
```

Reporting the difference keeps the mismatch visible while allowing the rest of the pass to converge.

## The repository is a source of drift

Drift does not only mean the host changed. A commit that alters a resource
produces drift on every host the resource applies to, without anything happening
on those hosts at all.

Both directions are the same measurement and Datum does not distinguish them.
Comparing desired state against observed state detects a changed file on the
host and a changed file in the repository in the same way, without
notifications, triggers or a record of the previous pass.

A host that has been powered off for a month needs no catch-up mechanism. Its
first pass produces the drift between its current state and the current
revision, and it converges from there.

## Where drift comes from

Manual changes during incidents are the obvious case. The cases that are harder
to diagnose are the ones nobody performed.

Package upgrades replace configuration files, sometimes prompting and sometimes
silently depending on the package manager and how the file was modified. Runtime
state set outside a persistence mechanism is lost at reboot, which is why a
kernel parameter applied with `sysctl -w` and never written to a file reads as
drift after a restart. Other tooling on the same machine can manage the same
files as Datum, which produces drift that reappears on every pass and is a
configuration problem rather than a defect.

That last case needs detecting explicitly. A resource that drifts, is corrected
and drifts again on the following pass indicates that something else manages the
same target, and it should be reported as a conflict.

!!! note "Open question"

    Detecting and reporting repeated correction of the same resource clearly
    needs doing and has not been designed. It requires keeping some history
    across passes, which sits awkwardly with the decision that Datum holds no
    record of what it previously applied. The distinction is probably that such
    history is diagnostic output, not an input to the comparison, but that has
    not been worked through.

## Drift Datum cannot see

Drift is detectable only within the manifest and only for fields a provider can
observe. An undeclared configuration file has no resource describing it, and a
field a provider reports as unknown cannot be compared.

This is a consequence of Datum managing what is declared instead of owning whole
machines. The coverage of a repository is therefore a real property to know
about, and a host where forty resources are managed and four hundred files
matter is a host where drift reporting means less than it appears to.

## Reporting drift without correcting it

Detecting drift and acting on it are separate operations, and the separation is
not an optional extra.

A machine in the middle of an incident is a machine where automatic correction may
be the wrong thing to do, and the tool needs a way to say what is wrong without
changing anything. Diffing produces that answer, and because it uses the same
observation and comparison as a full pass, the answer is not an approximation of
what reconciliation would find.

An agent can be run this way permanently and not just as a one-off command. A
host in [`observe` mode](reconciliation-modes.md) reports drift on every pass
and applies nothing, which turns Datum into a fleet drift detector before it is
trusted to make changes.
