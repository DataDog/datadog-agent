# Research Notes — Sessions 2+3 (2026-03-13/14)

Focus: NEW algorithm implementations

---

## Session Plan (19:50)

Human feedback directs: all new detectors as SeriesDetector (batch), adapter handles streaming. 80%+ time on new algorithms.

Algorithms implemented:
1. **E-Divisive** — Gaussian log-likelihood variance scan. Nonparametric.
2. **ScanMW** — Mann-Whitney U scan. Nonparametric rank-based.
3. **RobustScan** — Welch t-statistic scan + median/MAD verification.
4. **ScanWelch** — Hybrid: Welch t-stat for split finding + MW verification for selectivity.
5. **ScanKS** — Kolmogorov-Smirnov scan. (Poor results, deprioritized.)


## Key Finding: Scan-based approaches dominate fixed-window approaches (22:15)

The ScanMW detector's success demonstrates a fundamental insight: **scanning all possible split points** for the optimal changepoint location is far superior to using a fixed baseline window.

Comparison:
- Fixed-window MW (existing): F1=0.000 (wrong split, wrong timing)
- MW V3 (relaxed thresholds): avg 0.215 (fires but imprecise)
- ScanMW V3 (scan all splits): avg 0.746 (near-optimal splits)

## Key Finding: Triple filter is the selectivity mechanism (22:15)

ScanMW's three-layer filter (p-value + effect size + deviation) is what makes it precise:
- p-value < 1e-8: statistical significance (eliminates noise)
- effect size > 0.85: practical significance (eliminates tiny changes)
- deviation > 3.0 MADs: robust magnitude check (eliminates outlier-driven false positives)

All three are necessary. Relaxing any one degrades F1 dramatically.

## Key Finding: Selectivity plateau (22:15)

There's a sweet spot for selectivity:
- V1 (moderate): avg 0.532
- V3 (strict): avg 0.746
- V6 (ultra-strict): avg 0.000

V3 hits the optimal tradeoff. Going stricter kills true positives.

## Key Finding: Parametric tests are less selective than nonparametric (Session 3)

All parametric approaches (Welch's t-test, variance gain, KS) produce more false positives
than rank-based Mann-Whitney, even with very strict thresholds. This is because metrics
data has heavy tails and non-normal distributions. Ranks are robust to outliers.

- ScanWelch pure (V1-V4): best F1=0.172 on food_delivery
- ScanKS V1: F1=0.058 on food_delivery
- E-Divisive (variance-based): F1=0.333 on food_delivery

## Key Finding: Hybrid approach works — t-test split + MW verification (Session 3)

ScanWelch V5 combines Welch's t-test for split point selection with MW p-value/effect
size/deviation verification. This achieves avg F1=0.655:
- food_delivery: 0.493 (t-test finds slightly different split than MW)
- postmark: 0.526 (same split as ScanMW)
- pagerduty: 0.945 (nearly identical to ScanMW)

The t-test finds the split point that maximizes parametric mean difference, then MW
verifies it's a real distributional change. This is the second-best detector.

## CRITICAL Finding: Cross-detector interference (Session 3)

The testbench's `-enable` flag is ADDITIVE, not exclusive. BOCPD and RRCF are
default-enabled in the testbench catalog. Running `-enable scanmw` without
`-disable bocpd,rrcf` causes all three detectors to run simultaneously.

This creates cross-detector interference through the time_cluster correlator:
- Isolated ScanMW: 1 anomaly period on food_delivery → F1=0.739
- Non-isolated: 38 anomaly periods (from BOCPD+RRCF+ScanMW) → F1=0.179

**Always use `-disable bocpd,rrcf` for isolated evaluation.**

## ScanMW implementation note (22:15)

The O(n log n) incremental rank computation (via pre-sorting) is essential for performance.
The naive O(n^2 log n) approach would be too slow for 95K series on pagerduty.

## ScanWelch implementation note (Session 3)

Uses O(n) cumulative-sum based t-statistic computation for the scan phase, then O(n log n)
MW verification at the best split only. This is faster than ScanMW (which does O(n log n)
ranking for all splits) while achieving competitive results.
