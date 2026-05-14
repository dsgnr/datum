# Service

`Service` describes whether a service is running now and whether it starts at boot.

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

Target identity is the unit name, taken from `metadata.name`. The only candidate
provider is `systemd`.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `state` | `running`, `stopped` | Yes | Whether the service should be running now. |
| `enabled` | boolean | Yes | Whether the service should start at boot. |
| `restartOn` | list of resource references | No | Resources whose change should restart the service. |

## Running and enabled are independent

They describe different things and neither implies the other. A service can be
running now and disabled at boot, which is a machine that will come back wrong, and
it can be enabled and stopped, which is a machine that is currently wrong and will
fix itself on restart.

Both are required, not defaulted, because every plausible default is wrong often
enough to be a trap. Making them explicit means a repository states its
intention for both, and a service that is enabled on purpose but not running
says so.

## Datum does not create units

There is no `create` action. A unit exists because a package installed it or
because a `File` resource wrote it, so a `Service` resource for a unit that does
not exist is an error at apply time and not something to be created.

The usual cause is a missing `dependsOn` on the package providing the unit. It works
on a host where the package is already installed and fails on a fresh one, which is
the most common shape of dependency mistake.

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the unit is known to the init system. |
| `state` | `running` or `stopped`, from whether the unit is active. |
| `enabled` | Whether the unit is enabled at boot. |

Reporting `running` from the unit being active rather than from a process
existing matters, because a unit can be active while its main process is
restarting, and a process can exist while the unit has failed.

## restartOn

When a resource listed in `restartOn` has a non-`none` action in the same plan, the
service is given an `update` whose reason names the trigger.

```text
update   Service[nginx]
         reason   File[nginx-config] changed, restartOn matched
```

This happens whether or not the service's own fields differ. A service that is
already running and enabled still restarts when its configuration changes, which is
the entire purpose.

Listing a resource in `restartOn` also orders it before the service, so a file named
there does not need repeating in `dependsOn`. The example at the top of this page lists
`File[nginx-config]` in both because the intent is clearer that way, and the second
mention changes nothing.

## Verification

After applying, the unit is checked for being active and enabled as required. This is
where the difference between a command succeeding and a state being correct is most
visible, because `systemctl restart` returns successfully as soon as the unit is
started and a service that exits on a bad configuration file has usually not failed
yet at that point.

!!! note "Open question"

    Whether verification should wait, and for how long, before deciding a restarted
    service is healthy is undecided. Checking immediately reports success for a
    service that dies a second later, and waiting makes every pass slower and
    introduces a timeout nobody can choose correctly. A unit's own readiness
    notification is the better signal where it exists, and it does not exist for most
    units.

## Open questions

There is no way to ask for a reload rather than a restart, which matters for services
where a restart drops connections and where the configuration change does not need
one. Whether that belongs on `restartOn`, as a field on the service, or as something
the provider decides from the unit's capabilities has not been worked out.

Units with instances, such as `getty@tty1`, are not addressed. The name would work as
a target identity, and whether anything else about them needs modelling is unexplored.

Providers other than `systemd` are not planned. Alpine uses OpenRC by default and is
in the list of distributions the design targets, so either an OpenRC provider is
needed or Alpine support means containers and images where no init system is running
at all. That tension is unresolved.
