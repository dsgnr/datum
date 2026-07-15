# Applications

Installing nginx does not give a working web server, and running `apt-get install postgresql` does
not give a usable database. Software frequently needs initialisation that no `Package`, `File` or
`Service` resource describes, and this page decides how Datum handles that.

## An application is not a Datum concept

There is no `Application` type and no grouping construct. nginx on a host is
`Package[nginx]`, `File[nginx-config]` and `Service[nginx]`, three independent resources with
declared dependencies between them.

Keeping it that way means composition, provenance, conflict detection and planning all work on one
kind of thing. A grouping construct would need its own merge rules, its own precedence behaviour and
its own conflict semantics, and every one of those is already solved for resources.

Higher-level types can still exist, as [extensions](#extensions), and a `PostgresDatabase` type is a
reasonable thing for somebody to write. What it is not is a group of primitives with a name. It is a
resource with its own target identity, its own observation and its own provider.

## The post-installation problem

A package that needs a setup step before it is usable presents a problem the resource model does not
obviously solve.

```text
install postgresql        Package
write postgresql.conf     File
initialise the cluster     ?
create a schema            ?
run a migration            ?
```

The missing rows have three properties in common. They are commands, not states, they are often safe
exactly once, and whether they have already happened is knowable but not from any field a `Package`
or `File` describes.

## What Datum does not do

There is no resource that runs a command, and no field anywhere in desired state that causes Datum
to execute something the repository supplied. That is recorded as
[ADR-0011](../adr/0011-no-command-execution-from-desired-state.md), and it applies to validators and
initialisers as much as to a hypothetical `Exec` type, because a command in a validator field is
arbitrary root execution just as surely as one in a resource.

The reasoning needs restating, because the cost is real. A resource that runs a command has nothing
to observe before it runs and nothing to verify afterwards beyond an exit status, so its presence in
a manifest makes drift detection, planning and verification conditional on whether that manifest
used it. One escape hatch in one repository weakens every claim the design makes about every host
that repository manages.

## What Datum does instead

Initialisation is modelled as a resource type, implemented by an extension, whose state is genuinely
observable.

```yaml
datum: v1alpha1
type: PostgresCluster

name: main

requires:
  - Package[postgresql]

desired:
  state: initialised
  dataDirectory: /var/lib/postgresql/16/main
  encoding: UTF8
```

The provider for that type knows how to observe whether the cluster is initialised, which for
PostgreSQL means checking for a `PG_VERSION` file and a valid control file rather than remembering
whether Datum once ran `initdb`. It knows how to initialise it, and it knows how to verify the
result.

That is the whole pattern. The thing that makes initialisation expressible as a resource is not a
command, it is an observation. Anything whose completion can be read back from the host can be a
resource type, and anything whose completion cannot be read back is not something Datum can
reconcile no matter what syntax is offered.

| Question | Answer |
| -------- | ------ |
| A command safe exactly once | A resource whose observation reports whether it has happened |
| A command safe repeatedly | Still a resource, and repeated safety is not what makes it one |
| Schema creation | Observable through the schema's own catalogue |
| A migration | Observable through a migration table |
| Activation | Observable through whatever records the activation |
| Something with no observable outcome | Out of scope, and no syntax changes that |

The last row is the honest one. Software that leaves no trace of having been initialised cannot be
managed declaratively by anything, and Datum says so instead of offering a way to pretend.

## Operations that are not safe to repeat

An operation that must happen exactly once, and that damages something if it happens twice, is
representable only where its completion can be read back from the host. Where that reading is possible
the operation stops being non-idempotent as far as Datum is concerned, because the provider observes
that it has already happened and plans no action.

```text
initialise a cluster        PG_VERSION exists           observable, so expressible
apply a schema migration    the migration table row     observable, so expressible
send a notification         nothing on the host         not observable, out of scope
consume a one-time token    nothing on the host         not observable, out of scope
```

The two lower rows are out of scope and no syntax makes them otherwise. A resource whose completion
leaves no trace has nothing for the next pass to observe, so every pass would attempt it again, and
the only way to prevent that would be for Datum to record that it once acted. Recording that makes
Datum the authority on what happened instead of the host, which is the assumption [observed
state](../concepts/observed-state.md) exists to avoid and the reason drift detection works at all.

The consequence is that Datum is not an orchestration tool. A one-off sequence with no observable
end state is a job for whatever runs jobs, and attempting it here would mean the first resource
whose state Datum has to remember rather than read.

## Partial initialisation

An initialisation that fails halfway is the case that makes command-based tools unsafe, and the
resource model handles it without special machinery.

The resource is observed on the next pass. If the provider reports the cluster as not initialised,
the action is attempted again. If it reports a state that is neither initialised nor absent, the
provider reports that explicitly and the resource is `failed` and not retried into a worse
condition.

Recovering from a genuinely broken half-initialised state is the provider's problem and frequently
is not solvable automatically, which the provider says instead of guessing at. A provider that
cannot distinguish "not yet initialised" from "initialised and broken" is a provider that should not
offer the type.

## Extensions

An extension supplies provider implementations for resource types Datum does not ship.

!!! note "Proposed design"

    The extension mechanism is proposed in outline. It is load-bearing, because
    [ADR-0011](../adr/0011-no-command-execution-from-desired-state.md) makes it the only path for
    anything not already modelled, and the detail below is the current position, not a
    specification.

Extensions run out of process, as a subprocess the agent speaks to over a defined protocol.

In-process plugins would be faster and would let a bad extension take the reconciler down with it,
or corrupt its state, while running with the same root privileges. Out of process means a provider
that crashes fails one resource instead of one pass, and it makes the provider interface a real
boundary with a versionable protocol across it, not a set of function signatures.

An extension runs as root, because changing a host requires it. There is no sandbox and no privilege
reduction, so an extension is trusted code and installing one is a privileged operation.

Installing an extension is therefore not done through desired state. A repository cannot cause code
to be installed and then cause that code to run, for the same reason it cannot manage its own
[trust anchors](../adr/0010-no-self-managed-trust-anchors.md), and an extension arrives the way the
agent itself arrives.

!!! note "Open question"

    How extensions are built, distributed, signed, installed, discovered and updated is undecided,
    and it is the largest gap this decision creates. It interacts with the
    [agent supply chain](../architecture/self-management.md) question, because both are about
    getting trusted code onto a host outside the mechanism Datum uses for everything else.

## Higher-level and primitive types together

A `PostgresCluster` extension and a `File` resource can both end up managing
`/var/lib/postgresql/16/main/postgresql.conf`, which is a conflict.

It is detected by the mechanism already proposed for this, where a provider declares the host paths
it owns and the graph builder checks declared ownership across the whole manifest. A higher-level
type declaring the paths it manages means an overlapping `File` resource is rejected during
[manifest validation](../reference/manifest-format.md#validation-summary) instead of discovered as
two resources fighting on every pass.

That makes the ownership question answerable in both directions. A repository can use the
higher-level type and let it own the file, or use primitives and not install the higher-level type,
and declaring both is an error, not a race.

## Reload against restart

`restartOn` restarts a service when something it depends on changes. Many applications reload
configuration without dropping connections, and restarting them unnecessarily is a self-inflicted
outage.

```yaml
datum: v1alpha1
type: Service

name: nginx

requires:
  - Package[nginx]
reloadOn:
  - File[nginx-config]

desired:
  state: running
  enabled: true
```

`reloadOn` behaves exactly as [`restartOn`](dependencies.md#restarton) does, ordering the referenced
resources before the service and updating the service when any of them changes, and it asks the
provider for a reload in place of a restart.

A provider whose unit does not support reloading reports that and fails the action. It does not
quietly restart instead, because a caller who asked for a reload was avoiding a restart, and
substituting one for the other converts a stated requirement into an outage.

Declaring both `reloadOn` and `restartOn` for the same resource is an error. The two express
different intents for the same event, and picking one silently would be exactly the kind of
resolution the design refuses elsewhere.

This resolves what was previously an open question against `Service`.

## Several changes, one reload

A service with three files in `reloadOn`, all of which changed in the same pass, reloads once.

That falls out of the plan having [one action per resource](../concepts/plan.md#actions) and not one
action per trigger. The three files each get their own `update`, the service gets a single `update`
whose reason names every trigger that fired, and the ordering guarantees every file is written
before the reload happens.

```text
update   File[nginx-main]
update   File[nginx-tls]
update   File[nginx-upstream]

update   Service[nginx]
         reason   3 resources changed, reloadOn matched
```

No accumulation mechanism is needed, because the plan was never a queue of triggers.

## Configuration that changes shape between versions

An application whose configuration format changes between major versions is a case Datum does not
solve and should not pretend to.

A repository managing both versions during a migration has two different configurations to express,
and the mechanism for that is the existing one, a layer whose matcher narrows to the hosts running
each version with the version as a declared label. That is the same approach [multi-distribution
differences](../providers/multi-distribution.md#handling-a-genuine-difference) use, and it has the
same cost, which is that the difference is visible and has to be written out.

What Datum will not do is transform one format into the other. That is a migration, it is
application-specific, and it belongs in whatever performs the upgrade rather than in a system whose
job is to make a host match a description.
