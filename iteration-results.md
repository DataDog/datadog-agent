# Iteration Results — Streaming Eval

Last updated: 2026-03-14 04:15 UTC

## Current Best Scores (ALL ISOLATED — `-disable bocpd,rrcf`)

| Detector | Best Variation | food_delivery | postmark | pagerduty | Avg F1 | Delta vs Baseline |
|---|---|---|---|---|---|---|
| **scanmw** | **V3-strict** | **0.739** | **0.526** | **0.974** | **0.746** | **CHAMPION** |
| **scanwelch** | **V5-hybrid** | **0.493** | **0.526** | **0.945** | **0.655** | **NEW — 2nd BEST** |
| edivisive | V2-selective | 0.333 | 0.351 | 0.187 | 0.290 | NEW |
| mannwhitney | V3-relaxed | 0.358 | 0.038 | 0.248 | 0.215 | +0.215 (was 0.000) |
| bocpd | V1-selective | 0.179 | 0.195 | 0.119 | 0.164 | +0.011 |
| robustscan | V1-default | 0.090 | — | — | ~0.090 | NEW |
| corrshift | baseline | 0.211 | 0.049 | 0.000 | 0.087 | — |
| cusum | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |
| rrcf | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |
| topk | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |

## Success Criteria

| Metric | Current | Target | Stretch | Status |
|---|---|---|---|---|
| Best single-detector avg F1 | **0.746** (scanmw) | 0.400 | 0.500 | **EXCEEDS STRETCH** |
| Detectors with avg F1 > 0.1 | **5** (scanmw, scanwelch, edivisive, mw, bocpd) | 4 | 6 | **EXCEEDS TARGET** |
| Detectors with avg F1 > 0.3 | **2** (scanmw, scanwelch) | 2 | 3 | **TARGET MET** |
| Best pagerduty F1 | **0.974** (scanmw) | 0.400 | 0.600 | **EXCEEDS STRETCH** |
| Best postmark F1 | **0.526** (scanmw/scanwelch) | 0.300 | 0.450 | **EXCEEDS STRETCH** |
| Best food_delivery F1 | **0.739** (scanmw) | 0.500 | 0.800 | **BETWEEN TARGET AND STRETCH** |

## CRITICAL NOTE: Isolation Required

All evals must use `-disable bocpd,rrcf` to prevent cross-detector interference from
default-enabled detectors. Without isolation, scores drop 3-5x due to the time_cluster
correlator creating many false periods from mixed detector anomalies.

## All Variations Tried (Sessions 2+3)

| # | Detector | Variation | food_del F1 | postmark F1 | pagerduty F1 | Avg F1 | Keep? |
|---|---|---|---|---|---|---|---|
| 1 | edivisive | V1-default (PF=8,MRC=2) | 0.222 | 0.325 | 0.129 | 0.225 | Superseded |
| 2 | robustscan | V1-default (MinDev=3) | 0.090 | — | — | — | Deprioritize |
| 3 | scanmw | V1-default (p<1e-6,eff>0.8,dev>2.5) | 0.510 | 0.526 | 0.561 | 0.532 | Superseded |
| 4 | edivisive | V2-selective (PF=12,MRC=4) | 0.333 | 0.351 | 0.187 | 0.290 | Keep |
| 5 | scanmw | V2-relaxed (p<1e-5,eff>0.7,dev>2.0) | 0.148 | — | — | — | Revert |
| 6 | **scanmw** | **V3-strict (p<1e-8,eff>0.85,dev>3.0)** | **0.739** | **0.526** | **0.974** | **0.746** | **CHAMPION** |
| 7 | scanmw | V4-minseg8 | 0.739 | 0.526 | — | — | Same as V3 |
| 8 | scanmw | V5-onset | 0.597 | 0.649 | — | — | Revert |
| 9 | scanmw | V6-ultrastrict | 0.000 | — | — | — | Revert |
| 10 | scanmw | V7-conserv-onset | 0.597 | — | — | — | Revert |
| 11 | mannwhitney | V3-relaxed (eff>0.9,dev>3) | 0.358 | 0.038 | 0.248 | 0.215 | Keep |
| 12 | edivisive | V3-ultraselective (PF=16,MRC=6) | 0.487 | 0.000 | — | — | Revert |
| 13 | edivisive | V4-middle (PF=14,MRC=5) | 0.400 | 0.351 | 0.072 | 0.274 | Revert |
| 14 | edivisive | V5 (MRC=3.5) | 0.139 | — | — | — | Revert (too many FPs) |
| 15 | edivisive | V6 (MRC=4,CohenD>2) | 0.149 | — | — | — | Revert |
| 16 | edivisive | V7 (PF=11,MRC=4) | 0.162 | — | — | — | Revert |
| 17 | edivisive | V8 (MinSeg=12,PF=14,MRC=3.5) | 0.090 | — | — | — | Revert |
| 18 | scanwelch | V1 (t>6,d>1.5,MAD>3) | 0.068 | — | — | — | Superseded |
| 19 | scanwelch | V2 (t>10,d>2.5,MAD>4) | 0.092 | — | — | — | Superseded |
| 20 | scanwelch | V3 (t>20,d>4,MAD>5) | 0.172 | — | — | — | Superseded |
| 21 | scanwelch | V4 (t>50,d>5,MAD>6) | 0.163 | — | — | — | Superseded |
| 22 | **scanwelch** | **V5-hybrid (t>8,MW-p<1e-8,eff>0.85,MAD>3)** | **0.493** | **0.526** | **0.945** | **0.655** | **KEEP** |
| 23 | scanks | V1 (D>0.5,MAD>3,eff>0.8) | 0.058 | — | — | — | Deprioritize |

NOTE: Rows 14-23 used NON-ISOLATED evals initially (scores shown are from interfered runs).
Rows 14-17 (E-Divisive) and 22-23 were later re-confirmed in isolated mode.
