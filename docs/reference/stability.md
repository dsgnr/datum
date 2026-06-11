# Interfaces and stability

Datum will expose several interfaces, and they will not all become stable at the same time. Deciding
which ones are contracts, and saying so, has to happen before anyone automates against something
that was never meant to be relied upon.

## The interfaces

| Interface | What depends on it |
| --------- | ------------------ |
| Document schema | Every repository. The `datum: v1alpha1` documents. |
| CLI surface | Interactive use and scripts. Command names, flags, arguments. |
| Exit codes | Scripts and scheduled jobs that branch on the result. |
| JSON output | Tools that parse `datum status` and other structured output. |
| Provider interface | Anyone writing a provider. |
| Status schema | Dashboards and monitoring, whether read as text or JSON. |

## Everything is unstable before 1.0

Datum is pre-1.0, and until 1.0 every interface above may change without a migration path. The
`v1alpha1` schema version says this for the document format, and the same applies to the others.

Saying it plainly matters because the alternative is an interface that becomes a contract by
accident. The first time a scheduled job branches on an exit code, or a dashboard parses the status
JSON, that shape is load-bearing whether or not anyone decided it should be, and breaking it later is
expensive regardless of what the documentation claimed.

## Which interfaces become contracts, and when

The interfaces do not all deserve the same stability, and ranking them now guides where care is spent.

The document schema is the highest-value contract, because it is what every repository is written
against, and a breaking change rewrites everyone's configuration. It is the last thing that should
stabilise, precisely because it is the most expensive to get wrong, and `v1alpha1` exists to buy the
freedom to change it while the model is still moving.

Exit codes are the cheapest contract to honour and the easiest to break by accident, so they are
stabilised early. A [documented set](cli.md#exit-codes) of `0`, `1` and `2` is small enough to
commit to, and scripts depend on it the moment `datum diff` runs anywhere unattended.

JSON output is the interface most likely to be automated against without anyone noticing it became a
contract. It should be explicitly versioned when it stabilises, so that a tool can state which
schema it expects and a breaking change is a visible version bump rather than a silent shift in
field names.

The provider interface becomes a contract as soon as providers are written outside the main
repository, and until then it can change freely. The [multi-distribution](../providers/multi-distribution.md)
work is what will exercise it, and stabilising it before a second and third provider exist would be
premature.

## The rule the design commits to

The interfaces are not stable now, and each one that stabilises will say so explicitly and carry a
version where a version is meaningful. Nothing becomes a contract by having existed for a while.

Until then, the honest statement is the one each page already carries, which marks what is
implemented and what is still a proposed shape. The reader who automates against a proposed shape is
choosing to track a moving target, and the documentation is clear that it is moving.
