## Reopened: Lading-generated OpenMetrics interaction

Reopened after replacing the config-axis HTTP fixture with Lading's generated
OpenMetrics HTTP blackhole (Lading commit
`576129489df44ae0e71c4871c51910837c919b90`). The richer native OpenMetrics
payload exposes one interaction that the KSM/MSK fixtures did not.

Both knobs agree independently across a 500-iteration, one-knob seed-42 sweep.
The following combination diverges:

```yaml
namespace: diff
metrics: [".+"]
raw_metric_prefix: diff_lading_
share_labels:
  diff_lading_target_info:
    match: [service, region]
    labels: [shard]
```

Focused composition evidence (seed 42, 100 configs, 4 knobs/config):

```text
agree=97 divergent=3
```

All three divergent configs include both `raw_metric_prefix` and
`share_labels`; other knobs vary. Go and Python each emit 51 submissions, but
49 are only in Go and 49 only in Python. This points to operation ordering or
source-family naming during the shared-label prepass rather than dropped data.

Next step: minimize to the two knobs above and compare whether each
implementation applies `raw_metric_prefix` before or after locating the
`share_labels` source metric.

---

## Previous resolution

Previously closed after correcting the config-axis harness and rerunning its
KSM/MSK verification sweep.

The previously reported transformer-knob correlations were downstream of an
invalid `share_labels` generator, not evidence of independent transformer bugs.
The generator inverted source/target and join-label semantics and used KSM names
against the MSK fixture. After fixing those issues and making all generated
names fixture-specific, the original seed-42 sweep produced:

```text
1000 configs/fixture × 2 fixtures × 4 knobs/config
agree=2000
```

The focused valid `share_labels` cases and the corrected stateful cache test
also agree. No independent divergence remains for `rename_labels`,
`type_overrides`, distribution-bucket settings, health service checks, or
metric matching in this sweep.

---

## Summary

The config-variation differential harness initially flagged several
transformer-configuration knobs as appearing in divergent configs. A larger
sweep (seed=42, 1000 iters × 4 knobs/config) clarified the picture: **none
of these knobs cause divergence on their own. They are passengers — they
appear in divergent configs only when share_labels is also present.**

From the 1000-iteration sweep: 25 divergent configs total, and all 25
include share_labels. Zero non-share_labels divergent configs found.

This ticket is kept open as a verification-after-fix workstream:
once 07007 (share_labels) is fixed, re-run the harness with
`-config.knobs=1` and a large iteration count to confirm no
independent transformer-knob bugs remain.

## Knob-frequency table from the 1000-iter sweep

All counts are "this knob appeared in N divergent iterations." Every
divergent iteration also had share_labels.

```
join/share_labels                       divergent=26  (1 iter applied it twice)
labels/tag_by_endpoint_false            divergent=8
labels/exclude                          divergent=6
matching/mixed_regex_and_rename         divergent=6
transformer/non_cumulative_buckets      divergent=6
transformer/raw_metric_prefix           divergent=6
exclude/by_name                         divergent=5
transformer/histogram_as_distributions  divergent=5
transformer/send_histograms_buckets_false divergent=4
transformer/send_monotonic_with_gauge   divergent=4
exclude/by_label                        divergent=3
exclude/ignore_metrics_alias            divergent=3
health/disable_service_check            divergent=3
transformer/type_overrides              divergent=3
transformer/send_distribution_buckets   divergent=2
transformer/send_monotonic_counter_false divergent=2
matching/narrow_regex                   divergent=2
matching/rename_map                     divergent=2
labels/ignore_tags                      divergent=1
labels/include                          divergent=1
labels/rename                           divergent=1
matching/named_list                     divergent=1
```

The non-share_labels knobs appear roughly in proportion to their random
sampling weight — consistent with the "passenger" hypothesis.

## Repro

Same harness as 07007:

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/ \
    -config.iters=200 -config.knobs=3 -config.seed=1
