# ADR-0013: Secret references, resolved on the host

Status: Accepted

## Context

Datum could not place a credential anywhere, and that was recorded as a gap, not an open question,
on the grounds that no half-designed answer was waiting for a decision.

The gap turned out to block more than it appeared to. `User` models no authentication, so a password
hash cannot be set. An `authorized_keys` file is a `File` resource whose content is key material.
A TLS private key, a database password, a registry credential and a monitoring token are all ordinary
requirements of the hosts this system exists to configure. The flagship worked example hardens `sshd`
and cannot provision the keys needed to log in afterwards.

A configuration system that cannot place a credential does not avoid the problem. The credentials get
placed by something else, and that something else becomes the real configuration system for the parts
that matter most.

The reason the gap persisted is that the obvious answer is wrong. Committing secret values to the
repository would make every secret in the fleet readable by every managed machine, because in
[the deployment model](../architecture/deployment-models.md) each host reads the whole repository. That is not
a limitation to be worked around with encryption at rest in Git, because the hosts would still all hold
the decryption key.

A secret mechanism does not require anything central. What it requires is that the secret value never
travels through the repository.

## Decision

Desired state declares a **reference** to a secret. A host-local secret provider resolves that
reference to a value during apply, and the value exists nowhere else.

```text
repository          holds a reference             app/db-password
effective manifest  holds the same reference      app/db-password
host, during apply  resolves it to a value        (never recorded)
```

The reference is resolved after the effective manifest has been built, which places it in a
different phase from [label substitution](0012-substitution-from-declared-labels.md) and gives it
different properties on purpose.

| | Label substitution | Secret resolution |
| --- | ------------------ | ----------------- |
| Happens during | Resolution | Apply |
| Reads | Repository content | A host-local backend |
| Affects the manifest digest | Yes | No |
| Visible in `datum render` | The resolved value | The reference only |
| Works from a checkout with no host | Yes | No |

A secret value never appears in an effective manifest, its digest, a plan, a report, a log, a metric
label or any provenance record. Those sinks are enumerated, not described, because a redaction rule
that is described tends to cover the outputs somebody remembered.

The secret provider is selected by agent configuration and not by desired state, for the same reason
[trust anchors are not managed by Datum](0010-no-self-managed-trust-anchors.md). A repository that could
nominate where secrets come from could nominate a source the attacker controls.

Secret providers are out-of-process and named, never commanded, exactly as [resource
providers](0002-separate-resources-from-providers.md) are. No backend is configured by supplying a
command line, because that would be [arbitrary root execution from
configuration](0011-no-command-execution-from-desired-state.md) arriving through a different door.

A reference that cannot be resolved fails the resource. It does not resolve to an empty value, and it
does not skip the field.

## Consequences

The `sensitive` flag and this mechanism are now clearly different things, which they were not before.
`sensitive: true` suppresses rendering of content that is in the repository, and a secret reference means
the content was never there. Both remain, because a file can be confidential without being a credential.

Rotating a secret in the backend changes nothing in Git, so the manifest digest does not change and
[`datum affected`](../repository/validating-changes.md#which-hosts-a-change-would-affect) will not
report it. That is correct and it is also a real limitation, because the digest answers what the
repository says rather than what a host will end up holding, and rotation therefore has to be
tracked by whatever performs it.

Resolution stays a pure function of the repository, which is what keeps `datum validate` able to
resolve every host in CI with no access to any secret backend. Validation confirms that a reference
is well formed and cannot confirm that it exists, so a reference to a secret nobody has created is a
failure that appears on a host and not in review.

This is a per-host boundary the design did not previously have. The repository is still readable by every
machine, and secret values are not in it, so compromising one host no longer discloses the fleet's
credentials.

Datum does not become a secret store. It resolves references against something that already exists, whether
that is a file placed by provisioning, an agent-authenticated fetch from a vault, or a platform's own
instance credential service. Deciding what that thing is remains the fleet's problem, and the interface is
the only part Datum specifies.
