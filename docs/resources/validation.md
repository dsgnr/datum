# Configuration validation

Writing a syntactically invalid `sshd_config` and restarting sshd locks everybody out of a machine.
Most applications can check their own configuration, and Datum uses that check before the change
goes live.

Validating configuration content is a separate thing from checking that a repository is well formed,
which is [manifest validation](../reference/manifest-format.md#validation-summary) and happens at a
different stage. This page always means the former.

## Validation sits inside apply

The five phases apply to every resource. Configuration validation applies only to resources whose
content some application can check, which is a minority, so it is a step within apply rather than a
sixth phase.

```text
Observe  ->  Diff  ->  Plan  ->  Apply  ->  Verify
                                 │
                                 ├── stage
                                 ├── validate
                                 └── activate
```

It sits inside apply because it operates on content that has been staged and not yet activated, which
is a state that only exists during apply and only for one resource at a time.

## Staging already makes this possible

The `File` provider [already writes to a temporary file in the target
directory](../security/provider-safety.md#writing-a-file) and renames it into place, because a rename
is atomic and a partial write is not.

That existing mechanism is what validation needs. Between the write and the rename there is a complete
copy of the proposed content on the same filesystem, at a path the validator can be pointed at, while
the live file is untouched.

```text
write staged content      /etc/ssh/.datum.tmp.a91f
set ownership and mode
validate                  sshd -t -f /etc/ssh/.datum.tmp.a91f
rename into place         /etc/ssh/sshd_config
```

Validation failing means the rename never happens, the temporary file is removed, and the live
configuration is as it was. A failed validation changes nothing on the host.

## Validators are referenced by name

A validator is referenced by name.

```yaml
datum: v1alpha1
type: File

name: sshd-config

desired:
  path: /etc/ssh/sshd_config
  owner: root
  group: root
  mode: "0600"
  source: files/sshd_config
  validate: sshd
```

`sshd` names a validator definition that a provider supplies. The repository does not say what command
to run, what arguments to pass, or where the binary is.

A field holding a command line is arbitrary root execution driven by repository content, which
[ADR-0011](../adr/0011-no-command-execution-from-desired-state.md) rules out. A validator is the
most likely route by which that capability would have arrived without being noticed.

| Value | Behaviour |
| ----- | --------- |
| omitted | No validation. The default. |
| a name | The named validator runs against the staged content. |
| `none` | Explicitly no validation, for a file a validator would reject for unrelated reasons. |

Nothing is validated implicitly. A `File` at `/etc/nginx/nginx.conf` does not acquire `nginx -t`
from its path. Behaviour inferred from a path needs internal knowledge to predict, which the
[understandable principle](../introduction/design-principles.md#understandable) rules out, and
opting in is one field.

## Validators Datum expects to ship

!!! note "Proposed behaviour"

    The set below is what the first implementation is likely to carry. Each depends on the
    application supporting validation of a file at an arbitrary path, which not all of them do. The
    right-hand column records what was checked rather than what is assumed.

| Name | Checks | Arbitrary path supported |
| ---- | ------ | ------------------------ |
| `sshd` | `sshd_config` syntax | Yes, sshd accepts a configuration file argument |
| `sudoers` | sudoers syntax | Yes, and a rejected sudoers file locks out privilege escalation |
| `nginx` | nginx configuration | Partially, because nginx resolves includes from the file it is given |
| `systemd-unit` | unit file syntax | Yes, analysis of a unit file is possible without installing it |

The nginx row is the general problem, not an nginx quirk. An application whose configuration is one
file validates cleanly in isolation, and an application whose configuration is a directory of includes
does not, because validating one fragment says nothing about the whole.

## Configurations spanning several files

This is the part the design does not fully solve.

A validator pointed at one staged fragment of a multi-file configuration either ignores the rest, which
makes the check meaningless, or reads the live copies of the rest, which validates a combination that
will never exist because the other fragments are about to change too.

The position taken is to validate the assembled result after writing and before activating.

```text
write every changed file in the group
validate the application configuration as a whole
reload or restart only if validation passed
```

A service that is not reloaded keeps running the configuration it already loaded, so a failed
validation leaves files on disk that are wrong and a service that is unaffected. The host is then
one whose configuration will be wrong after its next restart, and the service is still serving in
the meantime.

!!! note "Open question"

    Two things about this are unresolved. Expressing that several resources form one application
    configuration needs a grouping concept, which the design has
    [avoided](applications.md#an-application-is-not-a-datum-concept) so far and which validation is
    the first genuine argument for.

    The second is that files left wrong on disk do not show as drift, because they match desired state,
    so the pending reload has to be recorded somewhere or the next pass will consider the host
    converged while it holds a configuration that would fail on restart. That interacts with the
    [`awaiting-reboot` state](../concepts/state.md#reboots), which has the same shape, and neither is
    designed.

## When a validator is missing

A named validator whose binary is not installed on the host is a failure, not a skip.

Proceeding without it would mean validation stopping on the hosts where the tooling is absent, and
nothing saying so. A repository asking for validation gets validation or gets an error.

The usual cause is a missing dependency, and the fix is an ordering edge to the package providing the
validator, which the manifest can already express.

## Validators must not have side effects

A validator reads the staged content and reports whether it is acceptable. It does not write, does not
restart anything, and does not touch the live configuration.

This is a requirement on the validator definitions Datum ships rather than something Datum can
enforce, since it cannot inspect what a third-party binary does. A validator that modified state
would break the guarantee that a failed apply changed nothing, which is what validation before the
rename is for.

## Privileges and output

Validators run as the agent does, which is root. A configuration file at mode `0600` owned by root
cannot be read by anything less. This is also why validator definitions are provider-supplied code
rather than repository content.

Validator output is captured and reported on failure, and the output is treated as potentially
sensitive. A validator that echoes the offending line of a configuration file will echo whatever that
line contains, which for a file holding a credential means the credential lands in the failure report.

Output from a validator on a resource marked
[`sensitive`](../security/provider-safety.md#marking-content-as-sensitive) is suppressed, and only the
fact of failure and the validator's exit status are reported. For everything else the output is included
because diagnosing a syntax error without it is guesswork.

!!! note "Open question"

    How much validator output belongs in a report that may be collected centrally is undecided.
    Truncating it loses the diagnostic, keeping it risks disclosure through a channel the
    [redaction rule](../security/provider-safety.md#reports-and-content-disclosure) otherwise closes,
    and the `sensitive` flag only covers files somebody remembered to mark.

## Validation against verification

The two are separate mechanisms answering separate questions.

| | Validation | Verification |
| --- | ---------- | ------------ |
| When | Before the change is activated | After the change is applied |
| Input | Staged content, not yet live | The live state of the host |
| Asks | Will the application accept this? | Did the intended state actually result? |
| Performed by | A validator, inside apply | The [observer](../resources/lifecycle.md#verify), as a phase |
| Applies to | Resources with a validator | Every resource, always |
| On failure | Nothing changed | Something changed and is wrong |

Validation is optional, narrow, and prevents a class of failure. Verification is universal,
mandatory, and detects failures of every class including the ones validation cannot anticipate.

A configuration can validate and still be wrong, because `sshd -t` checks syntax and not whether the
resulting policy is sensible, so validation passing is never taken as evidence that the change was
correct. Verification is what establishes that the host reached the state that was asked for, and
[convergence is defined by it](../concepts/reconciliation.md#convergence).

The combinations are as follows.

| Outcome | Result |
| ------- | ------ |
| Validation fails | Resource `failed`, live state untouched, dependents blocked |
| Validation passes, apply fails | Resource `failed`, state possibly partially changed, dependents blocked |
| Apply succeeds, verification fails | Resource `failed`, change was made and did not produce the intended state |

In all three the resource is `failed`, dependents are
[blocked](../concepts/state.md#resource-state-within-a-pass), independent resources continue, and
the next pass observes and tries again. Datum attempts no recovery of its own, since [there is no
rollback](../concepts/reconciliation.md#there-is-no-rollback), and a recovery path for the
validation case alone would cover the least damaging failure of the three.
