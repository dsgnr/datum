# Open questions

Every unresolved question recorded on this site, collected in one place. Each entry links to
the section that states it in full.

The list exists because open questions scattered across forty pages are easy to lose, and
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
:   Reacting to change is specific to `Service`. Whether a general mechanism is needed, and what
    it would mean for a type whose reaction is not a restart, is unresolved. There is also no way
    to request a reload rather than a restart.

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

## Providers

[Provider selection overrides](../providers/selection.md#overriding-selection)
:   There is no way to force a provider. The case for one is real, and an override is per-host
    configuration describing an implementation detail, which tends to spread once it exists.

[Skipped resources and pass outcome](../providers/selection.md#when-no-provider-matches)
:   A resource with no available provider is skipped, which lets a host sit at "mostly converged"
    indefinitely. Whether the pass outcome should reflect skips is probably yes and is not decided.

[Alpine and OpenRC](../providers/multi-distribution.md)
:   Alpine is in the target distribution list and does not use systemd, so either an OpenRC
    provider is needed or Alpine support means images with no init system running. Unresolved.

## Reporting and interface

[Approved plans](../concepts/plan.md#a-plan-is-not-a-stored-artefact-to-replay)
:   Change control usually wants a reviewed plan to be the thing applied, which conflicts with
    rebuilding the plan at apply time. Applying a fresh plan only when it is equivalent to the
    approved one is a possible resolution and has not been designed.

[Exit code for skipped resources](../reference/cli.md#exit-codes)
:   Whether `datum reconcile` should exit non-zero when resources were skipped.

[Detecting repeated correction](../concepts/drift.md#where-drift-comes-from)
:   A resource corrected on consecutive passes suggests something else on the machine manages the
    same target. Detecting it needs history across passes, which sits awkwardly against Datum
    keeping no state it later depends on.

[Machine-readable plan format](../concepts/plan.md)
:   The contents of a plan are settled and the serialised form is not. Something structured is
    needed before anything can consume plans programmatically.

[Report retention](../architecture/reconciliation-flow.md)
:   Where pass reports are kept, for how long, and whether they are readable through
    `datum status` or only as files on the host.

## Security

[Commit signature verification](../security/threat-model.md#an-attacker-who-can-merge-to-the-repository)
:   Whether the agent should refuse a revision not signed by a trusted key. This is the only
    proposed control that constrains a repository writer, and the cost is that every commit
    reaching the tracked branch has to be signed by a key the fleet trusts, which constrains
    automation and makes key rotation a fleet-wide operation.

[Downgrade protection](../security/handshake.md#verifying-that-a-revision-is-current)
:   Requiring each revision to be a descendant of the last one applied would stop a signed old
    commit being replayed. The awkward cases are a host that has been off long enough for
    history to have been rewritten, and a repository that force-pushes, both of which would
    need an override nobody has designed.

[Repository credential provisioning](../security/threat-model.md#an-attacker-who-can-read-the-repository)
:   How the credential each host uses to read Git is issued, scoped and rotated. Per-host,
    read-only, individually revocable credentials would limit the damage from stealing one, and
    none of that is designed.

[Placing a repository credential](../security/handshake.md#getting-the-repository-credential-onto-a-host)
:   The constraint is decided, being that whatever provisions a machine chooses its name and
    places its credential, and the machine asserts neither to anything. Which mechanisms ship,
    and what an attestation path looks like for each platform that offers one, is not.

[Filesystem race handling](../security/threat-model.md#an-attacker-with-an-unprivileged-account-on-a-managed-host)
:   The proposed handling for symlink substitution, hard links and directory swaps is specified
    in outline. Whether apply should operate on a directory descriptor captured during
    observation, which narrows the window between deciding and writing, has not been worked
    through.

[Report storage and permissions](../security/threat-model.md#an-attacker-with-an-unprivileged-account-on-a-managed-host)
:   Plans can contain file content, so a report written readable by other local users discloses
    it. Root ownership with mode `0600` and digests rather than text is the proposed default,
    and it interacts with the undecided question of where reports are kept.

[A Repository resource type](../security/threat-model.md#an-attacker-who-controls-upstream-content)
:   Adding a package repository through a `File` resource looks like an ordinary file change in
    review while granting root execution to whoever serves it. A dedicated type would make the
    intent reviewable, and it is also listed as the type most likely to be needed soonest.

[Agent supply chain](../security/index.md#what-is-not-defended)
:   How the agent binary is obtained, verified and updated is not designed. An agent updating
    itself through a resource describing its own package is a particular hazard, because a
    failed update leaves nothing running to retry it.

## Identity and delivery

[Hostname as an identity fallback](../architecture/host-identity.md#where-identity-comes-from)
:   Whether the system hostname should be usable when the identity file is absent. It would make
    first boot easier and would reintroduce the failure mode the explicit file exists to avoid.

[Concurrency within a pass](../architecture/dependency-graph.md#concurrency)
:   Whether unrelated actions should be applied concurrently, given that package manager locks
    would serialise the most expensive actions anyway.

## Not an open question

Secret material is a gap, not an open question, and the distinction is deliberate. Configuration
files need credentials and Datum has no mechanism for them, but there is no half-designed answer
waiting for a decision. Every host reads the whole repository, so a secret committed there is
readable by every managed machine, which means a mechanism cannot be added without changing how
desired state reaches a host.
