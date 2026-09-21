# Project status

Datum is being built, specification first. A complete pass runs end to end for all nine
resource types.

| Part | State |
| ---- | ----- |
| Document parsing and discovery | Implemented |
| Matchers, composition and precedence | Implemented |
| Substitution of declared label values | Implemented |
| Effective manifests and their digests | Implemented |
| The resource graph, cycles and duplicate targets | Implemented |
| Per-type field validation | Implemented |
| `datum render`, `datum explain`, `datum validate`, `datum affected` | Implemented |
| Observation, diffing and planning | Implemented |
| `datum observe`, `datum diff`, `datum plan` | Implemented |
| The `File`, `Directory` and `Symlink` provider | Implemented |
| The program runner providers execute through | Implemented |
| `Package` through `apt`, tested against Debian | Implemented |
| `Package` through `dnf`, tested against Fedora | Implemented |
| Provider selection from `os-release` | Implemented |
| `Sysctl` through `/proc/sys` and `/etc/sysctl.d` | Implemented |
| `User` and `Group` through the shadow utilities | Implemented |
| Drift a provider declares uncorrectable | Implemented |
| `Service` through `systemd`, tested against a booted systemd | Implemented |
| Applying, verification and failure propagation | Implemented |
| The pass lock and pass reports | Implemented |
| `datum reconcile`, `datum status` | Implemented |
| `datum agent`, `datum config check` | Implemented |
| `Repository` through `apt` and `dnf` | Implemented |
| The agent as a resident process, scheduling, metrics on a port | Implemented |
| Fetching from a remote, with a hardened checkout | Implemented |
| Signature verification and the descendant check | Implemented |
| Last known good, so a bad revision does not stop a host working | Implemented |
| `datum revision`, `datum version` | Implemented |
| Refusing resources that target Datum's own files | Implemented |
| Packaging as a `.deb` and an `.rpm` | Implemented |
| Grouping resource types into domains | Implemented |
| Secret resolution and reboot handling | Not started |
| `Package` and `Repository` through `apk` and `pacman` | Not started |
| `trust.strictPaths`, and the provider path-safety rules it switches on | Not started |
| The `--revision` and `--json` flags | Not started |
| `datum init` and `datum migrate` | Not started |
| Enrolment, so a host works out its own identity | Not started |

Applying is Linux-only. Reading a host works anywhere, since the safety rules
the writing path depends on have no portable equivalent.

