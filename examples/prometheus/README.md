# Prometheus configuration

Datum aggregates nothing. Each agent serves its own metrics and no component sees more than one
machine, so answering a fleet-wide question means an existing monitoring system scraping every host.
These files are that arrangement for Prometheus, taken from
[alerting](https://getdatum.sh/observability/alerting/).

| File | Contents |
| ---- | -------- |
| `prometheus.yml` | A `datum` scrape job and the rule file |
| `datum.rules.yml` | The documented alerts as rules |
| `datum.rules.test.yml` | `promtool` tests asserting each alert fires |

```bash
make test-prometheus
```

That runs `promtool check` over both files and the tests over the rules, in a container, so nothing
has to be installed to use them. The tests matter more than the syntax check, because a rule can
parse perfectly and still match no series, which is what happened to the fleet-lag alert before
`scalar()` was added to it.

The scrape job has to be called `datum` for `up{job="datum"} == 0` to work. That alert is the only
one a stopped agent cannot raise about itself, so it is the one worth getting right. An agent's
listener defaults to `127.0.0.1:10056` and answers a remote scrape only once
[`metrics.listen`](https://getdatum.sh/reference/agent-config/) names an address the
collector can reach. A fleet already running `node_exporter` can set `metrics.textfile` instead and
leave the listener off, in which case the series arrive under the `node_exporter` job and the `up`
alert is written against that job.
