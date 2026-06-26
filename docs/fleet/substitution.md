# Substituting label values

A layer can substitute a host's own label values into the desired state it contributes, so one
document can describe hosts that differ in a single field.

!!! note "Proposed behaviour"

    The syntax below is proposed. What is decided is the boundary, recorded as
    [ADR-0012](../adr/0012-substitution-from-declared-labels.md), which is that declared label values
    may be substituted and nothing else may.

## The two forms

```text
{{ labels.site }}      the value of the host's site label
{{ host }}             the host name
```

The host name is carried as the reserved [`datum/host`](labels-and-matchers.md#reserved-labels)
label, and a `/` cannot appear in a bare name inside the braces, so `{{ host }}` provides it
directly.

Whitespace inside the braces is optional, so `{{labels.site}}` and `{{ labels.site }}` are the same
reference.

## Substituting into a field

Any string-valued field inside `desired` may contain a reference.

```yaml
datum: v1alpha1
type: File

name: app-site-config

desired:
  path: /etc/app/conf.d/{{ labels.site }}.conf
  owner: root
  group: root
  mode: "0640"
  source: files/site.conf
```

For a host labelled `site: london` that resolves to `/etc/app/conf.d/london.conf`. The [target
identity](../resources/identity.md) is the resolved path rather than the template, so hosts in
different sites manage different paths from one document. Target identity is per host, so two hosts
in the same site managing the same path is not a conflict.

Substitution happens during resolution, before the [manifest
digest](effective-manifests.md#content-addressing) is computed, so the digest covers resolved
values. Resolution renders the template, and the manifest carries only the rendered result.

## Substituting into file content

A `File` reads content from `desired.source`, which is copied verbatim. `desired.template` reads the
same kind of path and renders it.

```yaml
datum: v1alpha1
type: File

name: nginx-site

requires:
  - Package[nginx]

desired:
  path: /etc/nginx/sites-enabled/app.conf
  owner: root
  group: root
  mode: "0640"
  template: files/site.conf.tmpl
```

```text title="fleet/roles/web/files/site.conf.tmpl"
server {
    listen 443 ssl;
    server_name {{ labels.site }}.example.com;
    access_log /var/log/nginx/{{ host }}.access.log;
}
```

Rendering is selected by which field is used rather than by a flag, so it is visible on the line
that names the file. Declaring both `source` and `template` is an error.

The rendered content is [staged, validated and renamed into
place](../security/provider-safety.md#writing-a-file), so [configuration
validation](../resources/validation.md) checks the rendered result. A template that renders to
something the application rejects fails validation on the hosts where it renders that way, which may
be a subset of the hosts using it.

!!! note "Important limitation"

    A template cannot contain a literal `{{`, since there is no escape sequence. A file needing
    those characters, most often one that is itself a template for another tool, uses `source` and
    is copied verbatim.

## A missing label is an error

A reference to a label a host does not declare fails manifest validation, before the host is read.

```text
error: undefined label in substitution
  File[app-site-config].desired.path
  references labels.site
  roles/web

  affects 12 hosts, first: web-014
```

An empty substitution would produce `/etc/app/conf.d/.conf`, which is a valid path and would be
created successfully on every host missing the label. Failing at validation reports it during
review, and [the affected count](../repository/validating-changes.md#datum-validate) gives the
number of hosts a typo reaches.

`datum validate` resolves every host for this reason. A layer referencing a label that three hosts
out of a fleet do not carry produces an error only for those three, which validating documents in
isolation would not detect.

## What is not substituted

Matchers are not rendered. A `match` block determines which hosts a layer applies to, so rendering
it against a host's labels would be circular.

Layer names, resource names and the `precedence` value are not rendered. A resource reference has to be
stable for [dependencies](../resources/dependencies.md) to resolve within a manifest, and a name that
varied per host would mean `requires: [Package[nginx]]` naming different resources on different hosts.

Substituted values are not themselves rendered. A label value containing `{{ host }}` is used
literally, so rendering is a single pass.

## Seeing what a host will get

```text
$ datum explain File[nginx-site] --host web-001

File[nginx-site]   /etc/nginx/sites-enabled/app.conf

contributed by
  roles/web        precedence  30   matched role=web

fields
  path      /etc/nginx/sites-enabled/app.conf    roles/web
  template  files/site.conf.tmpl                 roles/web
  rendered  sha256:d41f8a03                      from labels.site=london, host=web-001

substitutions
  labels.site   london     hosts/web-001
  host          web-001    reserved
```

The output names the labels the rendering consumed and where each value was declared. Without it,
reviewing a template means working out the resolution by hand, which is the problem
[provenance](index.md#the-question-the-model-has-to-answer) addresses for merged fields.
