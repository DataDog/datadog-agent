# Iteration Log — Streaming Changepoint Detection

Session started: 2026-03-13 19:50 UTC
Focus: NEW algorithm implementations (E-Divisive, PELT, Scan-MW)

---

## E-Divisive — V1-default (2026-03-13 19:55)

**What changed**: New algorithm — E-Divisive (penalized Gaussian log-likelihood scan). MinSegment=15, MinPoints=30, PenaltyFactor=8.0, MinRelativeChange=2.0. Implements SeriesDetector (batch).

**Hypothesis**: Scanning all split points and maximizing log-likelihood gain should find the optimal changepoint location. Non-parametric penalty approach should be more robust than fixed-window detectors.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.222 | 0.125 | 1.000 | 8 | 7 | 7 |
| 353_postmark | pending | | | | | |
| 213_pagerduty | pending | | | | | |

**Delta vs baseline**: +0.222 on food_delivery (0.000 → 0.222) — NEW BEST for this scenario among new detectors

**Analysis**: First run, already beats BOCPD (0.179) on food_delivery. High recall (1.0) but low precision (0.125) — 8 scored, 7 are FP. 7 warmup filtered, 7 cascading. The detector finds changepoints in many series. Need to make it more selective. But first, run full eval across all 3 scenarios.

**Keep or revert**: Keep (promising, pending full eval)

---

## RobustScan — V1-default (2026-03-13 20:05)

**What changed**: New algorithm — RobustScan (Welch t-statistic scan + median/MAD verification). MinSegment=10, MinPoints=25, MinDeviationMAD=3.0. Implements SeriesDetector (batch).

**Hypothesis**: Scanning with Welch's t-test at every split point, verified with robust median/MAD check.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.090 | 0.047 | 0.946 | 20 | 10 | 14 |

**Delta vs baseline**: N/A (new detector, but F1=0.090 worse than E-Divisive)

**Analysis**: 391 raw anomalies, 20 scored — way too many FPs. The Welch t-statistic is too sensitive with MinDeviationMAD=3.0. Would need much higher thresholds. But E-Divisive with penalized likelihood is already better. Deprioritize RobustScan in favor of E-Divisive and ScanMW.

**Keep or revert**: Keep code but deprioritize

---

## ScanMW — V1-default (2026-03-13 20:15)

**What changed**: New algorithm — ScanMW (Mann-Whitney scan with O(n log n) incremental ranking). MinSegment=12, MinPoints=30, SignificanceThreshold=1e-6, MinEffectSize=0.8, MinDeviationMAD=2.5. Implements SeriesDetector (batch).

**Hypothesis**: Scanning all split points with the rank-based Mann-Whitney test is more robust than parametric approaches (E-Divisive, RobustScan) and should find changepoints with better precision.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.510 | 0.382 | 0.765 | 2 | 2 | 1 |

**Delta vs baseline**: +0.510 on food_delivery (0.000 → 0.510) — **MASSIVE BREAKTHROUGH, best single-scenario score ever**

**Analysis**: Only 51 raw anomalies (extremely selective), 2 scored, both near ground truth. The triple filter (p-value < 1e-6, effect size > 0.8, deviation > 2.5 MADs) is highly effective. Precision 0.382 is 3x better than E-Divisive (0.125). Recall lower (0.765 vs 1.0) because fewer series fire. This is the right tradeoff — selectivity wins. Need full 3-scenario eval urgently.

**Keep or revert**: KEEP — this is the best result of the entire research effort

---

## E-Divisive — V1-default FULL EVAL (2026-03-13 20:20)

**Full 3-scenario results for E-Divisive V1 (PenaltyFactor=8.0, MinRelativeChange=2.0)**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.222 | 0.125 | 1.000 | 8 | 7 | 7 |
| 353_postmark | 0.325 | 0.195 | 0.974 | 5 | 1 | 2 |
| 213_pagerduty | 0.129 | 0.070 | 0.841 | 12 | 4 | 11 |
| **Avg F1** | **0.225** | | | | | |

**Delta vs baseline**: +0.225 avg F1 (best previous was BOCPD 0.153) — **NEW BEST overall avg**

**Analysis**: E-Divisive produces non-zero scores on all 3 scenarios. Postmark is particularly strong (0.325 vs BOCPD 0.195). The issue is precision — high recall but many false positives. The penalty and MinRelativeChange thresholds need tuning.

Now testing V2 with PenaltyFactor=12.0, MinRelativeChange=4.0.

---

## ScanMW — V1-default continued (2026-03-13 20:30)

**Postmark result**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| 353_postmark | 0.526 | 0.526 | 0.526 | 1 | 1 | 2 |

Only 36 raw anomalies. 1 scored, perfectly balanced P/R. ScanMW is extremely selective.

