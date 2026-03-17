# Session 4 Summary — Streaming Stateful Detectors

**Date**: 2026-03-17
**Branch**: `ella/changepoint-detection-results-2`
**Duration**: ~4.5 hours
**Goal**: Implement new streaming `Detector` interface algorithms (like BOCPD) that beat BOCPD tuned (avg F1=0.261)

## Result: Target not met

No streaming detector beat BOCPD's avg F1 of 0.261. The best was PageHinkley V3 at 0.211.

## Algorithms Implemented

| Detector | Description | Outcome |
|---|---|---|
| **StreamSeg** | Two circular buffers (baseline + recent, size 30 each), Welch t-test + MAD + effect size | Best food_delivery (0.306) |
| **PageHinkley** | Bilateral Page-Hinkley sequential test with regime resets and MAD verification | Best avg F1 (0.211) |
| **DualEWM** | Fast/slow exponential weighted mean crossover | Abandoned — 0 scored predictions across all variants |

## Best Results

**PageHinkley V3** — best overall avg:

| Scenario | F1 | Precision | Recall | Raw Anomalies | Scored |
|---|---|---|---|---|---|
| food_delivery_redis | 0.297 | 0.198 | 0.593 | 20 | 3 |
| 353_postmark | 0.000 | — | — | 81 | 1 (FP) |
| 213_pagerduty | 0.337 | 0.253 | 0.506 | 57 | 3 |
| **Average** | **0.211** | | | | |

**StreamSeg V3** — best food_delivery + postmark:

| Scenario | F1 | Precision | Recall | Raw Anomalies | Scored |
|---|---|---|---|---|---|
| food_delivery_redis | 0.306 | 0.184 | 0.921 | 4372 | 5 |
| 353_postmark | 0.248 | 0.144 | 0.867 | 9793 | 6 |
| 213_pagerduty | 0.000 | — | — | 3608 | 0 |
| **Average** | **0.185** | | | | |

## All Variations Tried (12 total)

| Detector | Version | Key Change | food_del F1 | Outcome |
|---|---|---|---|---|
| StreamSeg | V1 | Baseline (t>8, MAD>5, eff>0.85) | 0.272 | Too many raw anomalies (16K) |
| StreamSeg | V2 | t>12, MAD>8, eff>0.90 | 0.272 | Unchanged F1, fewer raw |
| StreamSeg | **V3** | **t>20, MAD>15, eff>0.95** | **0.306** | **Best StreamSeg** |
| StreamSeg | V4 | WindowSize 30→60 | 0.000 | Timing issues, cascading filtered |
| StreamSeg | V5 | V1 + top-K=10 per call | 0.272 | Top-K doesn't help |
| StreamSeg | V6 | V3 + onset backdating | 0.000 | Pushed into warmup filter zone |
| PageHinkley | V1 | Baseline (warmup=60, thresh=15, MAD>5) | 0.246 | Promising |
| PageHinkley | V2 | thresh=20, MAD>8 | 0.142 | Over-filtered |
| PageHinkley | **V3** | **warmup=120, thresh=15, MAD>5** | **0.297** | **Best PageHinkley** |
| PageHinkley | V4 | V3 + slack 0.5→0.3 | 0.297 | Helped postmark, destroyed pagerduty |
| DualEWM | V1-V3 | Various thresholds + frozen slow EWM | 0.000 | All failed |

## Why Streaming Detectors Underperform

1. **Information asymmetry**: ScanMW compares full segments (100+ pts each side) at the optimal split. StreamSeg compares 30 vs 30 in fixed windows. PageHinkley accumulates a single running sum. Far less statistical power per decision.

2. **No detector handles both abrupt AND subtle shifts**: PageHinkley catches pagerduty (abrupt, 0.337) but misses postmark (subtle, 0.000). StreamSeg catches postmark (0.248) but misses pagerduty (0.000). Tuning for one kills the other.

3. **Volume problem**: StreamSeg fires on 4,000-10,000 of 29K series per scenario. Even with strict thresholds, many series show genuine statistical shifts that aren't ground-truth changepoints. Batch detectors naturally limit volume via fire-once dedup.

4. **Timing imprecision**: Streaming detectors fire at detection time, not changepoint time. The scorer penalizes timing offsets (sigma=30s Gaussian). Onset backdating failed — pushed timestamps into the warmup filter zone.

## Comparison to Prior Sessions

| Session | Best New Detector | Avg F1 | Approach |
|---|---|---|---|
| 2 | ScanMW V3 | **0.746** | Batch scan, MW U-test |
| 3 | WinComp V2 | **0.509** | Batch window comparison |
| **4** | **PageHinkley V3** | **0.211** | **Streaming sequential test** |

## Files Created

| File | Description |
|---|---|
| `comp/observer/impl/metrics_detector_streamseg.go` | StreamSeg streaming Detector |
| `comp/observer/impl/metrics_detector_pagehinkley.go` | PageHinkley streaming Detector |
| `comp/observer/impl/metrics_detector_dualewm.go` | DualEWM streaming Detector (abandoned) |
| `comp/observer/impl/component_catalog.go` | Registered 3 new detectors |

## Suggested Next Steps

1. **Ensemble**: Combine PageHinkley (abrupt) + StreamSeg (subtle) with score-based priority
2. **BOCPD with Student-t likelihood**: More robust to heavy-tailed metrics data
3. **Accept batch wins**: The batch SeriesDetector approach (0.509-0.746) is fundamentally stronger for this problem. Future work may be better spent improving the seriesDetectorAdapter efficiency than reimplementing detection in the streaming interface.
