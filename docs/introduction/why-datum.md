# Why Datum?

Configuration management is neither new nor unsolved. Datum exists because a particular
combination of properties is hard to find in one tool.

## Machines diverge from what is believed about them

A machine is correct on the day it is built. Someone edits a file during an incident, a
package upgrade replaces a configuration file with the distribution default, a kernel
parameter set at the command line is lost at the next reboot. None of it is recorded, so
the gap between what is believed about a machine and what is true of it grows quietly.

Datum reads the host on every pass and reports what differs, so drift is the
output of routine work rather than the cause of an outage.

## Scripts describe steps, not state

A script that installs a package and edits a file is quick to write. It is correct only on
a machine in the state its author had in mind. Running it twice may not be safe, running it
against a partially converged machine usually is not, and reading it tells you what the
author intended to do, not what the machine should look like.

Describing state moves that burden into the tool. Datum has to work out what to do, which
is harder to implement and keeps the description valid whatever state the machine starts
in.

## Tracing why a value reached a host

On a fleet of any size, configuration arrives at a host from several places at
once, and the mechanisms tend to be inheritance hierarchies, group variables,
include files and conditionals. Tracing why a particular value ended up on a
particular machine becomes an exercise in reading the whole repository.

That question is treated here as a first-class requirement. Because every
resource in an effective manifest retains the layer it came from and the selector
that matched, Datum can answer directly why a resource applies to a host, rather
than leaving an engineer to reconstruct it.

## Distribution differences spread

The first time a repository manages both Debian and Rocky Linux, a conditional appears. The
conditionals multiply, and the distribution check ends up in the configuration, the
templates and the variable precedence rules. The resource model takes the shape of
whichever package manager was implemented first.

Resource semantics in Datum are distribution neutral. Every distribution-specific decision
belongs inside a provider, the only part permitted to know what `apt` is.

## Knowing what will change is separate from changing it

Applying configuration to a production machine without knowing what will change is usually mitigated
with review and caution, not tooling. Where a dry run exists, it is often an approximation of the
real run and not the same code path.

Datum builds a plan as an ordinary step in the cycle, so it is the same artefact whether or not it
gets applied. The same separation gives verification somewhere to live, since after applying, Datum
reads the affected resources again and reports whether the intended state was reached.

## Where this leaves things

None of these properties is novel. Declarative state, reconciliation loops, typed resources
and provider abstraction all exist in tools that predate this one, and several influenced
the design directly. The aim is a system that holds all of them at once for Linux hosts,
keeps the resource model small enough to reason about, and can explain its own decisions.