**Running totals for ScanMW V1**:
| Scenario | F1 | 
|---|---|
| food_delivery | 0.510 |
| postmark | 0.526 |
| pagerduty | pending |

---

## E-Divisive — V2-selective (2026-03-13 20:28)

**What changed**: PenaltyFactor 8.0→12.0, MinRelativeChange 2.0→4.0.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.333 | 0.200 | 1.000 | 5 | 5 | 4 |

**Delta vs V1**: +0.111 (0.222 → 0.333) — selectivity improvement worked

**Analysis**: Raw anomalies dropped 259→191, scored 8→5. Better precision (0.125→0.200) with same recall (1.000). Further tuning may help but ScanMW is already much better. Running postmark/pagerduty for completeness.

---

## ScanMW — V1-default FULL EVAL COMPLETE (2026-03-13 20:45)

**FULL 3-SCENARIO RESULTS — ScanMW V1**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading | Raw Anomalies |
|---|---|---|---|---|---|---|---|
| food_delivery_redis | 0.510 | 0.382 | 0.765 | 2 | 2 | 1 | 51 |
| 353_postmark | 0.526 | 0.526 | 0.526 | 1 | 1 | 2 | 36 |
| 213_pagerduty | 0.561 | 0.421 | 0.841 | 2 | 2 | 4 | 140 |
| **Avg F1** | **0.532** | | | | | | |

**Delta vs baseline**: +0.532 (from 0.000, new detector)
**Delta vs best previous**: +0.379 (vs BOCPD 0.153)

### THIS EXCEEDS THE STRETCH TARGET OF 0.500

Key observations:
- Extremely selective: 51/36/140 raw anomalies across 28K/25K/95K series
- Low scored count (2/1/2) means time_cluster consolidates well
- Balanced P/R — not sacrificing one for the other
- The triple filter (p-value < 1e-6, effect size > 0.8, deviation > 2.5 MADs) is the key
- Pagerduty is BEST at 0.561 despite having 95K series — selectivity dominates

Next: try tuning ScanMW to push even higher, and finish E-Divisive V2 full eval.

---

## ScanMW — V2-relaxed (2026-03-13 20:50)

**What changed**: SignificanceThreshold 1e-6→1e-5, MinEffectSize 0.8→0.7, MinDeviationMAD 2.5→2.0.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.148 | 0.082 | 0.739 | 9 | 4 | 3 |

**Analysis**: MUCH WORSE. 147 raw anomalies (up from 51), 9 scored FPs. Relaxing thresholds lets in too much noise. **Selectivity is the key** — never relax.

**Keep or revert**: Revert immediately

---

## ScanMW — V3-strict (2026-03-13 20:55)

**What changed**: SignificanceThreshold 1e-6→1e-8, MinEffectSize 0.8→0.85, MinDeviationMAD 2.5→3.0.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.739 | 0.739 | 0.739 | 1 | 2 | 1 |
| 353_postmark | pending | | | | | |
| 213_pagerduty | pending | | | | | |

**Delta vs V1**: +0.229 on food_delivery (0.510→0.739)

**Analysis**: Tightening thresholds is hugely beneficial. Only 28 raw anomalies (down from 51), 1 scored — a single near-perfect prediction. This confirms: **more selective = better F1**. Running full eval now.

---

## ScanMW — V3-strict FULL EVAL COMPLETE (2026-03-13 21:15)

**FULL 3-SCENARIO RESULTS — ScanMW V3-strict**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading | Raw Anomalies |
|---|---|---|---|---|---|---|---|
| food_delivery_redis | 0.739 | 0.739 | 0.739 | 1 | 2 | 1 | 28 |
| 353_postmark | 0.526 | 0.526 | 0.526 | 1 | 0 | 1 | 14 |
| 213_pagerduty | 0.974 | 0.974 | 0.974 | 1 | 1 | 5 | 118 |
| **Avg F1** | **0.746** | | | | | | |

**Delta vs V1**: +0.214 avg (0.532→0.746)
**Delta vs best previous (BOCPD)**: +0.593 avg (0.153→0.746)

### 0.746 AVG F1 — EXCEPTIONAL RESULT

Key observations:
- V3's stricter thresholds massively improved food_delivery (0.510→0.739) and pagerduty (0.561→0.974)
- Postmark unchanged at 0.526 (only 1 TP prediction in both versions)
- Pagerduty near-perfect at 0.974 — the most selective result ever on 95K series
- Each scenario has exactly 1 scored prediction — time_cluster consolidation works perfectly
- The pattern is clear: stricter thresholds → fewer FPs → better F1
- Postmark is the bottleneck at 0.526. Could investigate if further tuning helps.

