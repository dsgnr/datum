# Secret references

Desired state names a secret and never contains one. A host resolves the name to a value while applying,
and the value exists nowhere that Datum writes.

!!! note "Proposed behaviour"

    The field names and the backend interface are proposed. The boundary is decided, and it is recorded
    as [ADR-0013](../adr/0013-secret-references-resolved-on-the-host.md), which is that a reference
    travels through the repository and a value never does.

## Why a reference rather than a value

Every managed host [reads the whole repository](../architecture/deployment-models.md). A
secret committed there, encrypted or not, is readable by every machine under management, and encrypting it
does not help because all of those machines would need the key.

Holding a reference instead means the repository describes which secret a file should contain without
containing it, so the blast radius of one compromised host stops at that host's own secrets.

## A whole file that is a secret

```yaml
datum: v1alpha1
type: File

name: app-tls-key

requires:
  - Directory[app-tls]

desired:
  path: /etc/app/tls/server.key
  owner: app
  group: app
  mode: "0600"
  secretRef: app/tls-key
```

`secretRef` is mutually exclusive with `content`, `source` and `template`. The file's entire content
is the resolved secret, and
[`sensitive`](../security/provider-safety.md#marking-content-as-sensitive) is implied and not
something that has to be remembered.

## A secret inside a configuration file

```yaml
datum: v1alpha1
type: File

name: app-config

desired:
  path: /etc/app/app.yaml
  owner: app
  group: app
  mode: "0640"
  template: files/app.yaml.tmpl
```

```text title="fleet/roles/app/files/app.yaml.tmpl"
site: {{ labels.site }}
database:
  host: db.{{ labels.site }}.example.com
  password: {{ secrets.db_password }}
```

Two placeholder namespaces appear there and they are resolved in different phases, which is the
thing to understand about them.

`{{ labels.site }}` is [substituted during resolution](../fleet/substitution.md), so the rendered value is
in the effective manifest and contributes to its digest. `{{ secrets.db_password }}` is left alone by
resolution and resolved on the host during apply, so the manifest carries the placeholder.

```text
$ datum render --host web-001

File[app-config]   /etc/app/app.yaml
  template   files/app.yaml.tmpl
  secrets    db_password
```

The manifest naming which secrets a resource consumes, without their values, is what keeps a rendered file
reviewable. A reader can see that this file needs one secret and what it is called.

## Where the value comes from

```yaml title="/etc/datum/agent.yaml"
secrets:
  provider: file
  path: /etc/datum/secrets
```

The backend is agent configuration, never desired state. A repository that could nominate where secrets
come from could nominate a source an attacker controls, which is the same reasoning that keeps
[trust anchors](../adr/0010-no-self-managed-trust-anchors.md) out of the repository, and
`/etc/datum/secrets/` is protected as one for the same reason.

| Provider | Resolves a reference by |
| -------- | ---------------------- |
| `file` | Reading `<path>/<reference>`, root-owned at mode `0600` |
| `exec` | Not offered, see below |
| Others | An out-of-process backend speaking the provider protocol |

The `file` provider is the trivial case and is genuinely useful, because it turns secret distribution into
a provisioning problem a fleet may already have solved. A vault, an age-encrypted store with a host key, or
a platform's instance credential service are all out-of-process backends behind the same interface.

There is no backend configured by supplying a command to run. A command in a configuration field is
[arbitrary root execution](../adr/0011-no-command-execution-from-desired-state.md) arriving through a
different door, and a backend that needs to run something is
[an extension](applications.md#extensions) installed the way extensions are installed.

## Resolution failure fails the resource

A reference the backend cannot resolve makes the resource `failed`, and dependents are
[blocked](../concepts/state.md#resource-state-within-a-pass).

```text
failed   File[app-config]
         secret   db_password could not be resolved
         reason   no such key in provider "file"
```

Resolving to an empty value would be the harmful alternative. An empty password in a configuration file is
syntactically valid, would be written successfully, would pass
[configuration validation](validation.md), and would leave an application running with no credential. A
file that was not written is recoverable, and a file written with an empty secret may already have been
read.

The existing file is left exactly as it was, because a secret-bearing file is written through the same
[stage, validate, activate](../security/provider-safety.md#writing-a-file) sequence as any other, and a
failure before the rename changes nothing.

## What this unblocks

```yaml
datum: v1alpha1
type: User

name: deploy

desired:
  state: present
  shell: /bin/bash
  passwordRef: users/deploy-hash
```

`User` gains `passwordRef`, which resolves to a password hash, not a password. The hash is still
secret material, which is why it is a reference, and Datum does not compute hashes because doing so
would mean handling the plaintext.

An `authorized_keys` file is a `File` with `secretRef`, which is not strictly secret and is
authorisation-bearing, so the same handling is wanted. Placing a public key that grants shell access
is a change to keep out of a repository every host can read.

## What is deliberately not solved

Datum does not store secrets, generate them, rotate them, or know when they changed.

Rotation is the consequence that needs stating plainly. Changing a value in the backend changes
nothing in Git, so the [manifest digest](../fleet/effective-manifests.md#content-addressing) is
unchanged and [`datum
affected`](../repository/validating-changes.md#which-hosts-a-change-would-affect) reports nothing. A
host picks up the new value on its next pass, because it resolves the reference every time, and
nothing in Datum records that the value differs from the one applied before.

That is consistent with the digest answering what the repository says rather than what a host holds,
and it means rotation has to be tracked by whatever performs the rotation.

!!! note "Open question"

    Whether a resource should be able to state that a secret's value changed, so that a
    [`restartOn`](dependencies.md#restarton) edge can fire when a certificate is renewed, is undecided.
    Detecting it needs the host to remember a digest of a resolved secret between passes, which is
    [cross-pass state](../architecture/reconciliation-flow.md#what-is-written-down) holding a
    fingerprint of secret material, and that is a worse thing to store than it first appears.

!!! note "Important limitation"

    A secret reference protects the value from the repository and not from the host. The resolved value is
    written to a file on that machine as root, so anything with root there can read it, and an agent
    holding a backend credential can fetch every secret that credential permits. Narrowing what one host
    may resolve is the backend's job, and Datum's contribution is that the reference says which secrets a
    host needs, so a backend has something to authorise against.
