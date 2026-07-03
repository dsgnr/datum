# Journey: changing sshd configuration across production

A change with real consequences. Tightening `sshd_config` across every production host, where getting it
wrong locks everybody out of the estate.

## The change

```yaml title="fleet/environments/production/sshd.yaml"
datum: v1alpha1
type: File

name: sshd-config

requires:
  - Package[openssh-server]

desired:
  path: /etc/ssh/sshd_config
  owner: root
  group: root
  mode: "0600"
  source: files/sshd_config
---
datum: v1alpha1
type: Service

name: sshd

requires:
  - Package[openssh-server]
restartOn:
  - File[sshd-config]

desired:
  state: running
  enabled: true
```

The new `files/sshd_config` disables password authentication and root login.

## Before merging

The change is reviewable as a diff of declared state, which is most of the value of the model here. A
reviewer sees the configuration file's contents change and sees that `Service[sshd]` will restart as a
consequence, because `restartOn` names the file.

It is also renderable before it exists anywhere. `datum render --host web-001 --revision <branch-sha>`
resolves the manifest for a specific host from the branch, and `datum plan` against that revision shows
what would change on that host, from a laptop, without touching production.

That property comes from [resolution not reading the
host](../concepts/desired-state.md#resolution-does-not-read-the-host), and it is what makes reviewing a
fleet-wide change tractable.

## The risk the design does not remove

If the new `sshd_config` is malformed, sshd will fail to start after the restart, and the host becomes
unreachable. Datum will notice, and noticing is not the same as fixing.

Nothing in Datum prevents this. A valid manifest containing a broken configuration file resolves
cleanly, validates cleanly, and applies faithfully, which is the
[boundary of validation](bad-commit.md#what-this-journey-does-not-cover). Review and a staged rollout are
the mitigations, and the second one Datum does not currently provide.

!!! note "Important limitation"

    This journey merges straight to the branch every production host tracks, so every production
    host the layer matches picks the change up on its next pass. A fleet that wants the change to
    reach five hosts first uses [ring branches](../reconciliation/staged-rollout.md), which is a
    property of how the fleet is provisioned, not something this journey's repository expresses.

    What remains absent either way is a gate. Nothing holds a promotion back until the canary ring
    reports healthy, because deciding what healthy means for `sshd` is
    [outside what Datum measures](../resources/validation.md#where-datums-responsibility-ends).

## What a host does

**Resolution.** The production environment layer now contributes two more resources, so the effective
manifest for every production host grows and its digest changes.

**Observation.** The existing `sshd_config` is read and its digest differs. The mode on disk is `0644`
where the manifest asks for `0600`. The unit is active and enabled.

**Plan.** Ordered by the graph, so the package sorts first with nothing to do, then the file, then the
service restart.

```text
none     Package[openssh-server]   present

update   File[sshd-config]
         path      /etc/ssh/sshd_config
         mode      0644 -> 0600
         content   differs
         from      environments/production

update   Service[sshd]
         reason    File[sshd-config] changed, restartOn matched
```

**Apply.** The file is written [safely](../security/provider-safety.md#writing-a-file), to a temporary
file in `/etc/ssh` with ownership and mode set on the descriptor before the content, then renamed. A
concurrent sshd reading the file sees either the old content or the new content and never a partial one.

**Verify.** The file is re-read and matches. The unit is checked for being active and enabled.

## When sshd fails to start

This is where verification earns its place in the model.

`systemctl restart sshd` with a malformed configuration fails, so the provider reports failure and
the resource is `failed`. Had the restart instead succeeded and the daemon exited a moment later,
the [verification step](../resources/lifecycle.md#verify) would catch it, because verification reads
the unit's state rather than trusting the restart's exit code.

```text
none     Package[openssh-server]   present
update   File[sshd-config]         ok
failed   Service[sshd]             unit failed to start
```

The pass outcome is `failed`, the host state is `failed`, and the
[failing alert](../observability/alerting.md#alerts-worth-having) fires.

The uncomfortable part is what has already happened. The file was written before the service failed, so
the host is [partially changed](../concepts/reconciliation.md#pass-outcomes), and
[nothing is undone](../concepts/reconciliation.md#there-is-no-rollback). A host whose sshd is down is a
host that cannot be logged into to fix by hand.

What saves the situation is that the agent is still running locally and still reconciling. A corrective
commit reaches the host on its next pass without anyone needing to log in, which is the one respect in
which this failure is better under Datum than under a push-based tool that has just lost its transport.

## Recovery

Committing a corrected `files/sshd_config`. The next pass observes the broken file, diffs it against the
corrected content, plans an update and a restart, applies both, and verifies that sshd is active.

The host converges without intervention. That works because the agent runs on the host and pulls, so
losing sshd does not lose the management channel, which is a consequence of the deployment model and
not of anything specific to this journey.

## What this journey tests

Verification has to be a distinct phase, because a restart exiting zero is a weaker claim than a service
running, and this is the case where the difference locks people out of an estate.

It also shows where per-host correctness stops being enough. Reconciliation is correct on each
machine independently, and the order and rate at which machines receive a change is a separate
concern that [ring branches](../reconciliation/staged-rollout.md) answer with Git instead of a
central component.