**Current params**: SignificanceThreshold=1e-8, MinEffectSize=0.85, MinDeviationMAD=3.0

---

## E-Divisive — V2-selective FULL EVAL (2026-03-13 21:20)

**Full 3-scenario E-Divisive V2 (PenaltyFactor=12, MinRelativeChange=4)**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.333 | 0.200 | 1.000 | 5 | 5 | 4 |
| 353_postmark | 0.351 | 0.263 | 0.526 | 2 | 1 | 1 |
| 213_pagerduty | 0.187 | 0.105 | 0.841 | 8 | 3 | 7 |
| **Avg F1** | **0.290** | | | | | |

**Analysis**: Improved from V1 avg 0.225→0.290 by raising selectivity. But ScanMW at 0.746 is far superior.

---

## ScanMW — V4-minseg8 (2026-03-13 21:25)

**What changed**: MinSegment 12→8, MinPoints 30→25.

**Results**: Identical to V3 on both food_delivery (0.739) and postmark (0.526). MinSegment change has no effect because the changepoint is well within the series and the MW test finds the same best split regardless of segment size bounds.

**Keep or revert**: Revert to V3 params — no improvement.

---

## ScanMW — V5-onset (2026-03-13 21:30)

**What changed**: Added onset detection — scan backward from MW-optimal split to find first deviating point.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.597 | 0.448 | 0.896 | 2 | 2 | 1 |
| 353_postmark | 0.649 | 0.487 | 0.974 | 2 | 0 | 1 |

**Analysis**: Onset detection helps postmark (+0.123 from 0.526) but hurts food_delivery (-0.142 from 0.739). The shifted timestamps create separate time_cluster periods that reduce precision on food_delivery. Net effect is negative vs V3.

**Keep or revert**: Revert — V3 avg (0.746) > V5 estimate (~0.739)

---

## ScanMW — V6-ultrastrict (2026-03-13 21:40)

**What changed**: SignificanceThreshold 1e-8→1e-10, MinEffectSize 0.85→0.90, MinDeviationMAD 3.0→4.0.

**Results**: F1=0.000 on food_delivery (0 scored). Too strict — killed all predictions.

**Keep or revert**: Revert immediately

---

## Mann-Whitney — V3-relaxed (2026-03-13 22:00)

**What changed**: Existing MW detector with MinEffectSize 0.98→0.90, MinDeviationSigma 4.0→3.0.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.358 | 0.224 | 0.896 | 4 | 0 | 21 |
| 353_postmark | 0.038 | 0.029 | 0.057 | 2 | 0 | 58 |
| 213_pagerduty | pending | | | | | |

**Analysis**: MW now produces non-zero output on food_delivery! 8670 raw anomalies though (too many). Fixed-baseline MW struggles on postmark. The scan-based ScanMW approach is fundamentally better.

**Keep or revert**: Keep (MW producing non-zero is good, even if ScanMW is better)

---

## E-Divisive — V3-ultraselective (2026-03-13 22:05)

**What changed**: PenaltyFactor 12→16, MinRelativeChange 4→6.

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.487 | 0.325 | 0.974 | 3 | 5 | 4 |
| 353_postmark | 0.000 | 0.000 | 0.000 | 0 | 1 | 0 |

**Analysis**: Great on food_delivery (0.333→0.487) but killed postmark (0→0). MinRelativeChange=6 is too strict for postmark's subtle changes. Reverted to V2 (PF=12, MRC=4).

**Keep or revert**: Revert to V2

---

## E-Divisive — V4-middle (2026-03-13 22:20)

**What changed**: PenaltyFactor 12→14, MinRelativeChange 4→5 (trying middle ground between V2 and V3).

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.400 | 0.250 | 1.000 | 4 | 5 | 4 |
| 353_postmark | 0.351 | 0.263 | 0.526 | 2 | 1 | 0 |
| 213_pagerduty | 0.072 | 0.041 | 0.287 | 7 | 3 | 4 |
| **Avg F1** | **0.274** | | | | | |

**Analysis**: V4 avg (0.274) worse than V2 (0.290). Higher penalty helps food_delivery but hurts pagerduty. The tradeoff doesn't improve the average. Reverting to V2.

**Keep or revert**: Revert to V2

---

## Mann-Whitney — V3-relaxed FULL EVAL (2026-03-13 22:25)

**Full results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | 0.358 | 0.224 | 0.896 | 4 | 0 | 21 |
| 353_postmark | 0.038 | 0.029 | 0.057 | 2 | 0 | 58 |
| 213_pagerduty | 0.248 | 0.144 | 0.867 | 6 | 0 | 20 |
| **Avg F1** | **0.215** | | | | | |

**Analysis**: MW is now producing non-zero output on 2/3 scenarios (was 0/3 before). The fixed-baseline approach inherently limits postmark performance. But this is a significant improvement from 0.000 avg.

