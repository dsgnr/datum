# Enrolment

Enrolment is the step where a machine with no identity acquires one the fleet recognises. It happens
once per machine per installation of the operating system, and it is the only point at which
something has to be delivered to a machine out of band.

!!! note "Proposed behaviour"

    None of this exists, and the mechanisms are less settled than the constraints. What each way of
    placing a credential costs is set out under
    [handshakes](../security/handshake.md#getting-the-repository-credential-onto-a-host).

## What enrolment produces

There is no authority to enrol with, so enrolment reduces to two local facts and a commit.

```text
1. the machine is given a host name and a repository credential
2. a Host document for that name is committed to the repository
3. the next pass resolves and reconciles
```

| Produced | Chosen by | Lives where |
| -------- | --------- | ----------- |
| A host name | Whoever provisions the machine, never the machine | `/etc/datum/agent.yaml` and a `Host` document |
| A repository credential | The Git server | On the machine |

The credential is a long-lived read-only Git credential, not something Datum issues or renews.
Nothing authenticates the host to Datum, so there is no key material of Datum's own.

The ordering is worth noting, because a machine configured with a name that has no `Host` document
fails its pass with an error naming the identity it claimed, which is the correct outcome and looks
like a failure instead of a machine waiting.

## The host name is not a security boundary

The name in `agent.yaml` is a local assertion and nothing verifies it. That is safe because every
agent can read the whole repository anyway, so a machine claiming a different name gains nothing it
did not already have, and
[identity selects configuration rather than granting access](../architecture/host-identity.md#identity-is-not-a-security-control).

It still matters who chooses the name. A machine that supplies its own name during enrolment decides
which configuration it receives, which is the escalation path
[host identity](../architecture/host-identity.md#why-the-split-matters) exists to close. The name
comes from whatever provisions the machine, and the labels come from the `Host` document.

## Reinstallation

A machine that has its operating system reinstalled is provisioned again the same way, and its
`Host` document stays where it is.

Nothing has to be retired, because nothing was issued. Host names are reusable and there is no key
bound to them, so rebuilding `web-001` a dozen times produces one name and no accumulated state.

## Machines that never enrol

A machine that is installed and never given a name and credential does nothing. It resolves no
desired state and applies no changes, which is the correct behaviour for a machine nobody has yet
decided anything about.

An offline machine is the same case with different timing. Fetching needs to reach the Git remote,
so a machine with no network path stays unconfigured until it has one, instead of reconciling
against a cached or assumed identity.

!!! note "Open question"

    Whether unenrolled machines should be discoverable at all is undecided. A fleet cannot tell the
    difference between a machine that was never meant to be managed and one whose enrolment failed,
    and closing that gap would mean something outside Datum knowing which machines are supposed to
    exist. That is inventory, not configuration, and putting it in Datum would make Datum
    responsible for a list it has no way to verify.
