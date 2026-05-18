## Summary

When a metric is declared with two different TYPE keywords in the same
payload — e.g. `# TYPE m gauge` then later `# TYPE m counter` for the
same metric `m` — Go honors the **first** declaration and Python honors
the **last**. The two implementations submit the same sample under
different kinds (gauge vs `monotonic_count` with the `.count` suffix).

This produces silent data divergence: the metric still appears in
Datadog, but its type and accumulated semantics change between Go and
Python deployments.

## Context

The Prometheus text format doesn't strictly forbid multiple `# TYPE`
declarations for the same metric name (it's "should not", not "must
not"). Real-world payloads can hit this when:

- An exporter is buggy and emits TYPE for the same name twice.
- Two different metric sources are accidentally serialized to the same
  endpoint.
- A migration window where an exporter is changing its TYPE for a
  metric (and the deploy hasn't fully rolled).

Both Go and Python could defensibly choose either rule. The bug is
that **they disagree**, so a side-by-side migration produces type-shape
data drift.

## Repro

Adversarial catalog:

```bash
go test -tags openmetrics_differential -v \
    -run 'TestOpenMetricsAdversarial/name/conflicting_type' \
    ./pkg/collector/corechecks/openmetrics/differential/
```

Current output:

```
divergent  go=3 py=3 diffs=2
divergence breakdown: only_in_go=1 only_in_python=1
only_in_go:     gauge\x00diff.m\x00\x00  value=1 tags=[]
only_in_python: monotonic_count\x00diff.m.count\x00\x00  value=1 tags=[]
```

Minimal payload:

```
# TYPE m gauge
m 1
# TYPE m counter
m 2
```

Go submits `diff.m` as gauge=1. Python submits `diff.m.count` as
monotonic_count=1 (it uses the LAST TYPE, counter, and applies the
`.count` suffix transformation).

## Suggested fix

Match Python's last-write-wins semantics. The OpenMetrics spec says
each metric family should have one TYPE; when duplicates appear, the
"reasonable" behavior is take the later declaration as the operator's
latest intent.

Implementation: in the TYPE-line parser, replace any existing TYPE for
the metric name instead of keeping the first.

This is a small change in the parser's metadata-tracking pass.

## Severity rationale

P2 because:
- The metric still gets submitted on both sides — partial degradation,
  not total failure.
- Real-world incidence is probably low (most exporters get TYPE right).
- But silent data drift during migration is worth fixing — operators
  comparing pre- and post-migration dashboards will see metrics
  reshape.

## Verification

After fix:

```
TestOpenMetricsAdversarial/name/conflicting_type
```

should produce `verdict: agree` with both impls submitting
`monotonic_count diff.m.count`.

## Note

There's a defensible argument for the opposite fix (Python adopts
Go's first-wins). If that becomes the chosen direction, the adversarial
case should still be updated to reflect agreement, and the divergence
documented as resolved.
