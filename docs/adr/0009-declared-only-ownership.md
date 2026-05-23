# ADR-0009: Manage only declared resources

## Status

Accepted

## Context

Desired state describes some of a machine. The question is what Datum should believe about
everything it does not describe.

Treating desired state as a complete description of the host is internally consistent and
appealing. Anything present on the machine and absent from the manifest would be removed, which
makes a host's state exactly what the repository says and makes drift impossible to accumulate
quietly.

It is also unusable. Adopting Datum on an existing machine would begin by deleting most of it,
because no repository describes every package, file and user on a working system. Reaching a
complete description would mean declaring thousands of resources before the first pass could
safely run, and a single omission would delete something that mattered. The agent runs as root,
so the cost of getting that wrong is not recoverable.

Treating undeclared things as unmanaged is the opposite position. A host can be partially
managed, adoption is incremental, and the blast radius of an incomplete repository is bounded by
what it declares.

The cost is that removing a resource from the repository stops managing it rather than removing it
from the host, which is not what everybody expects.

## Decision

Datum manages what is declared. A resource absent from an effective manifest is not managed and
is not scheduled for removal.

Removing something requires declaring that intent.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Package

metadata:
  name: nginx

spec:
  state: absent
```

The same rule applies within a resource. Only the fields a resource declares are compared, so a
`File` setting `mode` and not `owner` corrects permissions and leaves ownership as it is.

## Consequences

Adoption is incremental. A repository can start with three resources on a running machine and
grow, which makes the first pass safe to run on something that matters.

Deleting a resource document is not a way to remove software. `Package[nginx]` removed from the
repository leaves nginx installed on every host that has it, and Datum stops having an opinion.
Removal is a two-step change, first declaring `state: absent` and later deleting the resource once
every host has converged.

The blast radius of an incomplete repository is bounded. Nothing is removed because it was
forgotten, which is the failure mode the alternative would have made routine.

Partial field ownership means an omission is indistinguishable from an intention. A file whose
ownership matters needs ownership declared even when it currently looks correct, because Datum
cannot tell the difference between not caring and not having noticed.

Drift reporting means less than it appears to on a lightly managed host. A machine where forty
resources are managed and four hundred files matter can be fully converged and still be wrong, so
the coverage of a repository is a real property to know.

Drop-in directories are the one case where this rule produces the wrong answer. A file left in
`/etc/nginx/conf.d` after being removed from the repository keeps being loaded by the service, and
the general rule leaves it alone. That is recorded as an open question against the `Directory`
type, and it is the strongest argument for some bounded form of reclaim behaviour.
