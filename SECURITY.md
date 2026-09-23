# Security

Datum runs as root and acts on paths and names that come from a Git repository, so a defect in
it is a defect in the privilege boundary of every host running it. Reports are welcome.

## Reporting a vulnerability

Use [GitHub's private vulnerability reporting](https://github.com/dsgnr/datum/security/advisories/new)
for anything that looks exploitable. That opens a private advisory rather than a public issue.

Do not open a public issue for a vulnerability first. Anything else, including a hardening
suggestion or a question about the threat model, belongs in a normal issue.

A report is more useful with the version from `datum version`, the distribution and its version,
and the smallest repository content that shows the problem.

## What is in scope

The [threat model](https://getdatum.sh/security/threat-model/) names the principals the
design defends against and the controls that apply to each. Anything that defeats one of those
controls is in scope, and so is anything that reaches root by a route the model does not mention.

The controls most worth attacking are [trusting desired
state](https://getdatum.sh/security/repository-trust/), which covers signature
verification and downgrade protection, [obtaining the
repository](https://getdatum.sh/security/repository-fetch/), which covers the hardened
checkout, and [applying state
safely](https://getdatum.sh/security/provider-safety/), which covers path handling as
root.

## What is out of scope

A repository that is trusted to configure a host can configure it. Writing a systemd unit, a
sudoers drop-in or root's authorized_keys grants root, and Datum permits all of it, because that
is what a configuration system does. The control there is review of the diff, not a restriction
in the tool. This is stated in full under [trust
anchors](https://getdatum.sh/security/repository-trust/#trust-anchors-are-never-managed-by-datum).

A vulnerability in the Git implementation is a vulnerability in Datum, and signature verification
does not help, since parsing happens first. Report it upstream as well.

## Known gaps

Several controls are specified and not implemented, so they cannot be relied on in an alpha.
`trust.strictPaths` is read and does nothing, the provider path-safety rules are a specification
rather than code, and secret resolution does not exist, so a `secretRef` fails rather than
resolving. [Project status](https://getdatum.sh/introduction/project-status/) tracks
what is built.

A report that one of these is absent is useful as an issue rather than an advisory, since the
documentation already says so.

## Supported versions

There are no releases yet, so only `main` is supported. Once there are releases, this section
will say which of them receive fixes.
