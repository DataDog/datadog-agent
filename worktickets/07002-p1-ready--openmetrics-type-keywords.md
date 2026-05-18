## Summary

The Go scraper aborts the entire scrape with
`scrape: invalid metric type "<keyword>"` when it encounters any OpenMetrics
1.0.0–only TYPE keyword: `stateset`, `gaugehistogram`, or `info`. Python's
implementation treats unknown types as gauge/untyped and continues.

Modern OpenMetrics-compliant endpoints (kube-state-metrics 2.x, several
node_exporter collectors, anything using
`prometheus_client.metrics_core.StateSetMetricFamily` or `InfoMetricFamily`)
emit these types regularly. A Go-side scrape against such an endpoint
returns zero metrics until the operator notices.

## Context

The Prometheus text format 0.0.4 defines five TYPE keywords:
`counter`, `gauge`, `histogram`, `summary`, `untyped`. OpenMetrics 1.0.0
adds three more: `stateset`, `gaugehistogram`, `info`. The Go scraper
currently treats anything outside the Prometheus-5 set as fatal; Python
treats it as gauge/untyped and proceeds.

## Repro

Mutate any line in `pkg/collector/corechecks/openmetrics/testdata/upstream_benchmarks/ksm.txt.gz`
from `# TYPE foo gauge` to `# TYPE foo stateset` (or `info` or
`gaugehistogram`), serve it, and run the Go scraper. It returns:

```
scrape: invalid metric type "stateset"
```

Python returns 743 of the 744 metrics (the one whose TYPE we changed is
mapped to gauge).

The differential harness has this case in
`adversarial.go::valueRenderingCases` indirectly; the most direct repro is:

```bash
echo '# TYPE foo stateset
foo{a="x"} 1
' | curl -sS -X POST --data-binary @- ...   # or just hand a payload to the harness
```

Or simulate via the mutator:

```bash
go test -tags openmetrics_differential -v \
    -run TestOpenMetricsMutation -mutation.seed=1 -mutation.iters=50 \
    ./pkg/collector/corechecks/openmetrics/differential/
```

The output dumps regressions to `testdata/regressions/<sha>.prom` with
`.meta` sidecar showing `go_err: scrape: invalid metric type "..."`.

## Suggested fix

In the TYPE-line parser, accept any of the OpenMetrics 1.0.0 keywords
(`stateset`, `gaugehistogram`, `info`) and any unknown keyword.

Mapping suggestions (mirroring Python's behavior):

| TYPE keyword | Treat as | Submission |
|---|---|---|
| `stateset` | gauge | one gauge per state, value 0 or 1 |
| `gaugehistogram` | histogram | same buckets/le semantics, but values are gauge not counter |
| `info` | gauge | always-1 gauge with the labels as tags |
| `<unknown>` | untyped | submit as gauge with no special semantics |

The simplest landing point is "accept and map to untyped/gauge" — full
OpenMetrics 1.0.0 semantic fidelity for `stateset` / `info` /
`gaugehistogram` can come later. The harness already has
`format/openmetrics_unit_directive` passing (we accept `# UNIT`), so
adding the new TYPE keywords is the analogous change.

## Severity rationale

P1 because:
- It's narrow and well-understood.
- It affects any OpenMetrics 1.0.0-compliant endpoint, which is most
  modern exporters.
- It can be fixed independently of the broader per-line-recovery work
  (P0 ticket), making it a small, mergeable PR.

If P0 lands first this becomes a no-op, but P1 lands faster.

## Verification

After fix, the following should pass:

```
TestOpenMetricsMutation  (with seed where a TYPE-mutation hits an info/stateset/gaugehistogram replacement)
```

And the `openmetrics_type_keyword` entry in `known_divergences.go`
should be removable.
