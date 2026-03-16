# Iteration Results — Streaming Eval

Last updated: 2026-03-13 20:52

## Current Best Scores

| Detector | Best Variation | food_delivery | postmark | pagerduty | Avg F1 | Delta vs Baseline |
|---|---|---|---|---|---|---|
| **bocpd** | **V3 (H=0.005,CPT=0.95)** | **0.224** | **0.444** | **0.140** | **0.269** | **+0.116** |
| bocpd | V2 (H=0.01,CPT=0.9) | 0.199 | 0.333 | 0.140 | 0.224 | +0.071 |
| bocpd | V1 (H=0.01,CPT=0.8) | 0.179 | 0.333 | 0.129 | 0.214 | +0.061 |
| mannwhitney | V3 | 0.121 | 0.149 | 0.140 | **0.137** | +0.137 |
| corrshift | baseline | 0.211 | 0.049 | 0.000 | 0.087 | — |
| cusum | best (V3) | 0.000 | — | — | ~0.000 | +0.000 |
| rrcf | best (V3) | 0.000 | — | — | ~0.000 | +0.000 |
| topk | best (V4) | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 |

## Best Single-Detector Result: BOCPD V3 — Avg F1 = 0.269

## All Variations Tried

| # | Detector | Variation | food_delivery | postmark | pagerduty | Avg F1 | Keep? |
|---|---|---|---|---|---|---|---|
| 1 | bocpd | baseline | 0.146 | 0.195 | 0.119 | 0.153 | — |
| 2 | bocpd | V1 (H=0.01,CPT=0.8,CPM=0.8) | 0.179 | 0.333 | 0.129 | 0.214 | Superseded |
| 3 | bocpd | V2 (H=0.01,CPT=0.9,CPM=0.9) | 0.199 | 0.333 | 0.140 | 0.224 | Superseded |
| 4 | **bocpd** | **V3 (H=0.005,CPT=0.95,CPM=0.95)** | **0.224** | **0.444** | **0.140** | **0.269** | **BEST** |
| 5 | bocpd | V4 (H=0.002,CPT=0.98,CPM=0.98) | 0.224 | — | — | — | Diminishing |
| 6 | mannwhitney | V3 (Win=15,MinPts=5,Sig=1e-4) | 0.121 | 0.149 | 0.140 | 0.137 | Yes |
| 7 | mannwhitney | V4 (stricter filters) | 0.000 | — | — | — | Revert |
| 8 | cusum | V2 (TF=8.0,median/MAD) | 0.000 | — | — | — | No |
| 9 | cusum | V3 (TF=12,WP=60) | 0.000 | — | — | — | No |
| 10 | topk | V1-V4 (various) | 0.000 | 0.000 | 0.000 | 0.000 | No |
| 11 | rrcf | V1-V3 (various) | 0.000 | — | — | — | No |
