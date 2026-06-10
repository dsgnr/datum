# ADR-0010: Datum does not manage its own trust anchors

## Status

Accepted

## Context

An agent verifying that desired state is genuine needs a set of trusted signing keys. An agent
reading a repository needs a credential. An agent detecting a downgrade needs a record of the
revision it last applied. All three are files on the host.

Managing them with Datum is the obvious thing to want. They are configuration, they differ per
host, and a fleet that can distribute its own signer list can rotate a key without rebuilding
machines. Every argument that applies to managing `/etc/ssh/sshd_config` appears to apply here.

The problem is that a control cannot be allowed to authorise its own replacement.

If `allowed-signers` were an ordinary `File` resource, one malicious commit would replace it with
an attacker's key. That commit would be verified against the old, trusted key and would pass.
Every commit after it would verify against the new key. Signature verification would have
validated the change that disabled signature verification, and nothing in the plan would look
unusual, because replacing a file is what Datum does.

The same reasoning applies to the recorded revision. A resource that could write the state
directory could reset the baseline used for downgrade protection, after which any older revision
becomes acceptable. It applies to the credential, where a resource could redirect a host at a
different remote. It applies to `agent.yaml`, which contains the host's identity and the trust
settings themselves, so a resource managing it could set `require: none` fleet-wide.

## Decision

Datum rejects a resource whose target is one of its own trust anchors.

| Path | Protects |
| ---- | -------- |
| `/etc/datum/agent.yaml` | Host identity, source, and the trust settings |
| `/etc/datum/allowed-signers` | The keys that can authorise desired state |
| `/etc/datum/credentials/` | Repository credentials |
| `/var/lib/datum/` | The recorded revision used for downgrade protection |

The refusal happens when the manifest is validated, so it fails before the host is touched and
the error names both the file and the control it protects.

The exclusion is a specific list, not a general rule about `/etc`. A broad exclusion would be
unpredictable, would grow by accident, and would stop Datum managing configuration it has no reason
to avoid.

These files are provisioned and rotated by whatever builds machines, which is outside Datum.

## Consequences

A compromised repository cannot escalate into a compromised trust configuration. The worst a
malicious commit achieves is whatever that commit's own content does on the hosts it matches, which
is already the accepted trust assumption, rather than permanently disabling the control that would
have caught the next one.

Rotating a signer key is a task for the provisioning system, and on a large fleet that is
genuinely worse than a commit would have been. This is the real cost of the decision and it is
accepted, because the alternative is a control that can be switched off by the thing it exists to
constrain.

Bootstrapping is pushed to provisioning, including the initial recorded revision. That turns out
to be the right place for it anyway, since a host that chooses its own first revision cannot
detect a downgrade on its first pass.

The list has to be kept accurate. A future control that relies on a file on the host has to be
added to it, and forgetting is the mistake that reopens the hole. Anything the agent reads in
order to decide whether to trust something belongs on the list, which is the test to apply when
the question comes up again.

The convenience argument will come back, most likely as a request for a narrow exception allowing
`allowed-signers` to be managed so that key rotation can be automated. The answer is that an
exception for the signer list is an exception for the entire control, because a fleet that can
rewrite its own signer list has the security properties of a fleet with no signer list at all.