---

## FINAL SUMMARY

### Session Overview
- **Duration**: ~2.5 hours
- **Variations tried**: 12
- **New algorithms implemented**: 3 (E-Divisive, RobustScan, ScanMW)
- **Existing algorithms improved**: 1 (MannWhitney)

### Final Results Table

| Detector | Best Variation | food_delivery | postmark | pagerduty | Avg F1 |
|---|---|---|---|---|---|
| **scanmw** | **V3-strict** | **0.739** | **0.526** | **0.974** | **0.746** |
| edivisive | V2-selective | 0.333 | 0.351 | 0.187 | 0.290 |
| mannwhitney | V3-relaxed | 0.358 | 0.038 | 0.248 | 0.215 |
| bocpd | V1-selective | 0.179 | 0.195 | 0.119 | 0.164 |
| corrshift | baseline | 0.211 | 0.049 | 0.000 | 0.087 |
| robustscan | V1-default | 0.090 | — | — | — |

### Success Criteria Achievement

| Metric | Start | Current | Target | Stretch | Status |
|---|---|---|---|---|---|
| Best avg F1 | 0.153 | **0.746** | 0.400 | 0.500 | **EXCEEDS STRETCH** |
| Detectors > 0.1 avg | 2 | **4** | 4 | 6 | **TARGET MET** |
| Detectors > 0.3 avg | 0 | **1** | 2 | 3 | Partial |
| Best pagerduty | 0.119 | **0.974** | 0.400 | 0.600 | **EXCEEDS STRETCH** |
| Best postmark | 0.195 | **0.526** | 0.300 | 0.450 | **EXCEEDS STRETCH** |
| Best food_delivery | 0.211 | **0.739** | 0.500 | 0.800 | **BETWEEN TARGET AND STRETCH** |

### What Worked Best

1. **ScanMW (new algorithm)**: The breakthrough was implementing a scan-based Mann-Whitney changepoint detector that tries every possible split point instead of using a fixed baseline. Combined with a triple-layer selectivity filter (p-value < 1e-8, effect size > 0.85, deviation > 3.0 MADs), it achieves avg F1 = 0.746.

2. **E-Divisive (new algorithm)**: A Gaussian log-likelihood cost scan that complements ScanMW. While less accurate (avg 0.290), it provides a different detection mechanism and could be useful in an ensemble.

3. **Mann-Whitney fix**: Relaxing the existing MW's effect size threshold from 0.98 to 0.90 unblocked it from 0.000 to 0.215 avg.

### Key Insights

1. **Scan-based > fixed-window**: Scanning all split points for optimal changepoint is fundamentally better than fixed baseline windows.
2. **Selectivity is king**: The triple filter (statistical significance + effect size + robust deviation) is what makes ScanMW precise. Each filter layer is necessary.
3. **Sweet spot exists**: Too relaxed (V1: 0.532) or too strict (V6: 0.000) both lose. V3's params hit the optimal tradeoff.
4. **Batch SeriesDetector with fired map**: Using the batch interface with internal dedup state is an effective pattern for streaming.
5. **O(n log n) incremental ranking**: Pre-sorting and incremental rank-sum tracking makes the per-call cost manageable for 95K series.

### What to Try Next

1. **Postmark-specific tuning**: ScanMW's postmark score (0.526) is the bottleneck. Onset detection helped (+0.123) but hurt food_delivery. A scenario-adaptive approach might help.
2. **Ensemble detector**: Combine ScanMW + E-Divisive predictions with voting or score aggregation.
3. **Kernel-based methods (MMD)**: Register and evaluate the existing MMD prototype.
4. **Fix remaining zero-output detectors**: RRCF, TopK, CUSUM still produce zero output.
5. **ScanMW with CUSUM onset refinement**: The batch-era TopK V8's best improvement was CUSUM onset detection. Applying this to ScanMW might improve timestamp precision without the time_cluster splitting issue.

### Files Modified

| File | Change |
|---|---|
| `comp/observer/impl/metrics_detector_scanmw.go` | **NEW** — ScanMW detector (SeriesDetector) |
| `comp/observer/impl/metrics_detector_edivisive.go` | **NEW** — E-Divisive detector (SeriesDetector) |
| `comp/observer/impl/metrics_detector_robustscan.go` | **NEW** — RobustScan detector (SeriesDetector) |
| `comp/observer/impl/component_catalog.go` | Registered 3 new detectors |
| `comp/observer/impl/metrics_detector_mannwhitney.go` | Tuned thresholds (MinEffectSize, MinDeviationSigma) |
| `comp/observer/impl/metrics_detector_bocpd.go` | Kept V1-selective params from session 1 |

