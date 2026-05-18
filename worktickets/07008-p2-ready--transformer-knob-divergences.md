## Summary

The config-variation differential harness found that several transformer-
configuration knobs produce divergent output between Go and Python even
when `share_labels` (the dominant cause) is excluded. These are smaller
silent-data-drift bugs that compound during migration: each one shifts a
small fraction of submissions but in different shapes.

Knob attribution from the first 100-iteration config run (knobs are sorted
by divergence count; each iteration applied 3 random knobs, so any single
knob can be "blamed" for multiple iterations):

```
join/share_labels                       divergent=6   ← tracked separately
labels/rename                           divergent=3
transformer/type_overrides              divergent=2
transformer/send_distribution_buckets   divergent=2
health/disable_service_check            divergent=2
transformer/histogram_as_distributions  divergent=1
matching/rename_map                     divergent=1
matching/mixed_regex_and_rename         divergent=1
```

Since every divergent iteration also had `share_labels`, the standalone
contribution of these knobs isn't separable from share_labels' effect.
Bisecting that out is part of this ticket's investigation.

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

1. Fix share_labels first (07007). Most of the noise here will vanish.
2. Re-run the harness with the same seed to see what remains.
3. For each remaining knob, run with `-config.knobs=1` for isolated
   attribution. The harness will report `divergent=N` per knob.
4. Pick the highest-correlation knob; minimize a repro (the harness
   already minimizes by knob count when knobs=1).
5. Write a focused unit test in `pkg/collector/corechecks/openmetrics/`
   for the specific transformer behavior, fix the Go side to match
   Python, verify both the unit test and the differential test pass.

## Severity rationale

P2 because:
- Each is partial-degradation, not total failure.
- All are tracked behind 07007 (share_labels) which dominates the
  divergence space — fixing that first will clarify these.
- Real-world incidence depends on how many users have non-default
  values for these knobs. type_overrides and send_distribution_buckets
  are common; rename_labels is moderately common; histogram_as_distributions
  is rarer.

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
