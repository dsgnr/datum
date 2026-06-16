# Dependencies

Resources declare what has to be settled before them. Ordering is never implied by
where a document sits in a file or by the order files appear on disk.

```yaml
apiVersion: datum.dev/v1alpha1
kind: Service

metadata:
  name: nginx

dependsOn:
  - Package[nginx]
  - File[nginx-config]

spec:
  state: running
  enabled: true
  restartOn:
    - File[nginx-config]
```

`dependsOn` sits alongside `spec` rather than inside it, because ordering is
behaviour common to every resource type rather than something a type defines.

## What dependsOn means

Three things, precisely.

The referenced resources are processed before this one, so `Package[nginx]` is
applied, or determined to need nothing, before `Service[nginx]` is considered.

If a referenced resource fails to apply, this resource is not attempted and its
action becomes `skip`. Failure propagates along dependency edges, so a resource
depending on something that was skipped is also skipped.

The reference must resolve to a resource in the same effective manifest. A
dependency on something not present is an error raised by the graph builder
before the host is read, rather than a dependency silently treated as satisfied.

What it does not mean is a requirement that the target exists on the host.
`dependsOn: [Package[nginx]]` combined with `Package[nginx]` declaring
`state: absent` is a coherent, if unusual, manifest, and Datum orders the two
without objecting.

## Ordering is not enough on its own

Ordering says a file is written before a service is considered. It does not say the
service should restart because the file changed.

```text
Package[nginx]
      │
      ▼
File[nginx-config]
      │
      ▼
Service[nginx]
```

On a converged host where only the file content changed in the repository, ordering
alone produces an `update` on the file and `none` on the service, and nginx carries
on serving the old configuration until something restarts it. That is why change
reaction is a separate mechanism.

## restartOn

`Service.spec.restartOn` lists resources whose change should cause the service to
restart.

```yaml
spec:
  state: running
  enabled: true
  restartOn:
    - File[nginx-config]
```

When a listed resource has a non-`none` action in the same plan, the service gets an
`update` whose reason records which resource triggered it.

```text
update   Service[nginx]
         reason   File[nginx-config] changed, restartOn matched
```

Declaring the reaction on the service instead of on the file is deliberate.
Reading `Service[nginx]` shows everything that can restart it, whereas
notification declared on files means reconstructing the list by searching for
anything that mentions the service.

A resource named in `restartOn` does not become a dependency automatically, and both
are usually wanted. Listing a file in `restartOn` without also listing it in
`dependsOn` would allow the restart to be planned before the file was written.

!!! note "Open question"

    Reacting to change is currently specific to `Service`, which is the only type
    with an obvious reaction. Whether a general mechanism is needed, and what it
    would mean for a type whose reaction is not "restart", is undecided. Adding one
    prematurely risks a generic trigger system that ends up being used to sequence
    arbitrary work, which is the direction this design is trying to avoid.

    There is also no way to ask for a reload rather than a restart, which matters
    for services where a restart drops connections. Whether that is a field on
    `restartOn` or a property the provider decides has not been worked out.

## Where dependencies come from

Most dependencies are obvious from the resources themselves. A file in a directory
depends on the directory, a file owned by a user depends on the user, a service
depends on the package providing its unit.

Datum does not infer any of them. A `File` at `/etc/nginx/conf.d/tls.conf` is not
automatically ordered after a `Directory` at `/etc/nginx/conf.d`, even though the
relationship is plain from the paths.

Inference was rejected because it is either incomplete or surprising. Path
nesting would cover directories and not users, adding ownership inference would
cover users and not packages, and each rule added makes the plan order depend on
knowledge that is not in the repository. An engineer reading a manifest would
then have to know Datum's inference rules to predict the order, which is a cost
the design declines to pay.

The consequence is that dependencies are the most common thing to forget, and the
failures are the confusing kind where a resource works on a converged host and fails
on a fresh one.

!!! note "Open question"

    Whether Datum should detect probable missing dependencies and warn, without
    acting on them, is still open. A warning that a `File` under a managed
    `Directory` has no dependency on it would catch the common mistake while
    keeping the plan order fully determined by what the repository says.

## Cycles

A cycle is rejected. The graph builder detects it and the pass ends before the host
is read.

```text
error: dependency cycle

  Service[nginx] -> File[nginx-config] -> Package[nginx] -> Service[nginx]
```

There is no attempt to break a cycle by dropping an edge, because the choice of
which edge to drop would be arbitrary and the resulting order would be one nobody
asked for.
