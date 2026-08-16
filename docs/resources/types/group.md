# Group

`Group` describes a local group.

```yaml
datum: v1alpha1
type: Group

name: deploy

desired:
  state: present
  system: false
```

Target identity is the group name, taken from `name`.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `state` | `present`, `absent` | Yes | Whether the group should exist. |
| `gid` | integer | No | A specific numeric id. |
| `system` | boolean | No | Whether to allocate a system-range id. |

## There is no members field

Membership is declared on [`User`](user.md) and only there.

A `members` field here would let a repository say that `deploy` contains `alice`
while the `User[alice]` resource says `alice` is in no groups, and both documents
would be individually valid. Detecting that as a conflict is possible, and removing
the ability to express it is simpler and needs no detection at all.

The consequence is that this type is small. It exists so that a group can be brought
into existence with a known id before the users that reference it, which is an
ordering problem `requires` solves.

```yaml
datum: v1alpha1
type: User

name: deploy

requires:
  - Group[deploy]

desired:
  state: present
  primaryGroup: deploy
```

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the group exists. |
| `gid` | The numeric id. |

## Identifiers

As with `User`, a group that exists with a different gid than the one declared is
drift that Datum reports and does not correct, because changing a gid orphans the
group ownership of every file that refers to it. The provider declares the field
[uncorrectable](../../concepts/drift.md#drift-no-provider-will-correct), so the
difference is reported on every pass and the number is left alone.

Since `gid` is the only field a group has, that makes an update to an existing group a
no-op by construction. A `Group` resource either creates the group, removes it, or reports
a gid that will not be changed.

## Removal

`state: absent` removes the group. A group that is still the primary group of an
existing account cannot be removed, and the action fails rather than forcing it.

Ordering removals is the caller's problem, expressed the same way as any other
ordering. A group and its users being removed in the same pass needs the users
declared absent and the group depending on them, which is the reverse of the
creation order and has to be written out, not inferred.

!!! note "Open question"

    Reversing dependency direction between creation and removal is not something
    the model handles. `requires` means "processed before this one" regardless
    of the action, so a manifest that removes a group and its users has to
    express the ordering that removal needs, which is the opposite of what the
    same resources would need when being created. Whether the planner should
    reverse dependency edges for `remove` actions is undecided, and doing so
    would make plan order depend on the action instead of only on the declared
    graph.
