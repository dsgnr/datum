# Components

Each component below is described by its inputs, its outputs and the boundary it
operates within. The boundaries are part of the specification, since a component
that takes on work belonging to another one changes the behaviour of the pass.

## Repository

The repository is the source of desired state and is not a Datum component. It is
a Git repository containing `Host` documents, `Layer` documents, resource
documents and any files those resources reference.

Datum reads the repository at a specific revision and treats the content as
authoritative. Writes are outside Datum's scope, so hosts record no state back
into the repository and there is no command that converts an existing machine
into configuration.

## Fleet resolver

**Takes** a repository at a revision, and the name of one host.
**Produces** an effective manifest for that host.

The resolver reads the `Host` document for the named host, evaluates every
`Layer` matcher against that host's labels, orders the matching layers by
precedence, and merges them. It records for each resulting resource which layer
contributed it and which labels caused the layer to match.

Resolution uses the labels declared in the repository and requires no access to
the host, so a manifest can be rendered for any host from a checkout. Provider
selection is left to the observer and the reconciler, which run on the machine.

The manifest describes desired state alone. A resource already satisfied on the
host still appears in it.

## Effective manifest

The effective manifest is an artefact, not a component. It is the complete,
resolved desired state for one host at one revision, carrying the provenance of
every resource and identified by a digest of its content.

## Graph builder

**Takes** an effective manifest.
**Produces** a validated resource graph.

The graph builder turns `requires` references into edges, checks that every
reference resolves to a resource present in the manifest, detects cycles, and
detects two resources claiming the same target identity. Any of those failures
aborts the pass.

Graph validation runs before observation, so a manifest with a dependency cycle
or a duplicate target is rejected before the host is read.

The graph expresses a partial order over resources. Turning it into the total
order a plan needs is the planner's work.

## Observer

**Takes** a resource graph, and access to the host.
**Produces** observed state for every resource in it.

For each resource the observer calls the provider that serves its type and
collects the reported state of the target, covering existence, the fields the
resource type defines, and any fields the provider reports as unobservable.

Observation makes no change to the host. Planning, verification and reporting
drift without correcting it all depend on this, which makes it the strongest
constraint in the architecture.

Comparison against desired state happens in the planner. The observer reports
what it found, so an absent file is reported as absent.

Observation is limited to the targets named in the manifest.

## Planner

**Takes** desired state, observed state, and the resource graph.
**Produces** a plan.

The planner compares desired against observed state field by field, selects one
action per resource, and orders the actions so that every dependency edge is
respected. An unchanged resource is recorded with the action `none` and stays in
the plan.

Planning modifies nothing on the host. The planner queries providers for which
fields are observable and which provider serves a resource, and both are
recorded in the plan.

Whether a plan is applied is determined outside the planner, so the same planner
serves a preview and a real pass.

## Reconciler

**Takes** a plan, and access to the host.
**Produces** a result for each action and an outcome for the pass.

The reconciler executes the actions in plan order, passing each to the provider
that serves the resource, records the result, then re-reads the affected
resources through the observer to confirm the intended state was reached. When
an action fails, the resources depending on it are marked skipped and not
attempted.

Every action executed comes from the plan. Unexpected provider results are
recorded and reported, and the reconciler performs no action the plan does not
contain.

There is no rollback. A failed pass leaves the host in the partially changed
state it reached.

## Providers

**Take** a resource and a requested operation.
**Produce** observed state, or the result of a change.

A provider implements one resource type on one class of system. Tool-specific
behaviour such as `apt`, `systemd` or atomic file replacement is contained here,
and the provider translates a field-level description into the calls that read
or achieve it.

Providers act on the action they are given. A provider asked to update a file
updates it, since the comparison that selected the action has already happened
in the planner.

A provider receives one resource and has no access to fleet configuration,
matchers, labels or the rest of the manifest.

A request a provider cannot express is reported as an error and never
approximated.

## Linux

The operating system is the thing being changed, and providers are the only
components that interact with it. Code above the provider boundary that needs to
run a command or read a system path indicates a missing resource type or
provider interface.
