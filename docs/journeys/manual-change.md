# Journey: somebody edits a managed file

An engineer edits `/etc/nginx/nginx.conf` on `web-001` during an incident at 02:00, raising a worker
limit to get through a traffic spike. Nothing is committed. This journey follows what Datum does next,
which depends entirely on the host's [mode](../concepts/reconciliation-modes.md).

## The starting state

`web-001` is converged on revision `7ab21f`, with `File[nginx-config]` managing that path with mode
`0600` and content from `files/nginx.conf`. Its last pass reported `converged`, and its metrics show a
recent `datum_pass_last_success_timestamp_seconds`.

The edit changes the file's content and nothing else. The mode is untouched, and the package and
service are unaffected.

## The next pass, in enforce mode

**Resolution.** Unchanged. The repository is still at `7ab21f`, so the fleet resolver produces the
same effective manifest with the same digest, `sha256:3f2a9c4e`. Nothing about the edit is visible at
this stage, because [resolution does not read the
host](../concepts/desired-state.md#resolution-does-not-read-the-host).

**Observation.** The `File` provider reads the path and reports a content digest that no longer
matches. Mode, owner and group still match.

**Diff.** One field differs on one resource. Drift is recorded [per
field](../concepts/drift.md#drift-is-per-field), so the diff says content differs and says nothing
about mode.

```text
File[nginx-config]
  path     /etc/nginx/nginx.conf     match
  mode     0600                      match
  owner    root                      match
  content  differs                   drift
```

**Plan.** `File[nginx-config]` gets `update`. Because `Service[nginx]` declares `restartOn` for that
file, and `restartOn` [implies ordering](../resources/dependencies.md#restarton), the service is
scheduled for an update after the file with the reason recorded.

```text
update   File[nginx-config]
         content  differs
         from     roles/web, hosts/web-001

update   Service[nginx]
         reason   File[nginx-config] changed, restartOn matched

0 to create, 2 to update, 0 to remove, 0 to skip, 12 unchanged
```

**Apply.** The file is rewritten from the repository content, discarding the manual edit. The service
restarts.

**Verify.** Both resources are read again. The file matches and the unit is active and enabled, so the
pass outcome is `changed`.

**Status.** The host is `converged` again. `datum_passes_total{outcome="changed"}` increments, and the
engineer's change is gone.

Nothing in that sequence treated the edit as an error. Datum did not know a human made it, did not
know when, and did not need to, because the comparison is against the repository, not against a
record of what Datum last did.

## The problem with that outcome

The worker limit that was keeping the site up has just been reverted, automatically, at 02:00,
by a system nobody was watching.

This is the tension the design names explicitly under [safety against
reconciliation](../introduction/design-principles.md#where-the-principles-pull-against-each-other).
A tool that corrects drift automatically will eventually correct a change somebody made on purpose,
and the correction is most likely to happen at the least convenient moment, because incidents are
when manual changes happen.

## The same pass, in observe mode

**Everything up to the plan is identical.** Resolution, observation and diffing produce exactly the
same result, because [modes change only whether the reconciler
applies](../concepts/reconciliation-modes.md#what-a-mode-does-not-change).

**The pass stops after the plan.** Nothing is applied. The worker limit survives.

**Status differs.** The host reports `drifted` and not `converged`, with
`datum_resources{state="drifted"}` at 1, and the drift stays reported on every subsequent pass until
either the host is corrected or the repository is.

That is the useful behaviour during an incident, and it is why the mode is [host-local
configuration](../concepts/reconciliation-modes.md#where-the-mode-is-set) rather than something a
commit controls. An engineer who needs a machine to stop being corrected can change it on that
machine, without a commit and without coordinating with anybody.

## What happens afterwards

The edit needs resolving in one of two directions, and Datum's job is to make the choice visible,
not to make it.

If the change was right, it belongs in the repository. Committing the new worker limit makes
`File[nginx-config]` match the host, at which point the next pass reports converged without applying
anything, because the host already holds what the repository now asks for.

If the change was wrong, reverting the host is the next enforcing pass, which is what would have
happened automatically had the mode not been changed.

The case the design does not handle well is the third one, where the change was right and nobody
commits it. The drift is reported indefinitely, and on an enforcing host it is reverted repeatedly.
A resource corrected on consecutive passes should be reported as a suspected conflict, and
[detecting that](../development/open-questions.md) is an open question because it needs history
across passes.

## What this journey tests

Drift caused by a human is the same measurement as drift caused by a repository change, and neither
needs a special code path. Per-field drift matters, because a content change should not cause an
ownership rewrite. And the mode is the mechanism that makes automatic correction a choice instead of
an imposition, which is what makes Datum safe to run on machines that matter before it is fully
trusted.
