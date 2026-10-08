# High-severity onset and recovery F1

The default scorer evaluates **new high-severity anomaly-scorer episode starts**.
It reads `anomaly_periods[].period_start` only for patterns beginning with
`anomaly_scorer_high:`. Medium-threshold episodes, TimeCluster correlations and
raw detector anomalies are excluded; they cannot substitute for high-severity
starts. Their count is reported as `num_filtered_non_high`.

| Phase | Treatment |
| --- | --- |
| Before `baseline.start` | Ignore warmup |
| Baseline before disruption onset | Each new high-severity episode adds one FP |
| Disruption | Reward the first high-severity episode at/after onset using the existing right-sided Gaussian overlap; ignore subsequent starts |
| Early recovery | Ignore new starts; no recovery TP reward |
| Recovery quiet tail | Each new high-severity episode adds one FP |
| At/after `cooldown.end` | Ignore starts outside the scenario |

The quiet tail is the **last half of recovery**, with no five-minute cap, calculated
from `episode.json`'s `cooldown.start` and `cooldown.end`. Its start is included
and its end is excluded. With integer-second episode timestamps, an odd
recovery duration rounds the tail start up to the next second.

An episode already open before the quiet tail is not penalized for overlapping
it: `period_end` is deliberately irrelevant. A first detection during recovery
cannot earn onset credit, even with a large `--sigma`. Missing the disruption
therefore still gives FN = 1.

For one disruption, onset overlap `w` gives TP = `w`, FN = `1 - w` (or TP = 0,
FN = 1 if missed). FP is the sum of baseline and recovery-tail episode starts.
Precision, recall and F1 use those totals. `alpha` remains the baseline-only
false-positive rate; recovery does not change its denominator.

## Running locally

```sh
dda inv anomalydetection.build-scorer
bin/anomalydetection-scorer --input /tmp/observer-eval-example.json \
  --scenarios-dir comp/anomalydetection/observer/scenarios --sigma 30 --json
dda inv test --module=internal/qbranch/anomalydetection-scorer --targets=.
```

Replay with `anomaly_scorer` enabled, `correlation_events: true`,
`correlation_event_threshold: high` and `cooldown_secs: 0`. Generated local
combination/Optuna configurations fix these output settings; numeric scorer
thresholds remain tunable. `eval-scenarios --only` automatically includes the
anomaly scorer. Medium-threshold output does not preserve high-transition
timestamps and must be replayed with the high threshold for this evaluation.

Scenario scoring requires valid cooldown metadata and reports the quiet-window
boundaries, baseline/recovery FP counts, and ignored-period counts. An explicit
`--ground-truth-ts` overrides onset but does not bypass invalid phase metadata.
For standalone timestamp-only use, pass `--scenarios-dir ''` together with
`--ground-truth-ts`; recovery is then explicitly unavailable, not scored.

Results identify this contract as `high-severity-onset-recovery-v2`. Local
Bayesian report reuse includes that version so old onset-only results are not
reused, nor are results from the earlier five-minute-capped recovery rule.
Historical outputs without high-severity episode patterns need a fresh
replay. The separate `--score-tp` metric-ground-truth mode is unchanged.

The quiet-tail rule is an evaluation requirement, not proof that the recording
is healthy. It does not validate telemetry coverage or relabel recorded late
changes. Inspect incomplete recordings and real late deterioration separately;
the window is never shortened to make an incomplete recording appear quiet.
