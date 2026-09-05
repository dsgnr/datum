# Compared with other tools

Configuration management has a long history and Datum borrows from most of it. What follows is where
the design differs from the established tools and where those tools are the better choice, because a
comparison that only lists advantages is marketing, not documentation.

!!! note "Read this alongside project status"

    Every tool below has been in production for years or decades. Datum is alpha, manages nine
    resource types on two distributions, and has never run a fleet. Nothing here is a claim that it
    is ready to replace them, and [project status](project-status.md) is the honest account of what
    works.

## The tools

| Tool | What it is |
| ---- | ---------- |
| [Puppet](https://www.puppet.com/) | Declarative resources, a typed resource and provider model, an agent that pulls a compiled catalogue. The closest relative of this design. |
| [Chef](https://www.chef.io/) | Declarative resources described in a Ruby DSL, applied by an agent. |
| [Ansible](https://www.ansible.com/) | Tasks pushed over SSH, with declarative modules underneath. No agent. |
| [Salt](https://saltproject.io/) | Declarative states, an agent connected to a master, strong remote execution. |
| [CFEngine](https://cfengine.com/) | Promise theory, a small continuously running agent. The oldest of these and the closest in spirit. |
| [NixOS](https://nixos.org/) | A whole system built from a declarative expression, activated atomically. |
| [Flux](https://fluxcd.io/) and [Argo CD](https://argo-cd.readthedocs.io/) | Reconciliation from Git, for Kubernetes objects rather than hosts. Where the GitOps part of this design comes from. |

## Model and delivery

| Tool | Describes | Reaches a host | Runs |
| ---- | --------- | -------------- | ---- |
| Datum | Declared resources | Agent pulls from Git | Continuously, on an interval |
| Puppet | Declared resources | Agent pulls a catalogue, or applies locally | Continuously, on an interval |
| Chef | Declared resources | Agent pulls from a server, or applies locally | Continuously, on an interval |
| Ansible | Tasks, with declarative modules | Pushed over SSH | When somebody runs it |
| Salt | Declared states | Agent connected to a master, or applies locally | Continuously or on demand |
| CFEngine | Promises | Agent pulls policy | Continuously, every few minutes |
| NixOS | The whole system | Rebuild and switch | When somebody rebuilds |
| Flux, Argo CD | Kubernetes objects | Controller pulls from Git | Continuously |

## The questions this design treats as requirements

Five things in [why Datum](why-datum.md) are requirements here, not features. Most of the
established tools answer several of them, and Puppet answers nearly all.

| Tool | Dry run is the real code path | Verifies after applying | Explains where a value came from | Corrects drift unprompted | Host verifies who authored the change |
| ---- | ----------------------------- | ----------------------- | -------------------------------- | ------------------------- | ------------------------------------- |
| Datum | Yes | Yes, re-reads and reports | Yes, layer and matcher per resource | Yes | Yes, signature and ancestry checked on the host |
| Puppet | Yes | Reports what it changed | Data lookups, yes. Which classes apply, less so | Yes | No, it trusts the server over TLS |
| Chef | Partly, `why-run` has gaps | Reports what it changed | Attribute precedence is traceable with effort | Yes | No |
| Ansible | Varies by module | No | Variable precedence, with effort | Only when run | No |
| Salt | Yes, `test=True` | Reports what it changed | With effort | Yes | No |
| CFEngine | Yes | Yes | With effort | Yes | No |
| NixOS | The build is the artefact | The system either activates or does not | Yes, the expression is the whole answer | No, not until a rebuild | No, pinning gives integrity and not authorship |
| Flux, Argo CD | Yes, a diff against live objects | Yes, health assessment | Yes | Yes | Yes, both can verify commit signatures |

The last column is the one that is unusual outside the Kubernetes tools. In the agent-and-server
model the host trusts whatever the server sends, so compromising the server compromises the fleet.
Datum has no server, and the host
[checks the signature and the ancestry itself](../security/repository-trust.md).

## Puppet

Puppet is the closest relative and the design owes it the most. Typed resources, a provider layer
that hides distribution differences, an agent reconciling on an interval, and a real dry run are all
Puppet ideas that arrived here more or less intact.

Three differences are deliberate, not incidental.

Puppet's manifests are written in a programming language with conditionals, loops and functions, and
classification happens through node definitions, an external classifier or the roles and profiles
pattern. That is more expressive than anything here. It is also why tracing how a value reached one
machine can mean reading the whole repository. Datum has no conditionals in desired state at all, and
[composition is matchers with explicit precedence](../fleet/precedence.md), which is a real loss of
expressiveness bought for a gain in traceability.

Puppet has an `exec` resource, so a manifest can run a command. Datum
[refuses to run commands from desired state](../adr/0011-no-command-execution-from-desired-state.md),
which makes write access to the repository something short of root on every host and also means the
escape hatch that makes `exec` so useful is missing.

The last difference is not about design at all. Puppet's open-source distribution changed during
2025, when Perforce moved new binaries to a [private location under a
licence](https://www.puppet.com/blog/open-source-puppet-updates-2025) and the community [forked the
project as OpenVox](https://voxpupuli.org/openvox/). That matters when choosing today, and it is not
an argument that this project is a substitute for either of them.

## NixOS

NixOS is the more radical design and it achieves something Datum explicitly does not. A NixOS system
is built from its expression, so the same expression produces the same system, and activation is
atomic with a previous generation to boot into. That is reproducibility, and
[Datum is declarative without being reproducible](../reconciliation/last-known-good.md#declarative-is-not-reproducible),
because a pinned package version still depends on a package repository serving it.

The trade is what has to be adopted. NixOS replaces how the whole machine is built, so it suits a
fleet willing to move to it and not a fleet of existing Debian and Rocky Linux machines that needs
the configuration on them described. Datum manages declared resources on conventional distributions
and has no opinion about the rest of the machine, which is a smaller promise and a much smaller
migration.

Drift behaves differently too, because between rebuilds a hand edit on a NixOS host persists with
nothing watching for it. Datum reads the host every pass and corrects what it declared.

## Ansible

Ansible is the most widely used of these and the least like this one. Tasks run when somebody runs
them, over SSH, with no agent to install. That is a genuine operational advantage, and it is why
Ansible is so often the right answer for provisioning, orchestration and one-off work across a mixed
estate.

What it does not give is a machine that stays converged. Between runs nothing corrects drift, and
answering whether a host matches its description means running a playbook and reading the output.
Check mode fidelity varies by module, so a dry run is an approximation in a way that a plan here is
not.

## Flux and Argo CD

These are where the GitOps part of the design comes from, and the model is the same, being a
controller that reconciles declared state from Git on an interval, corrects drift and verifies
commit signatures.

The difference is what they reconcile. Both act on the Kubernetes API, which is a uniform,
transactional interface with a controller for every resource type. A Linux host has no such thing.
Applying state to a host means driving `apt`, `dnf`, `systemd`, the account database and the
filesystem, each with its own idea of what an error is, which is why
[providers](../providers/index.md) exist and why so much of this design is about
[what happens when part of a pass fails](../reconciliation/failure-handling.md).

## Where the established tools are better

Being honest about this is more useful than a feature matrix.

**Coverage.** Puppet, Chef, Ansible and Salt manage hundreds of resource types across Linux, BSD,
Windows, macOS, network devices and cloud services. Datum manages
[nine types on Debian and Fedora](project-status.md).

**Ecosystem.** The Forge, Galaxy and Supermarket hold thousands of reusable modules written and
maintained by other people. There is nothing here to reuse.

**Maturity.** These tools have run large fleets for years, and their failure modes are known,
documented and worked around. Datum has never managed a production fleet, and the first serious
adoption will find things this documentation claims are settled and are not.

**Scope of the problem.** Secrets, Windows hosts, orchestration across machines and anything that is
not a single Linux host reconciling itself are out of scope here, and
[secret resolution is not even implemented](../resources/secrets.md).

**People.** Somebody already knows Puppet. Nobody knows this.

## Where this design differs deliberately

**No command execution from desired state.** Write access to the repository is not root on every
host, which costs the escape hatch every other tool on this page provides.

**The host decides whether to trust a change.** Verification and downgrade protection happen on the
machine instead of being delegated to a server, so there is no component whose compromise is
fleet-wide.

**It refuses to manage its own controls.** A commit cannot replace the key set that authorises it,
which is [ADR-0010](../adr/0010-no-self-managed-trust-anchors.md).

**Provenance is an output.** Every resource carries the layer and matcher it came from, so
[`datum explain`](../reference/cli.md#datum-explain) answers why a value is on a host without anybody
reading the repository.

**The resource model is small on purpose.** Nine types and no conditionals is a constraint, not a
roadmap gap. Whether that survives contact with a real fleet is the open question the whole project
turns on.

## If a tool already works

Keep it. None of the properties above justifies a migration on its own, and a fleet running Puppet
or Ansible successfully has solved the problem this project is still learning about.

The cases where this design is interesting are narrower. A fleet that wants desired state in Git with
the host verifying who signed it, that needs to explain why a value is on a machine without reading
everything, that would rather have no way to run a command from a repository than have one, or that
finds the Kubernetes GitOps model right and wants it for ordinary Linux hosts.
