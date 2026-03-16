# Research Notes — Changepoint Detection

Started: 2026-03-13 17:38 UTC

## Prior Session Context (2026-03-13 15:42-17:37)

Key findings from prior session:
- MW zero output: WarmupPoints=170 > food_delivery series length (~130 pts). WindowSize must be small.
- MW V2 (WindowSize=15): SignificanceThreshold=1e-12 mathematically unreachable with n=15 windows. Max z≈4.65, min p≈3.4e-6.
- TopK zero output: Fires on first tick after MinPoints met (~120s before baseline). All warmup-filtered.
- RRCF zero output: TestBenchRRCFMetrics returns cgroup.v2.* but scenario data uses container.* names.
- CUSUM: 427k raw anomalies with median/MAD+ThresholdFactor=5.0. All clustered pre-baseline. Need ThresholdFactor=8.0+.
- BOCPD: High recall (0.90+), terrible precision (0.06-0.11). Needs stricter thresholds.
- Changes were applied but no eval runs completed in prior session.

## Scoring Window Analysis (from scorer code + metadata)

Scorer filters:
1. **Warmup filter**: predictions before `baseline.start` → filtered
2. **Cascading filter**: predictions after `max(ground_truth) + 2*sigma` → filtered
3. **Scoring**: half-Gaussian overlap, effective window ≈ ±3σ around ground truth

Scenario timing (all times UTC, 2026-03-03):

| Scenario | baseline.start | disruption.start (GT) | Window (sigma=30) |
|---|---|---|---|
| food_delivery_redis | 12:44:48 | 12:54:48 | 12:44:48 to ~12:55:48 |
| 353_postmark | 12:40:39 | 12:55:15 | 12:40:39 to ~12:56:15 |
| 213_pagerduty | 12:39:35 | 12:49:35 | 12:39:35 to ~12:50:35 |

Detectors must emit predictions:
- AFTER baseline.start (or they get warmup-filtered)
- BEFORE ground_truth + 60s (or they get cascading-filtered)
- NEAR ground_truth ±30s for best F1 scores

## Key Findings from This Session

### BOCPD Selectivity Pattern
Progressive tightening of BOCPD thresholds produced monotonic improvement:
- Baseline (H=0.05, CPT=0.6, CPM=0.7): avg 0.153
- V1 (H=0.01, CPT=0.8, CPM=0.8): avg 0.214 (+40%)
- V2 (H=0.01, CPT=0.9, CPM=0.9): avg 0.224 (+47%)
- V3 (H=0.005, CPT=0.95, CPM=0.95): avg 0.269 (+76%)
- V4 (H=0.002, CPT=0.98, CPM=0.98): food_delivery same as V3 → diminishing returns

Recall stays constant at 0.84-0.90 across all variations. Only precision improves. Pattern: fewer, higher-confidence emissions → better F1.

### TopK Streaming Architecture Problem
TopK's `fired` map permanently deduplicates per-metric. In streaming mode:
1. TopK runs on every tick
2. First tick with enough data → scores all series
3. Top-K metrics fire and get added to `fired`
4. On subsequent ticks, those metrics are skipped
5. The first scoring happens during early data loading (before baseline_start)
6. Result: all emissions are warmup-filtered, and TopK never re-evaluates

Fix would require: periodic reset of `fired` map, or window-based dedup (only suppress fires within N seconds of previous fire, not permanently).

### RRCF Auto-Discovery Limitations
Auto-discover selects top-6 series by variance, but:
1. High-variance series are often infrastructure metrics with irregular cadences
2. Forward-fill alignment produces many synthetic (held) values
3. RRCF's z-score thresholding (mean + 3σ) is conservative for skewed score distributions
4. Even with alignment, only a fraction of timestamps produce genuine multivariate vectors

### Scorer Behavior Notes
- Warmup filter: predictions before metadata baseline.start
- Cascading filter: predictions after max(ground_truth) + 2*sigma (60s for sigma=30)
- Half-Gaussian overlap: predictions within ±σ of GT score well; ±3σ is negligible
- food_delivery: scoring window is only ~660s (baseline_start to disruption+60s)
- postmark: scoring window is ~960s
- pagerduty: scoring window is ~660s

---
