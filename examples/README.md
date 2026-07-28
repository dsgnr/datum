# Example fleet

A small repository to run the commands against. Three hosts, seven layers, and
enough overlap between them to show what composition does.

```bash
make build

./bin/datum validate --repo examples/fleet
./bin/datum render --host web-001 --repo examples/fleet
./bin/datum explain 'File[nginx-config]' --host web-001 --repo examples/fleet
```

## What is in it

| Host | Labels | Gets |
| ---- | ------ | ---- |
| `web-001` | production, london, web | Everything, plus a host override tightening one file mode |
| `web-002` | staging, london, web | The same role, without the production kernel tuning |
| `db-001` | production, london, database | The database role instead of the web role |

The layers follow the conventional precedence values.

```text
base                      0    every host
environments/production  10    production hosts
environments/staging     10    staging hosts
sites/london             20    hosts in London
roles/web                30    web servers
roles/database           30    database servers
hosts/web-001           100    one host
```

## Things worth looking at

**A value being overridden.** `base` sets `net.ipv4.ip_forward` to `0` and
`environments/production` sets it to `1`. Compare `web-001` with `web-002`, which
is in staging and therefore keeps the base value.

```bash
./bin/datum explain 'Sysctl[net.ipv4.ip_forward]' --host web-001 --repo examples/fleet
./bin/datum explain 'Sysctl[net.ipv4.ip_forward]' --host web-002 --repo examples/fleet
```

**A host override.** `roles/web` sets `File[nginx-config]` to mode `0640` and
`hosts/web-001` narrows it to `0600`. A host override is an ordinary layer with a
matcher for one host, so nothing about it is special.

**Substitution.** `base/motd.yaml` puts `{{ host }}` and `{{ labels.site }}` into a
file's content, and `roles/web/files/site.conf.tmpl` is rendered rather than copied.
`datum explain` reports which labels a resource consumed.

```bash
./bin/datum explain 'File[motd]' --host web-001 --repo examples/fleet
```

**Ordering.** `Service[nginx]` declares `reloadOn` for its two configuration files,
which orders them before the service and asks for a reload rather than a restart
when either changes.

## Trying a change

`datum affected` compares two revisions, so it needs the change committed.

```bash
cd examples/fleet
git init && git add -A && git commit -m "example fleet"

# Tighten a mode and see who it reaches.
sed -i '' 's/mode: "0640"/mode: "0600"/' roles/web/nginx.yaml
git commit -am "tighten nginx config mode"

cd ../..
./bin/datum affected --from HEAD~1 --to HEAD --repo examples/fleet
```

Both web hosts are affected and `db-001` is not, because the role matcher does not
select it.

## Breaking it on purpose

Every error path is worth seeing once.

```bash
# An unknown field.
echo '  contnet: hello' >> examples/fleet/base/motd.yaml
./bin/datum validate --repo examples/fleet

# A mode that grants privilege without saying so.
# A setuid bit needs desired.allowPrivileged: true.

# Two layers at equal precedence disagreeing about one field is a conflict, and
# resolution fails rather than picking a winner.
```

Undo with `git checkout examples/fleet` if the example is committed, or rerun the
commands after putting the file back.
