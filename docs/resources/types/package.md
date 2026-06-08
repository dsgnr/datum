# Package

`Package` describes whether a package is installed, and optionally which version.

```yaml
datum: v1alpha1
type: Package

name: nginx

desired:
  state: present
```

Target identity is the package name, taken from `name`. Providers are `apt`,
`dnf`, `apk` and `pacman`, selected from the host, never named in the document.

## Fields

| Field | Type | Required | Meaning |
| ----- | ---- | -------- | ------- |
| `state` | `present`, `absent` | Yes | Whether the package should be installed. |
| `version` | string | No | An exact version the package should be at. |

## Observation

| Field | Reported |
| ----- | -------- |
| `exists` | Whether the package is installed. |
| `version` | The installed version, as the package manager reports it. |

## Actions

`create` installs the package. `update` changes it to the requested version.
`remove` uninstalls it. There is no action that upgrades a package without a version
having been asked for.

## Versions

Omitting `version` means Datum has no opinion about which version is installed. The
package is installed if missing and otherwise left alone, so a system that upgrades
packages by other means does not fight with Datum.

Setting `version` makes it part of desired state, which means a package at a
different version is drift and gets corrected in whichever direction is required,
including downwards.

```yaml

desired:
  state: present
  version: "1.24.0-2"
```

Version strings are provider-specific and are not translated. `1.24.0-2` is a Debian
version and means nothing to `dnf`, so a layer pinning a version is usually a layer
whose matcher narrows to hosts of one distribution.

!!! note "Important limitation"

    Pinning a version does not prevent something else from upgrading the package
    between passes. It means Datum will notice and put it back, which is a slower
    and noisier mechanism than a package manager hold. Whether `Package` should also
    manage holds, and how that maps across four package managers where the mechanism
    differs and sometimes needs an extra plugin, is unresolved.

## There is no `latest`

A `state: latest` was considered and rejected, because it is not a state.

Desired state has to be a fixed description that two observers can agree on. What
"latest" refers to depends on the contents of a remote repository at the moment a
pass runs, so two hosts reconciling the same revision an hour apart can correctly
converge on different versions, and a plan built at one moment can be wrong by the
time it is applied.

It also breaks the useful meaning of an empty plan. A host declaring `latest` cannot
be described as converged, only as having been up to date recently.

Keeping packages current is a real requirement and the answer is a version bump in
the repository, which makes the change reviewable, reportable, and identical on every
host that receives it.

## Provider differences

The differences that matter here are not cosmetic.

Version string formats are unrelated between providers, and there is no common
grammar to compare them in. Whether the package manager can report the exact file
list for an installed package varies, which matters for any future work that wants to
detect a file managed by both a package and a `File` resource. Some providers install
recommended or weak dependencies by default and others do not, so the same
`state: present` produces different sets of packages on different distributions.

None of those are hidden behind a translation layer. The support matrix records
what each provider does rather than presenting them as equivalent.

## Open questions

Whether repository configuration belongs in the resource model at all is undecided.
Installing a package that is not in the distribution's default repositories requires
a repository definition, and expressing that means either a new resource type per
packaging system, which breaks distribution neutrality, or a `File` resource writing
a sources list, which works and pushes the distribution difference into the fleet
configuration.

Whether removing a package should also remove its unused dependencies is
undecided, and the honest answer may be that Datum should not, because the set
of packages that becomes removable depends on everything else installed and not
on anything in the manifest.
