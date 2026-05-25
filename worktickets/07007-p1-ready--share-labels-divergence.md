## Summary

The Go scraper's implementation of `share_labels` (the label-join transformer)
diverges from Python's in essentially every test scenario. Every divergent
config found by the config-variation harness involves `share_labels` (6/6,
100% correlation). When share_labels is the only join-related knob set, both
implementations produce the same submission count but disagree on tag sets
for hundreds of the submissions.

## Context

`share_labels` is the OpenMetrics check's label-join mechanism. From the
Python check's schema:

```yaml
share_labels:
  <target_metric_name>:
    labels: [<label1>, <label2>]    # labels to copy
    match:  [<source_metric_regex>] # source metrics whose labels we read
```

Semantically: for each sample of `target_metric_name`, find a sample of a
`match`ed source metric with matching join keys, and copy the named labels
from source to target.

Both Go and Python advertise support; both accept the same YAML shape
without complaint. But they emit different output for the same input.

## Repro

The cleanest minimized repro (9 submissions on each side, 8 diffs):

Config:
```yaml
namespace: diff
metrics:
  - kube_deployment_status_replicas_available: renamed.kube_deployment_status_replicas_available
  - kube_node_info: renamed.kube_node_info
send_distribution_buckets: true
share_labels:
  kube_deployment_labels:
    labels: [deployment, deployment]
    match: [kube_pod_status_phase]
```

Payload: `pkg/collector/corechecks/openmetrics/testdata/upstream_benchmarks/ksm.txt.gz`

Result: `go=9 py=9 diffs=8`, breakdown `only_in_go=7 only_in_python=1`. Both
sides agree on 1 submission and disagree on the other 8.

Full repros in `pkg/collector/corechecks/openmetrics/differential/testdata/regressions/`
(gitignored; regenerate with the command below). Specifically the
`config_*.instance.json` + `config_*.prom` + `config_*.meta` triples.

Regenerate:

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/ \
    -config.iters=100 -config.knobs=3 -config.seed=1
```

## What's diverging

Without instrumenting the implementations, the observable shape is:

- Both impls produce the same submission *count* (when share_labels is
  the dominant divergent knob).
- The tag sets attached to each submission differ. Specifically, the
  "joined" labels from the source metric appear differently on Go vs
  Python — either attached to a different subset of target samples, or
  attached with different values, or with different join-key semantics.

Possible root causes (Go scraper-side investigation needed):

1. **Match semantics**: Python's `match` may be a regex while Go's may be
   exact-name (or vice versa).
2. **Join key**: which labels are used to match source-to-target samples
   may differ.
3. **Cache vs streaming**: Python may build a full source-label cache
   before processing targets; Go may stream and only join against
   already-seen sources.
4. **Apply order**: rename_labels + share_labels may be applied in
   different orders, so the same label name resolves to different
   values between the two paths.

The cleanest experiment is to compare the simplest non-trivial case:
one target metric, one match metric, one label to copy, payload with
just those two metrics. If a discrepancy reproduces there, the cause
is fundamental; if not, the cause is in interaction with other knobs.

## Severity rationale

P1 because:
- The harness shows the divergence with realistic configs (just `share_labels`
  plus standard matching/renaming, nothing exotic).
- Production users have `share_labels` configured today; a migration to the
  Go scraper would silently reshape their submission tag sets.
- Some downstream Datadog products (dashboards, monitors) key off specific
  tag combinations; this is exactly the kind of silent data drift that
  ships incidents.

Not P0 because:
- The submission *counts* match, so volume monitoring is unaffected.
- Recovery via rollback is trivial once detected.

## Empirical evidence: share_labels is necessary AND sufficient (with caveat)

A larger sweep (seed=42, 1000 iters × 4 knobs/config) confirms the pattern:

- 1000 iterations, 25 divergent configs.
- **All 25 divergent configs include share_labels in their knob set.**
- **Zero divergent configs without share_labels.**

Probability analysis: with 23 knobs and 4-with-replacement picks per
iteration, ~16.5% of iterations should include share_labels (≈165 of 1000).
Of those, 25 (~15%) diverged. Of the ~835 iterations without share_labels,
zero diverged.

This means the other knobs that appear in divergent configs (rename_labels,
type_overrides, raw_metric_prefix, non_cumulative_buckets, exclude_*,
tag_by_endpoint, ...) are passengers — they correlate with divergence ONLY
in combination with share_labels. They modulate which submissions diverge
but do not cause divergence themselves. Fix share_labels and most/all of
the other observed divergences should resolve.

Caveat: stateful testing (`TestStatefulShareLabelsAcrossScrapes`) shows
that a **minimal** share_labels config (one target, one match, one label
to copy, two-metric payload) produces *agreement* on both single and
double scrapes. So share_labels alone is NOT broken — the bug requires
share_labels to be combined with at least one other config knob that
touches the same submissions (rename, exclude, raw_prefix, etc.). This
refines the investigation: the root cause is more likely an *ordering* or
*composition* bug between the share_labels pass and another pipeline
pass, not share_labels itself.

One caveat to the caveat: this conclusion is bounded by the harness
coverage. Pure non-share_labels divergences may exist in knob combinations
the random sampling hasn't hit yet. Re-run with `-config.knobs=1` after
fixing share_labels to verify nothing remains.

## Verification

A successful fix should produce `agree` for these repro configs and reduce
the `join/share_labels` divergence count to 0 in:

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/ \
    -config.iters=200 -config.knobs=3 -config.seed=1
```

## Out of scope

`labels/rename` and `transformer/type_overrides` divergences are tracked
in separate tickets; they likely share root causes with share_labels but
the cleanest investigation starts here.
