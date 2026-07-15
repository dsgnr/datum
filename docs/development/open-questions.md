# Open questions

Every unresolved question recorded on this site, collected in one place. Each entry links to
the section that states it in full.

The list exists because open questions scattered across ninety pages are easy to lose, and
because the count is a reasonable measure of how ready the design is. Resolving one means
deciding, updating the page, and removing the entry here.

## Blocking implementation

These affect the shape of code that would be written first.

[Matchers and observed host facts](../providers/multi-distribution.md#handling-a-genuine-difference)
:   Whether a matcher may match facts read from the host, such as the distribution reported by
    `/etc/os-release`. Allowing it removes the need to declare an `os` label by hand, which
    duplicates something the machine already knows and can be wrong. Forbidding it keeps desired
    state resolvable without reaching the machine, which the fleet model currently depends on.
    This is the most consequential unresolved question in the design.

[Observation failure handling](../architecture/reconciliation-flow.md#where-failure-stops-the-pass)
:   Whether failing to observe one resource ends the pass or allows planning to continue for the
    rest. Continuing is more useful on a large manifest, and stopping is safer when the failure
    suggests something is wrong with the host.

[Planning around unobservable fields](../resources/lifecycle.md)
:   Whether a resource with a field no provider can observe should be planned at all. Applying a
    field that cannot then be verified is a change Datum cannot confirm, which conflicts with the
    requirement that a pass reports whether the result was verified.

[Reconciliation interval](../concepts/reconciliation.md#retry)
:   How often a pass runs, whether the interval is configurable per host, and whether a failed
    pass should shorten the wait before the next one.

## Resource model

[Package logical names](../resources/identity.md#target-identity-by-type)
:   Whether `Package` should gain a `desired.package` field so one reference can mean `apache2` on
    Debian and `httpd` on Fedora, and whether that difference belongs in the resource document or
    inside a provider.

[Dependency removal](../fleet/composition.md#the-requires-exception)
:   There is no way for a higher-precedence layer to remove a dependency declared lower down.
    Adding one would reintroduce the ability to silently drop an ordering constraint, which is
    what treating `requires` as a set exists to prevent.

[Removal ordering](../resources/types/group.md#removal)
:   `requires` means processed before, regardless of action, so removing a group and its users
    needs the reverse of the creation order written out. Whether the planner should invert edges
    for `remove` actions is undecided, and doing so would make plan order depend on the action.

[Generic change reaction](../resources/dependencies.md#restarton)
:   Reacting to change is specific to `Service`, which now has both `restartOn` and
    [`reloadOn`](../resources/applications.md#reload-against-restart). Whether a general mechanism is
    needed for a type whose reaction is neither is unresolved, and adding one prematurely risks a
    generic trigger system used to sequence arbitrary work.

[Warning on likely missing dependencies](../resources/dependencies.md#where-dependencies-come-from)
:   Whether Datum should warn about a `File` under a managed `Directory` with no edge between
    them, without acting on it. A warning would catch the most common mistake while keeping plan
    order fully determined by the repository.

[Directory parents](../resources/types/directory.md#contents-are-not-managed)
:   Whether `Directory` should create missing parents, and with what ownership and mode. Creating
    them silently produces directories nobody described.

[Purging drop-in directories](../resources/types/directory.md#purging-undeclared-contents)
:   A file left in `/etc/nginx/conf.d` after being removed from the repository keeps being loaded.
    This is the one place where declared-only ownership produces the wrong outcome, and the
    strongest argument for a bounded reclaim mechanism.

[Group membership for unmanaged users](../resources/types/user.md#group-membership)
:   Membership can only be managed for users Datum manages, and `groups` is exclusive, not additive. Whether an additive mode is needed, and whether it can exist without making the
    field's meaning depend on a flag, is undecided.

[Service health after restart](../resources/types/service.md#verification)
:   Whether verification should wait before deciding a restarted service is healthy. Checking
    immediately reports success for a service that dies a second later, and waiting introduces a
    timeout nobody can choose correctly.

[File templating and symlinks](../resources/types/file.md#open-questions)
:   Templating is absent, and rendering content from host facts would reintroduce a dependency on
    observed state during resolution. Symbolic links have no representation at all.

[Numeric owner and group ids](../resources/types/file.md#open-questions)
:   Whether `owner` and `group` should accept numeric ids alongside names. Names are clearer and
    depend on the user existing, which is an ordering problem the manifest can express, and ids
    avoid that problem while being harder to read.

[Package repository configuration](../resources/types/package.md#open-questions)
:   Installing a package outside a distribution's default repositories needs a repository
    definition. Expressing it means either a new resource type per packaging system, which breaks
    distribution neutrality, or a `File` resource writing a sources list, which pushes a
    distribution difference up into the fleet configuration. This is the type most likely to be
    needed soonest.

[Removing unused dependencies](../resources/types/package.md#open-questions)
:   Whether removing a package should also remove packages that become unused. The set that
    becomes removable depends on everything else installed and not on anything in the manifest, so
    the honest answer may be that Datum should not.

[Instanced service units](../resources/types/service.md#open-questions)
:   Units with instances, such as `getty@tty1`, are not addressed. The name would work as a
    target identity, and whether anything else about them needs modelling is unexplored.

[Passwords and authentication](../resources/types/user.md#open-questions)
:   `User` models no authentication at all. Password hashes are secret material, which Datum has
    no mechanism for, and authorised keys are a `File` resource with the same problem. This is
    the largest gap in the type.

[Locking an account without removing it](../resources/types/user.md#open-questions)
:   Whether `state` needs a `locked` value alongside `absent`, which is probably the more common
    operational need than deletion.

[Provider path ownership](../resources/conflicts.md#overlapping-types)
:   Providers writing paths as an implementation detail, such as the `Sysctl` provider writing
    into `/etc/sysctl.d`, can collide with `File` resources. Declared path ownership is proposed
    and unspecified, and it is complicated by ownership depending on which provider was selected.

## Fleet and composition

[Multiple fleets per repository](../fleet/repository-layout.md#discovery)
:   Whether a repository may contain more than one `Fleet` document, and what it would mean for a
    host to appear in two of them.

[Reserved observed labels](../fleet/labels-and-matchers.md#reserved-labels)
:   Related to the blocking question above. Only `datum/host` is injected, and whether
    observed facts should join it is unresolved.

[Provenance and the manifest digest](../fleet/effective-manifests.md#content-addressing)
:   Whether provenance is inside the digest or alongside it. Including it means a refactor that
    moves a resource between layers changes the digest without changing behaviour. Excluding it
    weakens the claim that the digest identifies what Datum was told to do.

[Reporting layers that match no host](../repository/index.md#keeping-a-repository-from-accumulating-unused-content)
:   Whether `datum validate` should report a layer no host in the fleet matches. It catches the most
    common matcher mistake and it is also legitimate during a rollout, where a layer is committed
    before the hosts it targets exist.

## Providers

[Provider selection overrides](../providers/selection.md#overriding-selection)
:   There is no way to force a provider. The case for one is real, and an override is per-host
    configuration describing an implementation detail, which tends to spread once it exists.

[Capability set overrides](../providers/capabilities.md)
:   The [override question](../providers/selection.md#overriding-selection) applies to a whole
    capability set as much as to one provider. Whether a host can be pinned to a named capability
    set, and where that lives, is undecided for the same reasons.

[Alpine and OpenRC](../providers/multi-distribution.md)
:   Alpine is in the target distribution list and does not use systemd, so either an OpenRC
    provider is needed or Alpine support means images with no init system running. Unresolved.

## Applications and extensions

[Extension distribution](../resources/applications.md#extensions)
:   How extensions are built, distributed, signed, installed, discovered and updated. This is the
    largest gap created by [ADR-0011](../adr/0011-no-command-execution-from-desired-state.md), because
    extensions are the only path for anything Datum does not already model, and it interacts with the
    [agent supply chain](../architecture/self-management.md) question since both are about getting
    trusted code onto a host outside the mechanism used for everything else.

[Application-level configuration validation](../resources/validation.md#configurations-spanning-several-files)
:   Validating a configuration spread across several files needs a way to express that those resources
    form one application configuration, which is the first genuine argument for a grouping concept the design has [avoided so far](../resources/applications.md#an-application-is-not-a-datum-concept).
    The form that bites soonest is a configuration that is a whole directory, such as a `conf.d`, where
    a stale fragment nobody declared changes the assembled result and is invisible to validation.

[Recording a pending reload](../resources/validation.md#configurations-spanning-several-files)
:   When multi-file validation fails after the files are written, the files match desired state so
    nothing shows as drift, while the service is still running its old configuration. The pending
    reload has to be recorded somewhere or the host reports converged while holding a configuration
    that would fail on restart. This has the same shape as
    [`awaiting-reboot`](../concepts/state.md#reboots) and neither is designed.

[Validator output in reports](../resources/validation.md#privileges-and-output)
:   How much validator output belongs in a report that may be collected centrally. Truncating loses
    the diagnostic, keeping it risks disclosure through a channel the
    [redaction rule](../security/provider-safety.md#reports-and-content-disclosure) otherwise closes,
    and the `sensitive` flag only covers files somebody remembered to mark.

## Testing

[Where the VM matrix runs](testing.md#keeping-the-matrix-affordable)
:   Hosted runners with nested virtualisation are slow and simple, dedicated hardware is fast and needs
    maintaining, and cloud instances per run cost money continuously. The choice decides whether a
    nightly full matrix is achievable or becomes weekly.

## Ownership and modes

[Authoritative set composition](../resources/ownership.md#authoritative-sets-and-composition)
:   Whether an authoritative set merges its members across layers, in defiance of list replacement,
    or is owned by exactly one layer. This is the central difficulty in the ownership model and the reason authoritative sets are proposed and not accepted.

[Per-resource reconciliation mode](../concepts/reconciliation-modes.md#per-resource-policy)
:   Whether individual resources can override a host's `enforce`/`observe` mode, and what a host's
    status means when some resources are enforced and others only observed.

[Reboot policy](../concepts/state.md#reboot-policy)
:   Where reboot policy lives and which options exist. Automatic reboots need a decision about what
    happens when several hosts reach one at the same time.

## Reporting and interface

[Approved plans](../concepts/plan.md#a-plan-is-not-a-stored-artefact-to-replay)
:   Change control usually wants a reviewed plan to be the thing applied, which conflicts with
    rebuilding the plan at apply time. Applying a fresh plan only when it is equivalent to the
    approved one is a possible resolution and has not been designed.

[Exit code for skipped resources](../reference/cli.md#exit-codes)
:   Whether `datum reconcile` should exit non-zero when resources were skipped. The
    [`degraded` host state](../concepts/state.md#host-state-across-passes) answers the reporting
    half, and the exit code is still undecided.

[Detecting repeated correction](../concepts/drift.md#where-drift-comes-from)
:   A resource corrected on consecutive passes suggests something else on the machine manages the
    same target. Detecting it needs history across passes, which sits awkwardly against Datum
    keeping no state it later depends on.

[Machine-readable plan format](../concepts/plan.md)
:   The contents of a plan are settled and the serialised form is not. Something structured is
    needed before anything can consume plans programmatically.

[Status JSON schema](../reference/status.md#machine-readable-output)
:   The [status model](../reference/status.md) fixes the fields, and the JSON schema that exposes
    them is not settled. It becomes a [contract](../reference/stability.md) as soon as anything
    parses it.

[Metrics endpoint port, TLS and authentication](../observability/metrics.md#binding-and-exposure)
:   The agent serves metrics over HTTP, and the default port is provisional and needs registering in
    the Prometheus port allocation list. Whether the endpoint supports TLS and authentication is
    undecided, since exporters conventionally have neither and this one runs inside a root process
    holding repository credentials.

[Fleet expected revision](../observability/metrics.md#answering-is-this-host-up-to-date)
:   Revision lag is computed from the maximum across reporting hosts, so a fleet where every host is
    equally behind reports no lag. Closing that needs something that reads the repository on the
    fleet's behalf, which Datum does not have.

[Tracing](../observability/metrics.md#tracing)
:   Whether Datum emits traces. A pass is short, local and single-process, so durations in metrics and
    logs answer most of it.

[Report retention](../reference/status.md)
:   Where pass reports are kept, for how long, and whether they are readable through
    `datum status` or only as files on the host.

[Cached-revision expiry](../reconciliation/last-known-good.md#how-long-a-cached-revision-stays-usable)
:   Whether a last-known-good revision expires when a host is offline too long, which is the same
    decision as [signature freshness](../security/time.md#consequences-for-offline-hosts) seen from
    the desired-state side.

## Security

Most of what was open here is now specified in [trusting desired
state](../security/repository-trust.md) and [applying state
safely](../security/provider-safety.md). What remains is below.

[Placing a repository credential](../security/handshake.md#getting-the-repository-credential-onto-a-host)
:   The constraint is decided, being that whatever provisions a machine chooses its name and
    places its credential, and the machine asserts neither to anything. Which mechanisms ship,
    and what an attestation path looks like for each platform that offers one, is not.

[Trust-on-first-use at provisioning](../security/repository-trust.md#first-contact)
:   A host with no recorded revision accepts whatever signed revision it sees first, so the
    baseline has to be written when the machine is built. What writes it, and how a fleet checks
    that it was written, has not been designed.

[Signer key rotation](../adr/0010-no-self-managed-trust-anchors.md)
:   Because Datum refuses to manage its own signer list, rotating a key is a provisioning task.
    On a large fleet that is worse than a commit would have been, and no better answer exists
    yet that does not let the control disable itself.

[A Repository resource type](../security/threat-model.md#an-attacker-who-controls-upstream-content)
:   Adding a package repository through a `File` resource looks like an ordinary file change in
    review while granting root execution to whoever serves it. A dedicated type carrying the
    signing key explicitly would make the intent reviewable. This is also the type most likely to
    be needed soonest for reasons unrelated to security.

[Agent supply chain](../security/index.md#what-is-not-defended)
:   How the agent binary is obtained, verified and updated is not designed. An agent updating
    itself through a resource describing its own package is a particular hazard, because a failed
    update leaves nothing running to retry it.

[Agent self-update mechanism](../architecture/self-management.md)
:   The [boundary](../architecture/self-management.md#what-the-design-commits-to-now) is decided,
    being that the running reconciler does not replace its own binary mid-pass. Which mechanism does,
    whether a supervisor, external package management or an init-ordered restart, is not.

[Trustworthy time on hosts](../security/time.md)
:   What an agent does when it cannot establish trustworthy time for a time-dependent check. Ordering
    checks need no clock, and expiry checks do, and the fallback for a host with a wrong clock is
    undecided.

[Independent evidence that a host is in the state it claims](../observability/alerting.md#what-these-alerts-cannot-detect)
:   A host that has stopped reconciling is now detectable through [staleness
    alerting](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch), which relies on
    metrics being absolute timestamps. What remains unsolved is a host that has been compromised and
    reports healthy, because every signal Datum emits is produced by the host about itself. Closing
    that needs something a host cannot forge, which is a different problem from monitoring.

## Identity and delivery

[Hostname as an identity fallback](../architecture/host-identity.md#where-identity-comes-from)
:   Whether the system hostname should be usable when the identity file is absent. It would make
    first boot easier and would reintroduce the failure mode the explicit file exists to avoid.

[Concurrency within a pass](../architecture/dependency-graph.md#concurrency)
:   Whether unrelated actions should be applied concurrently, given that package manager locks
    would serialise the most expensive actions anyway.

## Host lifecycle

[An agent with no identity](../lifecycle/installation.md#verifying-an-installation)
:   Whether the agent refuses to start when it has no identity or runs and reports an unenrolled
    state. Running makes the state observable through metrics before any enrolment has happened,
    and it also means a misconfigured machine looks alive to anything checking only the process.

[Discovering machines that never enrolled](../lifecycle/enrolment.md#machines-that-never-enrol)
:   Whether unenrolled machines should be discoverable. A fleet cannot distinguish a machine that
    was never meant to be managed from one whose enrolment failed, and closing that gap means
    something outside Datum holding a list of machines that are supposed to exist.

[Retaining per-host reports](../lifecycle/decommissioning.md#what-the-fleet-keeps-afterwards)
:   Where a host's own reporting is retained once the machine is gone. Git records what a host was
    told to do and nothing records what it reported doing, so an audit after decommissioning has
    the intent and not the outcome.

## Not an open question

Secret material is a gap, not an open question, and the distinction is deliberate. Configuration
files need credentials and Datum has no mechanism for them, but there is no half-designed answer
waiting for a decision. Every host reads the whole repository, so a secret committed there is
readable by every managed machine, which means a mechanism cannot be added without changing how
desired state reaches a host.

One constraint on any future secret mechanism is settled even though the mechanism is not. The
[effective manifest describes secret references, never resolved secret
values](../fleet/effective-manifests.md#secrets-and-the-digest), so that secrets stay out of the
manifest, its digest, logs, plans and provenance. That is a boundary a design has to respect, not a
design in itself.