The order follows from where a mistake costs least. Resolution is [a pure function of the
repository](../concepts/desired-state.md#resolution-does-not-read-the-host), so it can be built and
tested without a machine to break, and it is the half of the system every other part depends on.
Reading a host came next, since it changes nothing, which left applying as the last part to build.

Everything above the [provider boundary](../providers/index.md) is portable, so
the observer, the differ and the planner are tested without a host at all. A
provider is the only part that touches an operating system, which is the
separation [ADR-0002](../adr/0002-separate-resources-from-providers.md) records.

The specification came first because the decisions that are expensive to change
later are the ones made early. The shape of a resource, the way identity works,
where the provider boundary sits and how composition resolves are all choices that
an implementation would otherwise make incidentally, and would then be stuck with.

## What the documentation is for

This site serves two audiences at once. For somebody evaluating Datum it
explains what the system does and how it is meant to be used. For somebody
implementing Datum it is the specification to build against, which is why pages
describe observable behaviour, failure handling and edge cases instead of
staying at the level of overview.

Where something is undecided, the documentation says so. A specification that
fills in an answer nobody has thought through reads as settled when it is not.

## How this site labels design maturity

| Label | Meaning |
| ----- | ------- |
| **Accepted** | Decided, and recorded in an [architecture decision record](../adr/index.md). Implementation should follow it, and changing it means superseding the record. |
| **Proposed** | A concrete design that has not been accepted. Detailed enough to argue with, likely to change in the detail, and not safe to depend on. |
| **Planned** | Accepted in principle but not specified. It needs to exist and how it behaves has not been worked out. |
| **Open question** | A known gap with no resolution yet. Stated so that it is visible rather than discovered during implementation. |

A label describes how settled a design is, not whether it is built. The table at the top
of this page and the [support matrix](../providers/support-matrix.md) record what actually
works, and an entry appears there only once the behaviour exists and is tested. A page can
be labelled proposed and implemented at the same time, which is the normal state of an
alpha schema.

Most configuration examples are proposed. The `datum: v1alpha1` marker at the top of
every document says the same thing more formally, because the alpha suffix means field
names, defaults and semantics can change without a migration path until the schema
reaches a stable version.

## What is settled so far

Fourteen decisions are accepted, each with a record explaining what it was weighed against
and what it costs.

| Decision | Record |
| -------- | ------ |
| Desired state is expressed as typed resources | [ADR-0001](../adr/0001-typed-resources.md) |
| Resource types are distribution neutral, providers are not | [ADR-0002](../adr/0002-separate-resources-from-providers.md) |
| Git is the source of desired state | [ADR-0003](../adr/0003-git-as-desired-state-source.md) |
| Host identity is separate from host classification | [ADR-0005](../adr/0005-identity-separate-from-classification.md) |
| Dependencies are declared, never inferred | [ADR-0006](../adr/0006-explicit-dependencies.md) |
| The effective manifest is the engine's only input | [ADR-0007](../adr/0007-effective-manifest-as-input.md) |
| Resource references are separate from target identities | [ADR-0008](../adr/0008-resource-reference-and-target-identity.md) |
| Only declared resources are managed | [ADR-0009](../adr/0009-declared-only-ownership.md) |
| Datum does not manage its own trust anchors | [ADR-0010](../adr/0010-no-self-managed-trust-anchors.md) |
| Desired state never causes a command to run | [ADR-0011](../adr/0011-no-command-execution-from-desired-state.md) |
| Only declared label values are substituted into desired state | [ADR-0012](../adr/0012-substitution-from-declared-labels.md) |
| Secrets are referenced in the repository and resolved on the host | [ADR-0013](../adr/0013-secret-references-resolved-on-the-host.md) |
| The implementation is written in Go | [ADR-0014](../adr/0014-go-as-the-implementation-language.md) |
| Resource types are grouped into domains | [ADR-0015](../adr/0015-resource-type-domains.md) |

The reconciliation model itself is settled. Desired state comes from Git, observed state
comes from the host, the two produce a plan, and the plan is applied and verified. The
five phases and their order are not up for negotiation, because everything else in the
design assumes them.

## What is not settled

The [fleet composition model](../adr/0004-labels-and-matchers.md) is proposed,
not accepted. Labels and matchers are the intended mechanism, and the precedence
and conflict rules are written down, but they have not survived contact with a
real repository yet and are expected to move.

Every resource type schema is proposed. The common behaviour they share is close to
settled and the individual field sets are not.

More than sixty questions are recorded as unresolved, and they are collected in [open
questions](../development/open-questions.md). Several of them would otherwise be answered
by accident during implementation, which is the main reason the list exists.

## What the specification is held to

A code change that makes a statement on this site false is not finished until
the statement is fixed. The documentation is the specification and not a
description written afterwards, so the two moving apart is a defect in both.

That cuts the other way as well. Where the implementation found a gap the specification had not
thought through, the answer goes into the documentation as a decision instead of staying in the code
as an accident. The fourth [pass outcome](../concepts/reconciliation.md#pass-outcomes) arrived that
way, because an observe-mode pass that found work to do fitted none of the three that had been
written down.

## Versioning

There are no releases. The packages build from the repository and carry a version that sorts below
any real one, so a machine cannot end up with a build claiming to be something it is not. When there
are releases, the documentation will carry a version matcher and this page will be replaced by
something that records what shipped rather than what is intended.
