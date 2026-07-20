# Staged rollout

A commit merged to the tracked branch reaches every host its matchers select, on each host's next pass.
For a fleet of five hundred reconciling every thirty minutes that is the whole estate within half an
hour, with no decision point after review and no way to stop it.

This page specifies how a fleet stages that, using Git and nothing else.

!!! note "Proposed behaviour"

    The mechanism is proposed. It is included because staging turns out to need nothing central.

## Rings are branches

A host tracks one ref, named in its agent configuration. A fleet that wants staging gives different
hosts different refs and moves changes between them.

```yaml title="/etc/datum/agent.yaml"
source:
  url: https://git.example.com/fleet.git
  branch: production
```

```text
canary       5 hosts
early        50 hosts
production   445 hosts
```

A change lands on `canary` first. When it looks healthy there, `canary` is merged into `early`, and
later `early` into `production`. Each merge is a fast-forward, so the revision every ring eventually
applies is byte-identical to the one the canary ring proved.

```text
canary      A---B---C
early       A---B
production  A
```

Promotion moves a branch pointer and introduces no new content, which is what makes a ring's success
evidence about the next ring and not merely encouraging.

## Why the ring is not in the repository

Putting a `ring` label on a `Host` document and having layers match it would be the obvious design, and
it is wrong.

A ring is a control over how far a change spreads. If the ring a host belongs to were declared in the
repository, then a single commit could both make a change and widen the set of hosts that change reaches,
which means the mechanism intended to limit a change's blast radius would itself be changeable by that
change. Review would have to catch it, and a matcher edit looks nothing like a rollout decision.

Keeping the ref in agent configuration means ring membership is a property of the machine, set when
it is provisioned, and changing it is an act on that machine, not a commit. That is the same
reasoning that keeps [reconciliation
mode](../concepts/reconciliation-modes.md#where-the-mode-is-set) and [trust
settings](../security/repository-trust.md#agent-configuration) local.

It also means the repository has one description of every host regardless of ring, so `datum validate`
still resolves the whole fleet and a host does not resolve differently because of which ring it is in.
Only *when* it receives a change differs.

## Seeing what a promotion would do

```text
$ datum affected --from production --to canary

42 of 500 hosts affected

web-001    sha256:3f2a9c4e -> sha256:8d10b7f2
...
```

[`datum affected`](../reference/cli.md#datum-affected) already answers this, because comparing two
revisions is what it does and two ring branches are two revisions. Running it between rings before a
merge reports exactly which hosts the promotion will change, which is the question a promotion decision
turns on.

## Pausing and reverting

Pausing a rollout means not performing the next merge. Nothing has to be told to stop, because hosts in
the later rings never saw the change.

Reverting a ring that has already received a change is a commit on that ring's branch, in the same way
[any correction is a new commit](../concepts/reconciliation.md#there-is-no-rollback). A `git revert`
moves history forward and satisfies
[downgrade protection](../security/repository-trust.md#verifying-that-a-revision-is-current), whereas
resetting a ring branch backwards and force-pushing does not, and hosts on that ring would refuse the
result.

That constraint is better learned before an incident than during one. A ring branch is not a thing
to rewind.

## Rings have to stay linear

Every ring branch must be an ancestor of the ring ahead of it. Promotion is a fast-forward merge and
never a merge commit that introduces content.

The reason is the descendant check. A host records
[the newest revision it accepted](../security/repository-trust.md#verifying-that-a-revision-is-current),
and if ring branches diverge then a host moved from one ring to another may be offered a revision that is
not a descendant of what it already has. It will refuse, correctly, and keep refusing.

```text
linear       production is an ancestor of early is an ancestor of canary
diverged     a host moved between rings fails the descendant check
```

Moving a host between rings is therefore safe in the direction of promotion and needs a
[baseline reset](../security/repository-trust.md#the-cases-this-makes-awkward) in the other. Demoting a
canary host back to `production` hands it a revision older than the one it has accepted, which is
indistinguishable from a downgrade attack and is refused as one.

## What this does not do

Promotion is a human decision, or a decision made by something outside Datum reading fleet status. There
is no automatic gate that holds a merge until every canary host reports `converged`, and nothing
measures whether the application on those hosts is healthier or worse.

Datum supplies the inputs for that decision, being
[per-host state](../reference/status.md), the
[revision each host has applied](../observability/metrics.md#the-metrics) and the affected-host set,
and a fleet builds the gate in whatever already runs its merges.

Deliberately so, because a gate needs to know what healthy means for the service being configured, and
that is [the boundary where Datum's responsibility ends](../resources/validation.md#where-datums-responsibility-ends).
A converged host running a broken application is exactly the case
[verification cannot catch](../resources/validation.md#verification-is-not-a-health-check), so a gate
built only on Datum's signals would promote it.

!!! note "Open question"

    Whether the agent should report which ref it tracks, so that a fleet can see its ring composition
    without inspecting every machine, is undecided. It is useful and it means publishing a fact about a
    host's place in the rollout order to anything that can reach
    [the metrics endpoint](../observability/metrics.md#binding-and-exposure), which discloses which hosts
    receive changes last and are therefore the most predictable to target.
