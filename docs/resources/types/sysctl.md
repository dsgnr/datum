# Sysctl

`Sysctl` describes the value of a kernel parameter, both in the running kernel and
after a reboot.

```yaml
datum: v1alpha1
type: Sysctl

name: net.ipv4.ip_forward

desired:
  value: "1"
```

Target identity is the parameter key, taken from `name`.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `value` | string | Yes | The value the parameter should hold. |
| `state` | `present`, `absent` | No, defaults to `present` | Whether Datum should manage the value. |

Values are strings even when they look like numbers, for the same reason `File`
requires a quoted mode. Several parameters take values that are neither integers
nor booleans, with `net.ipv4.tcp_rmem` holding three space-separated numbers.
Treating every value as a string gives one comparison rule for all of them.

## The state exists in two places

A kernel parameter is set in the running kernel and separately persisted so that it
survives a reboot. Both have to be correct for the resource to be satisfied, and
either can be wrong on its own.

```text
Sysctl[net.ipv4.ip_forward]
  value      1     match
  persisted  true  match
```

A value applied with `sysctl -w` and never written to a file is correct until
the next reboot. This is the most common form of sysctl drift, and it stays
invisible until the machine restarts.

## Persistence

The provider writes one file per parameter under `/etc/sysctl.d`, named from the
parameter key.

```text
/etc/sysctl.d/70-datum-net.ipv4.ip_forward.conf
```

One file per parameter keeps the resources independent. A failure writing one
parameter leaves the others alone, each resource is verified on its own, and two
parameters changing in the same pass write to separate files.

A single `70-datum.conf` would be tidier on the filesystem and would give every
`Sysctl` resource the same target, which brings every one of them into
contention.

These files belong to the provider and are not declared by a repository. A
`File` resource targeting one of them is a conflict, and detecting it requires
the provider to declare the paths it owns.

!!! note "Proposed behaviour"

    Provider-declared path ownership is proposed and not specified. Without it,
    a `File` resource writing a colliding name into `/etc/sysctl.d` and the
    `Sysctl` provider would overwrite each other on every pass, and nothing in
    the manifest would indicate why.

## Observation

| Field | Reported |
| ----- | -------- |
| `value` | The value the running kernel reports. |
| `persisted` | Whether the provider's file holds the same value. |

Observation reads the running value and the provider's own file. Other files
under `/etc/sysctl.d` are not examined. Where a lower-numbered file from a
package sets the same parameter, load order decides the running value, and
Datum's file can be overridden. The resource then reports a `value` mismatch
that correcting does not clear.

!!! note "Important limitation"

    The resource is corrected on every pass and never converges, and the cause
    cannot be seen from the resource. Reporting it would require reading every
    file in the directory and reasoning about load order, which has not been
    designed.

## Removal

`state: absent` removes the provider's file for that parameter. The running value is
left as it is.

A kernel default cannot be restored at runtime. Once the value has been changed,
the default is no longer recorded anywhere Datum can read. A parameter declared
absent stops being managed and keeps its current value until the machine
reboots.

## Provider differences

The parameter namespace is a kernel interface, so the keys and values are the
same everywhere the kernel version supports them. The set of parameters varies
with kernel version. A parameter introduced in a later kernel is absent on an
older one, which appears as a resource that works on some hosts and fails on
others.

Writing to `/etc/sysctl.d` is supported across the distributions this design
targets. A container without a writable `/proc/sys` cannot support the resource
at all, and that is reported as its own condition rather than as a permission
error.
