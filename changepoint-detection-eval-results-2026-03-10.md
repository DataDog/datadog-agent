# Changepoint Detection Eval Results — 2026-03-10

**Branch**: `ella/changepoint-detection-algorithms` (post-pruning)
**Commit**: `15021b5685` (Merge branch after pruning underperformers)
**Detectors**: cusum, bocpd, rrcf, mannwhitney, correlation, topk
**Scenarios**: 213_pagerduty, 353_postmark, food_delivery_redis
**Scorer**: Gaussian F1 (σ=30s), per-metric TP/FP scoring

---

## L2 — Timestamp Detection (TimeCluster Correlator)

| Rank | Detector | 213_pagerduty | 353_postmark | food_delivery | **Avg L2 F1** | vs Baseline |
|:---:|----------|:---:|:---:|:---:|:---:|:---:|
| 1 | **mannwhitney** | **0.630** | **0.263** | 0.544 | **0.479** | 3.21x |
| 2 | **correlation** | 0.350 | 0.000 | **0.713** | **0.354** | 2.38x |
| 3 | **topk** | 0.578 | 0.127 | 0.270 | **0.325** | 2.18x |
| 4 | bocpd | 0.154 | 0.088 | 0.206 | 0.149 | baseline |
| 5 | cusum | 0.125 | 0.084 | 0.111 | 0.107 | 0.72x |
| 6 | rrcf | 0.000 | 0.000 | 0.000 | 0.000 | 0x |

## L2 — Prediction Counts (after TimeCluster)

| Detector | pagerduty | postmark | food_delivery |
|----------|:---:|:---:|:---:|
| mannwhitney | 2 | 6 | 2 |
| topk | 2 | 6 | 6 |
| correlation | 1 | 0 | 1 |
| bocpd | 11 | 20 | 5 |
| cusum | 15 | 21 | 16 |
| rrcf | 0 | 0 | 0 |

## L1 — Per-Metric Scoring (Passthrough Correlator)

### 213_pagerduty (4 TP metrics, 3 FP metrics)

| Detector | L1 F1 | L1 Prec | L1 Rec | Scored | mTP | mFP | mUnk | mPrec | mRec |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| topk | 0.1733 | 0.0963 | 0.867 | 9 | 2 | 0 | 9 | 1.00 | 0.50 |
| correlation | 0.0700 | 0.0389 | 0.350 | 9 | 1 | 0 | 133 | 1.00 | 0.25 |
| mannwhitney | 0.0320 | 0.0163 | 0.946 | 58 | 39 | 0 | 978 | 1.00 | 0.75 |
| cusum | 0.0013 | 0.0007 | 1.000 | 1495 | 331 | 0 | 10929 | 1.00 | 1.00 |
| bocpd | 0.0009 | 0.0004 | 0.921 | 2094 | 167 | 0 | 3029 | 1.00 | 1.00 |
| rrcf | 0.0000 | 0.0000 | 0.000 | 0 | 0 | 0 | 0 | 0.00 | 0.00 |

### 353_postmark (3 TP metrics, 3 FP metrics)

| Detector | L1 F1 | L1 Prec | L1 Rec | Scored | mTP | mFP | mUnk | mPrec | mRec |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| topk | 0.0523 | 0.0278 | 0.445 | 16 | 3 | 2 | 15 | 0.60 | 1.00 |
| mannwhitney | 0.0031 | 0.0016 | 0.921 | 592 | 8 | 2 | 1784 | 0.80 | 0.67 |
| cusum | 0.0015 | 0.0007 | 0.921 | 1266 | 322 | 94 | 11515 | 0.77 | 1.00 |
| bocpd | 0.0011 | 0.0005 | 0.921 | 1692 | 64 | 15 | 2323 | 0.81 | 1.00 |
| correlation | 0.0000 | 0.0000 | 0.000 | 0 | 35 | 0 | 179 | 1.00 | 0.33 |
| rrcf | 0.0000 | 0.0000 | 0.000 | 0 | 0 | 0 | 0 | 0.00 | 0.00 |

### food_delivery_redis (3 TP metrics, 3 FP metrics)

| Detector | L1 F1 | L1 Prec | L1 Rec | Scored | mTP | mFP | mUnk | mPrec | mRec |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| topk | 0.1182 | 0.0630 | 0.946 | 15 | 2 | 0 | 14 | 1.00 | 0.67 |
| correlation | 0.0172 | 0.0087 | 0.713 | 82 | 425 | 56 | 438 | 0.88 | 0.67 |
| cusum | 0.0033 | 0.0016 | 0.946 | 577 | 529 | 72 | 12198 | 0.88 | 1.00 |
| mannwhitney | 0.0026 | 0.0013 | 0.946 | 720 | 59 | 2 | 2523 | 0.97 | 1.00 |
| bocpd | 0.0013 | 0.0006 | 0.946 | 1488 | 230 | 27 | 5563 | 0.89 | 1.00 |
| rrcf | 0.0000 | 0.0000 | 0.000 | 0 | 0 | 0 | 0 | 0.00 | 0.00 |

## L1 — Metric Score Summary (Averaged)

| Detector | Avg mPrec | Avg mRec | Total mFP | Notes |
|----------|:---:|:---:|:---:|---|
| mannwhitney | 0.92 | 0.81 | 4 | Best balance of precision and recall |
| topk | 0.87 | 0.72 | 2 | Fewest FPs, best precision among selective detectors |
| correlation | 0.96 | 0.42 | 56 | High precision but low recall; blind on postmark |
| bocpd | 0.90 | 1.00 | 42 | Perfect recall, moderate FP noise |
| cusum | 0.88 | 1.00 | 166 | Perfect recall, noisiest |
| rrcf | 0.00 | 0.00 | 0 | Zero output on all scenarios |

## Delta vs Previous Results (changepoint-detection-summary.md)

| Detector | Previous Avg L2 | Current Avg L2 | Delta |
|----------|:---:|:---:|:---:|
| mannwhitney | 0.479 | 0.479 | unchanged |
| correlation | 0.354 | 0.354 | unchanged |
| topk | 0.355 | 0.325 | **-0.030** (postmark 0.217→0.127) |
| bocpd | 0.149 | 0.149 | unchanged |
| cusum | 0.107 | 0.107 | unchanged |
| rrcf | 0.000 | 0.000 | unchanged |

**Pruned detectors** (removed in commit `cce23848`): pelt, edivisive, ensemble, cusum_hardened, cusum_adaptive.

## Notes

- **rrcf** is dead weight — zero output on all scenarios. It's hardcoded to `cgroup.v2` metrics which don't exist in scenario data.
- **topk postmark regression**: L2 dropped from 0.217→0.127, L1 recall dropped from 0.650→0.445. Needs investigation — may be a side effect of the pruning commit or a registry change.
- **mannwhitney** remains the clear overall winner (3.2x over BOCPD baseline).
- **correlation** is the best niche detector for multi-service cascading failures (food_delivery 0.713) but produces nothing on postmark.
- All metric FP counts shifted: previous summary reported 0 mFP for mannwhitney on pagerduty, current confirms 0. Previous reported 3 mFP on food_delivery, current shows 2. Minor scoring methodology differences possible.
