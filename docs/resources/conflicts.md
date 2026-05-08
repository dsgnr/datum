# Conflicts

Two resources conflict when they manage the same thing on the host. This is
distinct from a composition conflict, where two layers disagree about a field of
one resource, and the two are detected at different points for different reasons.

| Kind | Detected by | When |
| ---- | ----------- | ---- |
| Composition conflict | Fleet resolver | Two layers of equal precedence set the same field differently. |
| Resource conflict | Graph builder | Two resources in the manifest claim the same target. |

Both stop the pass before the host is read, and neither is resolved by a rule that
picks a winner.

## Duplicate target identity

The clearest case is two resources of the same type naming the same target.

```text
error: duplicate target identity /etc/nginx/nginx.conf

  File[nginx-config]         roles/web
  File[nginx-tls-settings]   roles/web-tls
```

Applying both would mean the second overwrote the first, the plan would report two
successful updates, and verification would pass for whichever ran last. The file
would then be rewritten on every pass as the two resources took turns, which reads
as permanent drift with no obvious cause.

Refusing the manifest turns that into an error naming both resources and the layers
that contributed them.

## Overlapping types

Conflicts also occur between different types, which is harder because the overlap is
not visible from the target identities alone.

A `File` and a `Directory` cannot both claim `/var/log/app`. A `Sysctl` provider that
persists settings by writing into `/etc/sysctl.d` overlaps with any `File` resource
targeting the same path. In both cases the resources look unrelated until something
has to put a value on the filesystem.

!!! note "Proposed behaviour"

    The intended mechanism is that a provider declares the host paths it owns,
    including paths it writes as an implementation detail rather than as its
    target identity. The graph builder then checks declared ownership across
    every resource in the manifest, so a `File` targeting a path a `Sysctl`
    provider writes is rejected in the same way as two `File` resources sharing
    a path.

    This has not been specified in detail, and the awkward part is that path
    ownership can depend on which provider is selected, which happens after the
    graph is built.

## Overlapping facts across types

Some conflicts are about a fact, not a path, and the cleanest way to handle them
is to make sure only one type can express the fact at all.

Group membership is the example that forced the decision. A `User` can declare the
groups it belongs to, and a `Group` could plausibly declare its members, at which
point a repository can say two incompatible things and both are valid on their own.

```yaml
apiVersion: datum.dev/v1alpha1
kind: User

metadata:
  name: deploy

spec:
  state: present
  groups:
    - docker
```

`Group` therefore has no `members` field. Membership is a property of a user,
and there is exactly one place in the model where it can be written, which means
the conflict cannot be expressed at all, so nothing has to detect it.

!!! note "Important limitation"

    The cost is that membership can only be managed for users Datum manages. Adding
    an existing, undeclared user to a group requires declaring that user as a
    resource, which means taking over its other fields as well. Whether a `User`
    resource should be able to manage membership alone without asserting anything
    else about the account is an open question.

## Conflicts Datum cannot see

A conflict between Datum and something else on the machine is invisible to the graph
builder, because the other party is not in the manifest.

Another configuration tool managing the same file, a package post-install script
rewriting a configuration file on upgrade, and a service that rewrites its own
configuration at runtime all produce the same signature. The resource drifts, Datum
corrects it, and it drifts again immediately.

Detecting that signature is the only realistic defence, since Datum cannot know what
else is running. A resource corrected on consecutive passes is reportable as a
suspected external conflict, which turns a permanently churning host into a specific
complaint about a specific resource.

!!! note "Open question"

    This needs history across passes, which sits awkwardly against the rule that
    Datum keeps no state it later depends on. The distinction is probably that
    such history is diagnostic output, not an input to the comparison, so losing
    it degrades reporting without changing behaviour, but that has not been
    specified.

## Why conflicts are not resolved automatically

Every resolution rule that could be applied here is worse than failing.

Ordering by layer precedence would silently pick one of two resources that both
look correct in isolation. Merging them is not meaningful, because two `File`
resources with different content have no sensible union. Applying both in a defined
order produces a host that matches one resource and not the other, while reporting
success for both.

Failing costs a resolution step for whoever introduced the overlap. Not failing
costs a host whose state contradicts its own manifest, applied as root, on every
machine both resources match.
