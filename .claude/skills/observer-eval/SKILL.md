---
name: observer-eval
description: >
  Running and scoring observer anomaly detection evaluations. Use this skill
  when the user wants to run eval scenarios, score detection results, interpret
  F1/precision/recall metrics, compare detector configurations, manage parquet
  files, or debug testbench replay divergences. Trigger on mentions of "eval",
  "scoring", "F1 score", "testbench", "parquet", "ground truth", "scenario",
  "q.eval-scenarios", "q.eval-tp", anomaly detection quality, or detector
  comparison — even if the user doesn't use these exact terms.
---

# Observer Evaluation Guide

## Running evals

```bash
# All scenarios with default detectors (bocpd, rrcf, time_cluster)
dda inv q.eval-scenarios

# Single scenario
dda inv q.eval-scenarios --scenario=food_delivery_redis

# Specific detectors
dda inv q.eval-scenarios --scenario=food_delivery_redis --only bocpd,rrcf,scanwelch,time_cluster

# True-positive scoring (needs passthrough correlator)
dda inv q.eval-tp --scenario=food_delivery_redis --only bocpd,passthrough
```

The `--only` flag controls which detectors/correlators are enabled. The
`time_cluster` correlator is always auto-added for `eval-scenarios`.

## What the eval does

1. Builds `observer-testbench` and `observer-scorer` Go binaries
2. Loads parquets from `comp/observer/scenarios/<scenario>/parquet/`
3. Testbench replays all data through the observer engine (same code path as live)
4. Outputs `/tmp/observer-eval-<scenario>.json`
5. Scorer matches anomaly periods against ground truth
6. Prints summary table: F1, precision, recall, alpha, scored, baseline FPs

## Interpreting results

```
Scenario                    F1  Precision  Recall  Alpha  Scored  Baseline FPs
food_delivery_redis     0.0000     0.0000  0.0000 0.0116      20             7
```

| Metric | Meaning |
|--------|---------|
| **F1** | Harmonic mean of precision and recall. 0 = no true positives matched. |
| **Precision** | What fraction of detected anomalies are real incidents |
| **Recall** | What fraction of real incidents were detected |
| **Alpha** | False positive rate = FPs / baseline duration (lower is better) |
| **Scored** | Total anomaly periods evaluated |
| **Baseline FPs** | Anomalies during the baseline (pre-disruption) period |
| **Warmup (excl)** | Anomalies excluded because they're in the detector warmup window |
| **Cascading (excl)** | Anomalies excluded as consequences of an earlier detection |

### Common gotchas

- **F1=0 doesn't mean detection failed.** The scorer checks specific ground
  truth metric names. If the observer detects anomalies under a different
  namespace (e.g. `system-checks-hf` vs `parquet`), they won't match.
- **BOCPD warmup is ~120 data points.** For 15s metrics that's 30 minutes.
  For 1s HF metrics it's 2 minutes. Anomalies during warmup are excluded.
- **Live vs replay divergences** are expected when HF data exists only in
  live. The divergence log shows "live-only" and "replay-only" anomalies
  by source namespace.

## Ground truth

Two files are needed per scenario:

1. **`comp/observer/scenarios/ground_truth.json`** — maps scenarios to expected
   TP metrics (service + metric name pairs)

2. **`comp/observer/scenarios/<scenario>/episode.json`** — disruption window
   timestamps from the episode run. If missing after a local run, copy from
   gensim-episodes:
   ```bash
   cp ~/dd/gensim-episodes/synthetics/food-delivery-redis-cpu-saturation/results/redis-cpu-saturation-1.json \
      comp/observer/scenarios/food_delivery_redis/episode.json
   ```

## Parquet management

### File types

| File pattern | Contents |
|-------------|----------|
| `observer-metrics-*.parquet` | Check metrics + DogStatsD |
| `observer-logs-*.parquet` | Log messages |
| `observer-trace-stats-*.parquet` | Pre-aggregated APM stats |
| `observer-traces-*.parquet` | Raw spans |
| `observer-lifecycle.jsonl` | Container lifecycle events |
| `advances.jsonl` | Scheduler advance log (parity debugging) |
| `detect_digests.jsonl` | Detection digests (parity debugging) |

### Compacting

Per-flush parquet files are small and numerous. Compact them:

```bash
dda inv q.compact-parquets --scenario=food_delivery_redis
# 468 files (959MB) → 4 files (23MB), 98% reduction
```

Uses pyarrow (in `deps/py_dev_requirements.txt`, synced by dda automatically).

### Downloading from S3

```bash
dda inv q.download-scenarios                    # all scenarios
dda inv q.download-scenarios --scenario=food_delivery_redis  # single
```

Fetches from `s3://qbranch-gensim-recordings` via `runs.jsonl` audit trail.
Requires AWS SSO auth: `aws-vault exec sso-agent-sandbox-account-admin`.

## Available detectors

| Detector | Default | Notes |
|----------|---------|-------|
| `bocpd` | on | Bayesian online changepoint detection. Needs ~120 points warmup. |
| `rrcf` | on | Robust random cut forest. Streaming anomaly scores. |
| `scanwelch` | off | Spectral (Welch PSD) changepoint. Strict thresholds. |
| `scanmw` | off | Mann-Whitney changepoint. Similar to ScanWelch. |
| `cusum` | off | Cumulative sum changepoint. |
| `time_cluster` | on (correlator) | Clusters anomalies by temporal proximity. |
| `passthrough` | off (correlator) | No clustering, needed for eval-tp. |

## Known detector issues

- **ScanWelch/ScanMW**: `MinDeviationMAD` should be 3.0 (was 15.0 from a
  temp noise reduction). `MinTStatistic=8.0` and `SignificanceThreshold=1e-8`
  are very strict — may need tuning.
- **Correlator ordering**: must accumulate correlations BEFORE advancing, or
  clusters from historical-timestamp anomalies get evicted before capture.
- **SamplingIntervalSec**: scan detectors populate this for dynamic proximity
  scaling in the correlator. Proximity capped at `WindowSeconds/2`.
