# Leaving the fleet

A machine leaves a fleet in one of three ways, and the three are separate operations with different
actors, different effects and different reversibility. Conflating them produces either machines that
are still being managed long after anyone thought they were gone, or machines that were emptied when
somebody only meant to stop changing them.

!!! note "Proposed behaviour"

    None of this exists. The distinctions matter now because a tool that makes unenrolment
    destructive teaches operators to fear it, and a tool that makes it silent leaves fleets full of
    machines nobody can account for.

## Revocation stops changes

Revocation withdraws a host's ability to obtain desired state. It happens on the Git server, and Datum
has no part in it.

| | |
| --- | --- |
| What is revoked | The host's Git credential, on the Git server |
| Who performs it | An operator, on the Git server |
| Mechanism | Deleting or disabling a deploy key or token |
| Takes effect | At the host's next fetch |
| Datum's involvement | None, there is no command for it |

The credential is a long-lived read-only Git credential recorded at
[`source.credential`](../security/repository-trust.md#repository-credentials). Datum has no command
for revoking it and cannot have one, because the agent holds no authority over the credential it was
given and there is nothing central to ask.

This is the main practical reason [per-host credentials](../security/repository-trust.md#repository-credentials)
are preferred over one shared credential. Revoking one host means removing one credential on the Git server,
whereas revoking a shared credential is a fleet-wide rotation.

A revoked host keeps reconciling from whatever it last fetched until its next fetch fails. From then
on it runs the configuration it last applied and reports nothing new.

The `Host` document stays in the repository, so the machine is still described by the fleet and still
counted by anything reporting on it. Revocation is reversible by issuing a new credential, and
nothing about the machine's configuration has changed.

## Unenrolment removes a machine from the description

Unenrolment removes the host's `Host` document, so the fleet stops describing the machine at all. It
is a commit.

A host can stop participating on its own by having its agent disabled or removed, and that is not
unenrolment. The fleet still describes it, still expects it to report, and will show it as
[unknown](../reference/status.md#the-host-status-model) instead of gone. A machine cannot remove
itself from the fleet's description, which is what keeps that description from being something
individual machines can edit.

!!! warning "Deleting the Host document is not enough"

    Removing a `Host` document stops the fleet describing the machine and does not stop the machine
    reading the repository. It still holds a valid read credential, which grants access to
    [the whole repository](../architecture/deployment-models.md), including every other host's
    configuration. Its next pass fails with an error naming the identity it claimed, and it keeps
    the access it had.

    Unenrolment is therefore two operations that have to happen together, the commit that removes
    the `Host` document and the revocation of that host's credential on the Git server. Doing only
    the first leaves a departed machine with fleet-wide read access indefinitely.

## Unenrolment is not destructive

Unenrolling a host leaves every resource Datum applied to it exactly as it is. Packages stay
installed, files stay in place, services keep running, and nothing is reverted.

This is the same rule as [declared-only ownership](../adr/0009-declared-only-ownership.md) applied
to a whole machine. Datum removes something when a repository declares it absent, and the absence of
a declaration means no opinion, not an instruction to remove, so a machine that drops out of the
fleet's description is a machine Datum has stopped having opinions about.

The alternative would be worse in a way that is easy to miss. An unenrolment that removed managed
resources would mean deleting a `Host` document, or a matcher change that stops a layer matching, is
capable of stripping a running production machine. Refactoring a repository would then be a
destructive operation, and no amount of care in review makes that a safe tool to hand somebody.

Emptying a machine is therefore a separate, explicit sequence.

```text
1. declare the resources absent in the repository
2. let the host converge, and confirm that it did
3. unenrol the host
```

The ordering there is a requirement, not a convention. Unenrolling first removes the host's ability
to resolve desired state, so the declarations that would have emptied it never reach it, and the
machine is left both undescribed and fully configured.

## Decommissioning a machine

Decommissioning is the whole sequence, ending with a machine that no longer exists.

```text
$ datum plan --host web-001

remove   Service[nginx]
remove   File[nginx-config]
remove   Package[nginx]
remove   User[www-data]

0 to create, 0 to update, 4 to remove, 0 to skip, 10 unchanged
```

Removal runs in reverse dependency order, which falls out of the [dependency
graph](../architecture/dependency-graph.md) rather than being a special case for decommissioning. A
service is stopped before the package providing it is removed, because the ordering that made the
package a prerequisite for the service still holds in reverse.

How much of a machine to empty is a judgement, not a rule. A virtual machine about to be deleted
does not need its packages removed first, and a physical machine being handed to another team does.
Datum makes both expressible and chooses neither.

!!! note "Important limitation"

    Removal is only as complete as the declarations. Datum removes the resources a repository
    declared and knows nothing about what an application wrote at runtime, so a decommissioned host
    can be fully converged on an empty desired state and still hold data, logs and state
    directories. Treating a converged removal as a wipe would be a mistake.

## What the fleet keeps afterwards

The repository holds the whole history, so a `Host` document that existed and was deleted remains
visible in Git along with the commit that removed it, who authored it and when. That is the audit
record, and it is the same record that covers every other change to desired state.

What is not retained is the machine's own reporting. Each host writes its report locally, so a
decommissioned machine takes its history with it, and a question about what `web-001` was actually
running three months ago cannot be answered from the repository.

!!! note "Open question"

    Where per-host reports are retained is undecided, and it is the gap that makes decommissioning
    lossy. Git records what a host was told to do and nothing records what it reported doing, so an
    audit after a machine is gone has the intent and not the outcome. Retaining them centrally would
    need a component Datum does not have.
