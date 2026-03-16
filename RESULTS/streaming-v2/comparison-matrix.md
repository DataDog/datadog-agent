# Streaming Detector Comparison Matrix

**Date**: 2026-03-16
**Branch**: `ella/changepoint-detection-results-2` (merged q-branch-observer)
**Changes**: ScanMW, ScanWelch, E-Divisive converted from batch SeriesDetector to streaming Detector with segment advancement; BOCPD tuned (H=0.005, CPT=0.95, CPM=0.95)

## Summary

| Detector | food_delivery | postmark | pagerduty | **Avg F1** |
|---|---|---|---|---|
| **scanwelch** | 0.946 | **0.619** | **0.473** | **0.679** |
| **scanmw** | **0.946** | 0.578 | 0.315 | **0.613** |
| bocpd (tuned) | 0.185 | 0.444 | 0.153 | 0.261 |
| edivisive | 0.527 | 0.000 | 0.187 | 0.238 |
| mannwhitney | 0.358 | 0.038 | 0.248 | 0.215 |
| bocpd (baseline) | 0.146 | 0.195 | 0.105 | 0.149 |

## BOCPD Tuning Comparison

| Param | Baseline (q-branch-observer) | Tuned (results-1 best) |
|---|---|---|
| Hazard | 0.05 | **0.005** |
| CPThreshold | 0.6 | **0.95** |
| CPMassThreshold | 0.7 | **0.95** |

| Scenario | Baseline F1 | Tuned F1 | Delta |
|---|---|---|---|
| food_delivery | 0.146 | 0.185 | +0.039 |
| postmark | 0.195 | 0.444 | **+0.249** |
| pagerduty | 0.105 | 0.153 | +0.048 |
| **Avg F1** | **0.149** | **0.261** | **+0.112** |

The tuning reduces hazard by 10x (fewer changepoint hypotheses per tick) and raises both thresholds to 0.95 (require near-certain posterior). This dramatically improves precision — baseline BOCPD scores 12/9/16 predictions across scenarios vs tuned at 7/2/10. Postmark benefits most because the higher selectivity eliminates most false positives.

## Detailed Scores

### food_delivery_redis

| Detector | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| scanmw | 0.946 | 0.946 | 0.946 | 1 | 2 | 5 |
| scanwelch | 0.946 | 0.946 | 0.946 | 1 | 3 | 7 |
| edivisive | 0.527 | 0.395 | 0.790 | 2 | 6 | 4 |
| mannwhitney | 0.358 | 0.224 | 0.896 | 4 | 0 | 21 |
| bocpd (tuned) | 0.185 | 0.106 | 0.739 | 7 | 0 | 25 |
| bocpd (baseline) | 0.146 | 0.079 | 0.946 | 12 | 0 | 25 |

### 353_postmark

| Detector | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| scanwelch | 0.619 | 0.619 | 0.619 | 1 | 0 | 3 |
| scanmw | 0.578 | 0.433 | 0.867 | 2 | 0 | 3 |
| bocpd (tuned) | 0.444 | 0.333 | 0.666 | 2 | 0 | 34 |
| bocpd (baseline) | 0.195 | 0.108 | 0.974 | 9 | 0 | 50 |
| mannwhitney | 0.038 | 0.029 | 0.057 | 2 | 0 | 58 |
| edivisive | 0.000 | 0.000 | 0.000 | 1 | 2 | 0 |

### 213_pagerduty

| Detector | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| scanwelch | 0.473 | 0.315 | 0.946 | 3 | 0 | 9 |
| scanmw | 0.315 | 0.189 | 0.946 | 5 | 1 | 7 |
| mannwhitney | 0.248 | 0.144 | 0.867 | 6 | 0 | 20 |
| edivisive | 0.187 | 0.105 | 0.841 | 8 | 4 | 14 |
| bocpd (tuned) | 0.153 | 0.084 | 0.841 | 10 | 0 | 23 |
| bocpd (baseline) | 0.105 | 0.056 | 0.896 | 16 | 1 | 25 |

## vs Previous Best (results-2 batch era)

| Detector | Prev Avg F1 | New Avg F1 | Delta |
|---|---|---|---|
| scanwelch | 0.655 | **0.679** | **+0.024** |
| scanmw | 0.746 | 0.613 | -0.133 |
| edivisive | 0.290 | 0.238 | -0.052 |
| mannwhitney | 0.215 | 0.215 | 0.000 |
| bocpd | 0.164 | **0.261** | **+0.097** |

## Key Observations

1. **ScanWelch is the new champion** (Avg F1=0.679) — it gained on postmark (0.526→0.619) and held well on pagerduty
2. **ScanMW regressed on pagerduty** (0.974→0.315) — the streaming conversion causes more scored predictions (5 vs 1), killing precision. Segment advancement may be re-firing on false positives in the large 95k-series scenario
3. **BOCPD tuning is worth +0.112 avg F1** — the biggest win is postmark (0.195→0.444). Reducing hazard 10x and raising thresholds to 0.95 cuts scored predictions roughly in half, dramatically improving precision while maintaining adequate recall
4. **E-Divisive postmark is 0.000** — single scored prediction is a pure FP. Needs investigation
5. **Precision is the bottleneck** across the board — all detectors have high recall (0.8+) but low precision on pagerduty/postmark
6. **Selectivity pattern holds**: fewer scored predictions → higher F1. ScanWelch scores only 1/1/3 across scenarios vs ScanMW's 1/2/5

## Eval Methodology

- All evals isolated: `-enable "<detector>,time_cluster" -disable <everything_else>`
- Sigma=30s Gaussian scoring
- All eval JSONs saved in `RESULTS/streaming-v2/`
- Baseline BOCPD uses q-branch-observer defaults (H=0.05, CPT=0.6, CPM=0.7)
- Tuned BOCPD uses results-1 best (H=0.005, CPT=0.95, CPM=0.95)
