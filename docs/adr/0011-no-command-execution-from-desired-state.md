# ADR-0011: No command execution from desired state

## Status

Accepted

## Context

Installing a package is frequently not enough to reach a usable state. A database needs
initialising, a schema needs creating, a licence needs activating, and an application needs a
one-time setup step that no `Package`, `File` or `Service` resource describes.

The obvious answer is a resource that runs a command, and every configuration management tool has
one. It is also the answer that has been rejected three times in this design already, in
[ADR-0001](0001-typed-resources.md) for the resource model, in the [type
set](../resources/types/index.md#types-considered-and-left-out), and in the [command line
interface](../reference/cli.md#commands-deliberately-absent) itself.

What forced this decision was noticing that a generic command resource is not the only way the
capability arrives. Two other features want it, and both look innocuous.

A configuration validator needs to run `sshd -t` before a new `sshd_config` goes live, and the
obvious design is a field holding the command to run. A one-time initialiser needs to run a
migration once, and the obvious design is a field holding the command plus a field holding a command
that reports whether it has already run.

Each of those fields is arbitrary command execution as root, driven by repository content. A
repository that can write `validate: {command: [...]}` can write anything, and the guarantee that
Datum only performs described state changes is gone, arriving through a field nobody thought of as
an escape hatch instead of a resource type somebody would have argued about.

## Decision

No field in desired state causes Datum to execute a command supplied by the repository. This holds
for resource types, validators, initialisers, health checks and anything added later.

Desired state declares intent using types. The code that realises intent is a provider, and
providers are [extensions](../resources/applications.md#extensions) installed on the host, running
out of process and versioned and reviewed separately from the repository.

Where a command has to run, the command lives in an extension and the repository names the type that
extension implements. Validators work the same way, being named definitions a provider supplies, not
command lines a repository writes.

## Consequences

The trust boundary stays where the rest of the design puts it. Repository content is reviewed and
is not trusted with arbitrary root execution, and code on the host is trusted because installing
it is already a privileged act. This is the same reasoning as
[ADR-0010](0010-no-self-managed-trust-anchors.md), and it means a compromised repository cannot
reach beyond changing state that Datum can describe, diff and verify.

Every guarantee the design makes survives contact with real applications. A resource is still
observable, diffable and verifiable, because there is no category of resource that is none of those
things, so drift detection, planning, idempotence and verification hold across every manifest rather
than across the manifests that happened not to use an escape hatch.

Datum is markedly less immediately flexible than a tool with an `exec`. A novel problem cannot be
solved by writing a shell command into YAML, and the answer is to write an extension or to manage
that thing some other way. That is a genuine and permanent cost, and it is the price of the
guarantees above.

The extension mechanism becomes load-bearing, not optional. It is the only path for anything Datum
does not already model, so a weak extension story makes the whole design impractical, and it moves
from a nice-to-have to a prerequisite for adoption beyond the built-in types.

Extensions are harder to distribute than YAML. Somebody has to build, sign, install and update
them on every host that needs them, which is a real operational burden and is
[not yet designed](../development/open-questions.md).

The convenience argument will come back, and it will come back as a narrow exception instead of a
request for a general `Exec`. Something like allowing a command only in a validator, only where it
cannot modify anything, is the shape to expect. The answer is that a validator running as root with
arguments from the repository is arbitrary root execution regardless of what it is called, and an
exception for it is an exception for the entire decision.
