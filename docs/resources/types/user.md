# User

`User` describes a local user account.

```yaml
datum: v1alpha1
type: User

name: deploy

requires:
  - Group[deploy]

desired:
  state: present
  primaryGroup: deploy
  groups:
    - docker
  home: /home/deploy
  shell: /bin/bash
  comment: Deployment account
```

Target identity is the user name, taken from `name`.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `state` | `present`, `absent` | Yes | Whether the account should exist. |
| `uid` | integer | No | A specific numeric id. |
| `primaryGroup` | string | No | The account's primary group. |
| `groups` | list of strings | No | Supplementary groups the account belongs to. |
| `home` | absolute path | No | Home directory path. |
| `shell` | absolute path | No | Login shell. |
| `comment` | string | No | The GECOS comment field. |
| `system` | boolean | No | Whether to allocate a system-range id. |
| `passwordRef` | string | No | Names a [secret](../secrets.md) holding the password hash. |

Fields not set are not managed, so a resource declaring `shell` and nothing else
corrects the shell and leaves the rest of the account alone.

## Group membership

Supplementary group membership is expressed on the user. `Group` has no
`members` field, so a repository cannot make two conflicting statements about
who is in a group.

The cost is that membership can only be managed for users that Datum manages, so
adding an existing account to a group means declaring that account as a resource.

`groups` is the complete set rather than an addition. A user whose resource
lists `docker` should be in `docker` and no other supplementary group, and any
other membership is corrected. This follows the general rule that lists replace.

!!! note "Open question"

    Exclusive membership is consistent with the rest of the model and breaks
    where another system manages a group. Whether an additive mode is needed,
    and whether it can exist without making the field's meaning depend on a
    flag, is undecided.

## Identifiers

Omitting `uid` lets the system allocate one, which is normal for accounts that only
exist locally. Setting it makes the id part of desired state, which matters where uids
have to match across machines because of shared storage.

An account whose uid differs from the declared one is reported as drift and not
corrected. Changing a uid leaves every file owned by the old one without a
matching account, and the manifest does not describe which files to reassign.

!!! note "Important limitation"

    A uid mismatch is reported and not acted on. This is the only field in the
    model where a difference is left uncorrected, since correcting it would
    orphan files.

    The provider declares the field uncorrectable, so the difference appears in the diff and in the
    plan marked as such, the resource ends the pass `drifted` and not `converged` or `failed`, and
    the number on the host is left alone.

    ```text
    User[deploy]
      uid  4242 -> 4343  (reported, not corrected)
    ```

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the account exists. |
| `uid` | The numeric id. |
| `primaryGroup` | The primary group name. |
| `groups` | Supplementary group names. |
| `home` | Home directory from the account record. |
| `shell` | Login shell. |
| `comment` | The GECOS comment field. |
| `password` | A digest of the stored hash, never the hash. |

`home` is read from the account record rather than the filesystem, so a user
whose home directory is recorded but missing reports the recorded path. Managing
the directory requires a `Directory` resource.

!!! note "Implementation status"

    `User` and `Group` are implemented by the `linux-user` provider, which reads through `getent`
    and changes through the shadow utilities. `getent` covers accounts from any configured name
    service rather than only local ones in `/etc/passwd`.

    The two utilities do not share an option table. `useradd` spells the home directory `--home-dir`
    and `usermod` spells it `--home`.

## Removal

`state: absent` removes the account and does not remove its home directory, its mail
spool, or any files it owns elsewhere.

The data is left in place, so removing an account is not destructive on its own.
A home directory that should also go is declared as a `Directory` with `state:
absent`, which puts the deletion in the plan.

!!! note "Security consideration"

    Removing an account does not remove its access if authorised keys live
    somewhere Datum does not manage, and files owned by a deleted uid become
    owned by a bare number that a future account could be allocated. Neither is
    something Datum can fix, and both need knowing about before using `state:
    absent` to revoke access.

## Open questions

Datum does not compute password hashes. `passwordRef` names a [secret](../secrets.md) that already
holds one, so the plaintext is handled by whatever populates the secret backend.

`passwordRef` is not observable in the way other fields are. A hash can be read from the account
record and compared, so drift is detectable, and the comparison happens on the host after the
reference resolves, not while diffing against the manifest.

Whether `absent` should have a `locked` counterpart, which disables login without
removing the account, is undecided and is probably the more common operational need.
