# Host lifecycle

A machine enters a fleet, participates in it for a while, and eventually leaves. Each of those
transitions is a separate operation with its own failure modes, and treating them as one step is
what produces fleets where nobody can say which machines are still being managed.

## The three phases

Installation
:   The agent and its providers are present on the machine. Nothing about the fleet is configured
    and the machine is not yet described by any repository.

Enrolment
:   The machine acquires an identity the fleet recognises and whatever credential that identity
    needs. From this point it can resolve desired state and reconcile.

Departure
:   The machine stops participating. This covers three different operations that are routinely
    confused, and they are separated below.

Keeping installation and enrolment apart is what makes golden images and immutable build pipelines
workable, because an image can carry the agent without carrying anything that identifies a
particular machine or grants access to the fleet.

```text
image build      agent installed, no identity, no credential
first boot       enrolment, identity issued
steady state     reconciliation
departure        revocation, unenrolment or decommissioning
```

## Revocation, unenrolment and decommissioning are different

These three get used interchangeably and they have different effects, different actors and
different reversibility.

| Operation | Performed by | Effect | Reversible |
| --------- | ------------ | ------ | ---------- |
| Revocation | An operator, on the Git server | The host can no longer obtain desired state | Yes, by issuing a new credential |
| Unenrolment | An operator, in a commit | The `Host` document is removed | Partly, by provisioning again |
| Decommissioning | An operator, over time | The machine is emptied and then switched off | No |

Revocation is a security operation that cuts off future changes, and
[it is not containment](../security/handshake.md#getting-the-repository-credential-onto-a-host), because a
revoked host keeps running whatever configuration it last applied.

Unenrolment is an administrative operation that removes a machine from the fleet's description. It
is never destructive, which is covered in full under
[decommissioning](decommissioning.md#unenrolment-is-not-destructive).

Decommissioning is a sequence of reconciliations that empty a machine of the things Datum put
there, followed by unenrolment, followed by whatever the platform does with a machine nobody needs.
The ordering matters, because unenrolling first removes the means of emptying it.

## The pages

[Installation](installation.md)
:   Getting the agent onto a machine, what an image may and may not contain, and how that fits
    with cloud-init, network installation and autoscaling.

[Enrolment](enrolment.md)
:   How a machine acquires an identity, why that identity is not a security boundary, and what
    happens when a machine is reinstalled.

[Decommissioning](decommissioning.md)
:   Leaving the fleet, in each of the three senses, and what the fleet retains afterwards.

!!! note "Implementation status"

    None of this exists. The reasoning is recorded because the boundaries between these phases
    determine what an image may contain and what an operator has to do by hand, and both of those
    are expensive to change once fleets depend on them.
