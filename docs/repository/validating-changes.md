# Validating changes before merging

A commit that reaches the tracked branch reaches every host its matchers select, so the useful place to
catch a mistake is the pull request. Everything on this page runs without access to any managed
machine.

!!! note "Proposed behaviour"

    The commands described here are proposed, not implemented. What makes them possible is already
    decided, which is that [resolution does not read the
    host](../concepts/desired-state.md#resolution-does-not-read-the-host), so every check below is a
    function of repository content alone.

## datum validate

A single command runs everything that can be checked without a host.

```text
$ datum validate

fleet      example
revision   9c02ab
hosts      500
layers     14

resolved 500 hosts, 0 errors
```

It parses every document, checks each against its declared
[schema version](schema-versions.md), discovers layers and hosts, then resolves and builds a graph for
**every host in the fleet**.

Resolving every host is the important part here. A conflict between two layers only exists for hosts
that both layers match, and a dependency cycle can appear in one host's manifest and not another's.
Validating the documents in isolation therefore misses precisely the errors that composition
introduces.

```text
$ datum validate

error: conflicting values for File[nginx-config].mode
  0640  roles/web            precedence 30
  0600  roles/web-tls        precedence 30
  affects 42 hosts, first: web-001

error: dependency cycle
  Service[nginx] -> File[nginx-config] -> Package[nginx] -> Service[nginx]
  affects 42 hosts, first: web-001

resolved 500 hosts, 2 errors
```

Reporting how many hosts an error affects, instead of the same error once per host, is what keeps
the output readable on a fleet. One typo in a widely-matched layer produces one error against 500
hosts instead of 500 separate errors.

Every row of the [manifest validation table](../reference/manifest-format.md#validation-summary) is
checked here, which is the same set an agent checks before touching a host. CI running `datum validate`
and an agent resolving a revision reject exactly the same commits, so a change that passes CI cannot
fail resolution on a host for a reason CI could have seen.

## There is no datum lint

Validation either passes or fails, and no separate command exists to produce advice alongside it.

A lint that emits warnings nobody is required to fix accumulates warnings nobody fixes, at which
point the output stops being read at all. Anything reportable is either an error, in which case it
belongs in `validate`, or a matter of taste, in which case it is not Datum's business.

Where a check is genuinely advisory, such as the proposed warning about a
[likely missing dependency](../resources/dependencies.md#where-dependencies-come-from), it is a
warning from `datum validate` and `--strict` promotes warnings to errors. That gives a repository the
choice between tolerating them and forbidding them, without a second command and a second exit code to
reason about.

## Which hosts a change would affect

The question a reviewer most often needs answered is how far a change reaches, which is what `datum
affected` reports.

```text
$ datum affected --from origin/main --to HEAD

42 of 500 hosts affected

web-001    sha256:3f2a9c4e -> sha256:8d10b7f2
web-002    sha256:3f2a9c4e -> sha256:8d10b7f2
...
db-001     unchanged
```

This resolves every host at both revisions and compares the resulting
[manifest digests](../fleet/effective-manifests.md#content-addressing). A host whose digest is
unchanged is provably unaffected by the change, because its desired state is byte-identical across the
two revisions.

Answering that question is the reason content addressing was adopted in the first place, so that a
single value could stand for an entire resolved desired state. Comparing two of them tells a reviewer the
blast radius of a commit before merging it.

```text
$ datum affected --from origin/main --to HEAD --show-resources --host web-001

web-001    sha256:3f2a9c4e -> sha256:8d10b7f2

  File[sshd-config]     added     environments/production
  Service[sshd]         added     environments/production
  File[nginx-config]    mode      0644 -> 0600
```

The per-host detail is a diff between two manifests instead of two revisions, so it reports what a
host's desired state will become and not what changed in the repository. Those two differ whenever a
change alters a layer that only some hosts match.

## Pull request validation

```yaml title=".github/workflows/fleet.yml"
name: Fleet
on: [pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: datum validate --strict
      - run: datum affected --from origin/${{ github.base_ref }} --to HEAD
```

The `fetch-depth: 0` setting matters, because `affected` needs both revisions present locally.

Nothing in that workflow needs a credential for a managed host, a network path to one, or a running
agent. A fleet of any size is fully validated by a job with a checkout, which is a direct consequence
of resolution being a pure function of the repository.

Posting the affected-host summary as a review comment is what turns it from a log nobody opens into
part of the review itself. Reviewing a change that says it touches 42 production web servers is a
different conversation from reviewing one that says it touches a YAML file.

## What CI cannot tell you

Validation proves a commit is well formed and resolvable, and it proves nothing about whether the
desired state it describes is correct.

A commit declaring `Package[nginx]` absent across production validates cleanly, affects a knowable
number of hosts, and removes nginx everywhere. The distinction between a malformed change and a mistaken
one is
[the boundary of what validation defends](../journeys/bad-commit.md#what-this-journey-does-not-cover).
The `affected` command exists precisely because the mistaken case can only be caught by a human who knows
how many machines the change reaches.

CI also cannot tell you whether a configuration file is syntactically acceptable to the application
that will consume it, because that is [configuration validation](../resources/validation.md) and it
needs the application's own tooling on a host. A repository can run those validators in CI against a
container holding the application, which is useful and is a fleet's own arrangement, not something
Datum provides.
