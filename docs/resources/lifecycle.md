# Resource lifecycle

Every resource participates in the same five operations, regardless of its type.

```text
Observe
Diff
Plan
Apply
Verify
```

Three of them are performed by the resource's provider, and two are performed by
the planner. The split keeps the comparison in the planner and the
implementation in the provider.

| Operation | Performed by | Input | Output |
| --------- | ------------ | ----- | ------ |
| Observe | Provider | The resource's target identity | Current field values, or absent |
| Diff | Planner | Desired and observed field values | The fields that differ |
| Plan | Planner | Differing fields and the graph | One action, positioned in an order |
| Apply | Provider | The resource and the chosen action | Success or failure |
| Verify | Provider, through the observer | The resource's target identity | Current field values again |

## Observe

A provider reports the current state of the target named by the resource, using
the fields the resource type defines.

Absence is a normal answer rather than an error. A `File` whose path does not
exist reports `exists: false`, and a `Package` that is not installed reports the
same. The planner needs that distinction to choose between `create` and
`update`.

Observation reports what is there rather than what was asked for. A provider
that returned desired values where it cannot read actual ones would leave the
diff, the plan and verification all working from the manifest they were meant to
be checked against. This is the tightest constraint on a provider.

## Diff

The planner compares field by field. Only fields the resource declares are
compared, so a `File` that sets `mode` and not `owner` is compared on mode alone,
and the ownership on disk is left as it is.

This is what makes partial management work. Declaring three fields of a file
leaves Datum with no opinion about the other three, rather than an expectation
that they match a default.

## Plan

One action is chosen per resource from the observed and desired state.

| Target exists | Desired | Fields differ | Action |
| ------------- | ------- | ------------- | ------ |
| No | present | n/a | `create` |
| Yes | present | Yes | `update` |
| Yes | present | No | `none` |
| Yes | absent | n/a | `remove` |
| No | absent | n/a | `none` |

A resource whose dependency failed, or for which no provider is available, gets
`skip` instead of any of the above.

Resource types that do not create or destroy their targets use a narrower set.
`Service` never gets `create`, since a unit exists because a package put it
there. `Sysctl` never gets it either, since a kernel parameter cannot be brought
into existence.

## Apply

The provider carries out exactly the action the plan specified.

A provider is not asked whether the action is necessary and does not check. That
decision was made during planning, from the whole manifest and the whole
observation. A provider that re-derived it could disagree with the plan, and the
plan would stop describing what the pass does.

Applying is expected to be atomic where the underlying operation allows it. A
`File` update writes to a temporary path in the same directory and renames it
into place, so a process reading the file concurrently sees either the old
content or the new content, never a half-written one.

## Verify

After applying, the affected resource is read again through the same observation
path used at the start of the pass, and the result is compared against desired
state.

Verification is not a second attempt. A resource that does not match after
applying is reported as unverified, and the pass outcome is `failed`. The next
pass is what corrects it, so the failure is reported rather than absorbed by a
retry loop.

Verification is a distinct operation because success from a provider is a weaker
claim than correctness. Restarting a unit can succeed while the service exits a
second later. Writing a file can succeed while the mode ends up wrong.
Installing a package can succeed while a post-install script reverts a
configuration file another resource owns.

## Idempotency

Applying a resource that is already in desired state produces no change.
Planning is what guarantees that, rather than a check inside each provider.

A converged resource gets the action `none`, and `none` means the provider is
never called. A provider has no check to get right, so the guarantee holds the
same way for every type.

The observable test is that a second pass immediately following a successful one
produces an empty plan. A resource type that fails it has a defect in its
observation or its comparison, where a written value is not read back in the
same form.

The usual cause is a field that is normalised on write. A mode written as `0640`
that reads back as `416`, or content with a trailing newline added by the
provider, produces a resource that differs on every pass and restarts everything
depending on it each time.

## When a provider cannot do something

A provider reports what it cannot do rather than approximating around it.

A field the provider cannot observe is reported unobservable, so it cannot be
diffed or verified, and the plan records it that way rather than assuming it
matches. A field the provider cannot set fails the action, so a partly applied
resource is reported instead of counting as a success.

!!! note "Proposed behaviour"

    Whether a resource with an unobservable field should be planned at all is
    undecided. Applying a field that cannot then be verified is a change Datum
    cannot confirm, which sits badly with the requirement that a pass reports
    whether the resulting state was verified. Refusing may be the better answer.