```

Once 07007 (share_labels) is fixed, re-run with a higher iter count to
see the secondary knobs in isolation. Or run with `-config.knobs=1` to
exercise each knob alone.

## Knobs to investigate (in priority order)

### labels/rename — apply rename_labels: {src: dst}

3/6 cases involve label rename. Possibilities:

- Order of operations: rename happens before vs after share_labels join,
  affecting which label name the join sees.
- Multi-rename: the harness generates 1-3 rename pairs per iteration;
  cases of `{a: b, b: c}` may be applied differently (chain vs
  parallel).
- Reserved labels: behavior with rename target = a reserved label
  (e.g. `__name__`) may differ.

### transformer/type_overrides — type_overrides: {metric: type}

2/6 cases. Worth checking:

- Whether type_overrides correctly maps `counter` → `monotonic_count`
  (with `.count` suffix) on both sides.
- Whether `gauge` override on a metric originally typed `counter`
  produces matching submissions.
- Untyped → counter via type_overrides: Python's behavior is known;
  Go's may diverge in the suffix transformation.

### transformer/send_distribution_buckets — bool

2/6 cases. This enables submitting histogram buckets as Datadog
distributions. The transformation involves both `_bucket{le="..."}`
samples and `_count`/`_sum`, and the bucket-to-distribution mapping is
non-trivial. Likely root cause: monotonic vs cumulative bucket
interpretation.

### transformer/histogram_as_distributions — bool

1/6 cases. Related to send_distribution_buckets but flips the histogram
output type. Probably the same root cause area.

### health/disable_service_check

2/6 cases. Surprising — disabling the health service check shouldn't
affect metric submissions. The correlation may be incidental (it
happened to be paired with share_labels in those iterations). Worth
verifying with `-config.knobs=1`.

### matching/rename_map and matching/mixed_regex_and_rename

1 each. Likely the same underlying interaction as labels/rename — the
matching layer applies metric-name renames, and the order of
{name-match, name-rename, label-rename, share-labels-join} matters.

## Approach

This ticket is mostly verification-after-fix. The plan:

1. Wait for 07007 (share_labels) to be fixed.
2. Re-run the harness with the same seed:
   ```bash
   go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
       ./pkg/collector/corechecks/openmetrics/differential/ \
       -config.iters=1000 -config.knobs=4 -config.seed=42
   ```
   Expected: divergent count drops dramatically (probably to 0 or near-0).
3. If divergences remain, run with `-config.knobs=1` for isolated
   attribution — this exercises one knob at a time so any remaining
   divergence attributes cleanly.
4. For any genuinely independent knob bug found in step 3, file a
   focused follow-up ticket.
5. Close this ticket when the harness produces zero non-suppressed
   divergent iterations across multiple seeds.

If step 2 shows zero divergent iterations, the entire P2 surface
convergently resolves with the P1 fix — close this ticket and update
07007's verification to include the broader sweep.

## Severity rationale

P2 because:
- No knob has been demonstrated to cause divergence independently of
  share_labels.
- All evidence points to share_labels being the underlying broken feature;
  these knobs are observably passengers.
- Real-world incidence: irrelevant until 07007 is fixed.

Keep as ready (not blocked) so the verification work has a tracking home,
but expect this ticket to close fast once 07007 lands.

## Verification

After all the contributing knobs are fixed, this should produce zero
non-suppressed divergent iterations:

```bash
go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
    ./pkg/collector/corechecks/openmetrics/differential/ \
    -config.iters=500 -config.knobs=4 -config.seed=1
```

The knob attribution log at the end should show no divergent counts
for any knob.

## Note on harness budget

100 iterations × 3 knobs × 2 fixtures finds 6 divergences. Bigger runs
(say 500 iterations) would surface ~30 — likely the same handful of
underlying bugs amplified by the random distribution. The harness is
producing repeated repros of a small set of real bugs, not novel
findings per iteration.
