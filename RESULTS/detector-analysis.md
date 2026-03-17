# Observer Changepoint Detector Analysis

**Date**: 2026-03-17
**Branch**: `ella/changepoint-detection-results-2`

## Complete Scored Detector Table

| Rank | Detector | Avg F1 | food_del | postmark | pagerduty | Type | Interface | Stateful? | Runtime per Detect() |
|------|----------|--------|----------|----------|-----------|------|-----------|-----------|---------------------|
| 1 | **ScanMW** | **0.746** | 0.739 | 0.526 | 0.974 | Scan (re-scan segment) | Detector (streaming) | Segment cursor only | O(n log n) per segment — re-reads full segment from storage on each call even if only 1 new point arrived |
| 2 | **ScanWelch** | **0.655** | 0.493 | 0.526 | 0.945 | Hybrid scan (Welch + MW verify) | Detector (streaming) | Segment cursor only | O(n) candidate scan + O(n log n) MW verify at best split — same re-read-from-storage pattern as ScanMW |
| 3 | **WinComp** | **0.509** | 0.630 | 0.666 | 0.232 | Sliding window comparison | SeriesDetector (batch) | fired count only | O(n) per series via cumsum — but batch interface means adapter re-reads ALL history every call |
| 4 | E-Divisive | 0.290 | 0.333 | 0.351 | 0.187 | Log-variance scan | Detector (streaming) | Segment cursor only | O(n) per segment via cumsum — same re-read pattern as ScanMW |
| 5 | MannWhitney | 0.215 | 0.358 | 0.038 | 0.248 | Fixed baseline vs sliding window | Detector (streaming) | Baseline + circular buffer | O(n log n) per new point (sort for MW test on every point when buffer full) |
| 6 | ~~EWM~~ | ~~0.176~~ | ~~0.527~~ | ~~0.000~~ | ~~0.000~~ | ~~Exponentially weighted mean/var~~ | ~~Detector (streaming)~~ | ~~Running mean/var per series~~ | ~~O(1) per point — abandoned~~ |
| 7 | BOCPD | 0.164 | 0.179 | 0.195 | 0.119 | Bayesian posterior over run lengths | Detector (streaming) | Full posterior (~9.6 KB/series) | O(MaxRunLength=200) per new point — truly incremental, only processes new points |
| 8 | Adaptive CUSUM | ~0.143 | 0.143 | — | — | Bilateral CUSUM with learned baseline | Detector (streaming) | Cumulative sum + baseline stats | O(1) per point |
| 9 | PELT | ~0.115 | 0.115 | — | — | Penalized exact linear time | Detector (streaming) | Segment costs | O(n) amortized per segment |
| 10 | CorrShift | 0.087 | 0.211 | 0.049 | 0.000 | Correlation matrix Frobenius norm | Detector (batch-like) | Recent correlation norms | O(MaxSeries² × WindowSize × nWindows) — most expensive detector |
| 11 | CUSUM | 0.000 | 0.000 | 0.000 | 0.000 | Classic Page's CUSUM | SeriesDetector (batch) | None (stateless batch) | O(n) per series — batch, recomputes from scratch |
| 12 | RRCF | 0.000 | 0.000 | 0.000 | 0.000 | Random cut forest (multivariate) | Detector (streaming) | Full forest (~large) | O(NumTrees × TreeSize × log TreeSize) per shingle |
| 13 | TopK | 0.000 | 0.000 | 0.000 | 0.000 | Rank all series by change severity | Detector (batch-like) | fired map | O(total_series × n_points) — reads ALL series every call |

## Per-Detector Deep Dive

---

### 1. ScanMW — Avg F1: 0.746 (Champion)

**File**: `comp/observer/impl/metrics_detector_scanmw.go`

**How it works**: Assigns ranks to all points in the current segment via sorting (O(n log n)), then slides a split point through, incrementally updating the rank sum. At each position computes the Mann-Whitney U statistic and z-score. Picks the split with max |z|. Verifies with three filters: p-value < 1e-8, rank-biserial effect size > 0.85, MAD deviation > 3.0. After detection, advances segment start to the changepoint timestamp so only post-change data is scanned going forward.

**Key parameters**: MinSegment=12, MinPoints=30, SignificanceThreshold=1e-8, MinEffectSize=0.85, MinDeviationMAD=3.0

**Strengths**:
- Fully non-parametric — no distributional assumptions
- Rank-based — inherently robust to outliers
- Triple-filter verification gives excellent selectivity
- Cleanest implementation in the codebase (no bugs found)
- Segment advancement enables multi-changepoint detection

**Weaknesses**:
- **Re-reads full segment from storage on each call** — if 100 points have accumulated since the last changepoint and 1 new point arrives, ScanMW re-reads and re-scans all 101 points. Compare with BOCPD which processes only the 1 new point. The segment shrinks after each detection (segment start advances), so it's bounded, but it's still redundant work re-analyzing data it already saw.
- Normal approximation for p-values may be inaccurate for small segments (n < 20)
- O(n log n) per segment from sorting

**Edge cases it misses**:
- Gradual drift (no level shift = no rank separation)
- Variance-only changes (mean unchanged, spread doubles)
- Very short-lived spikes that don't sustain across MinSegment points
- Seasonal patterns that look like level shifts at cycle boundaries

**Eval notes**: Near-perfect on food_delivery (0.739) and pagerduty (0.974) — only 1 scored prediction on pagerduty. Postmark (0.526) is harder because the shifts are subtler. The triple filter keeps false positives extremely low.

---

### 2. ScanWelch — Avg F1: 0.655 (2nd Best)

**File**: `comp/observer/impl/metrics_detector_scanwelch.go`

**How it works**: Hybrid two-phase approach. Phase 1: scans all split points using Welch's t-statistic with cumulative sums (O(n)) to find the best candidate. Phase 2: verifies the single best candidate using Mann-Whitney p-value, effect size, and MAD deviation. Combines parametric speed (t-test is optimal for Gaussian mean shifts) with non-parametric verification (MW provides distribution-free confidence).

**Key parameters**: MinSegment=12, MinPoints=30, MinTStatistic=8.0, SignificanceThreshold=1e-8, MinEffectSize=0.85, MinDeviationMAD=3.0

**Strengths**:
- O(n) candidate selection (faster than ScanMW's O(n log n))
- Parametric detection is highly sensitive to mean shifts
- Non-parametric verification compensates for Gaussian assumption
- Best selectivity: only 1/1/3 scored predictions across scenarios in our eval
- No bugs found — equally clean implementation

**Weaknesses**:
- Welch's t-test in Phase 1 assumes approximately normal distributions
- Less powerful than pure MW for detecting non-location-shift changes (e.g., scale changes)
- **Same re-read-from-storage pattern as ScanMW** — re-reads and re-scans the full segment even when only 1 new point arrived. Not truly incremental.

**Edge cases it misses**:
- Same as ScanMW (gradual drift, variance-only, seasonal)
- Non-Gaussian shifts where the t-test has low power but MW would catch them (rare in practice)
- Changes in distribution shape without mean shift

**Eval notes**: Best precision across all scenarios. food_delivery 0.493 (lower than ScanMW because t-test is less powerful on the specific redis spike shape), but pagerduty 0.945 with only 3 scored predictions (vs ScanMW's 1 in batch). The hybrid approach sacrifices some sensitivity for better selectivity.

---

### 3. WinComp — Avg F1: 0.509 (Best New, Session 3)

**File**: `comp/observer/impl/metrics_detector_wincomp.go`

**How it works**: Slides a split point through the series with two adjacent fixed-size windows (W=30 points each). At each position, computes Welch's t-statistic between the left window [k-W, k) and right window [k, k+W) using cumulative sums. Picks the split with max |t|. Verifies with MAD deviation (>5.0) and rank-biserial effect size (>0.85). Can detect up to MaxFires=2 changepoints per series by advancing past each detection.

**Key parameters**: WindowSize=30, MinTStat=8.0, MinDeviationMAD=5.0, MaxFires=2, MinEffectSize=0.85

**Strengths**:
- O(n) scan with cumulative sums — very efficient
- Fixed window size means no bias toward series midpoint (unlike full-segment scans)
- Multi-changepoint detection within a single call
- Best postmark score (0.666) of any detector — fixed windows handle subtle shifts well

**Weaknesses**:
- **Batch SeriesDetector interface** — wrapped by adapter, re-processes full history
- **Missing Anomaly metadata fields** (Type, Source, SourceSeriesID, DetectorName) — would break downstream processing
- `break` on verification failure (lines 209, 230) stops scanning entirely — misses later valid changepoints if an intermediate one fails verification
- Parametric candidate selection (Welch's t)
- **Critical dependency on time_cluster WindowSeconds** — scores swing wildly (0.000 to 0.666 on postmark) depending on TC window. TC180 is the sweet spot but this is fragile.

**Edge cases it misses**:
- Changes that span more than WindowSize points (gradual transitions)
- Changes near the start/end of series (need W points on each side)
- Variance-only changes
- Anything the verification `break` prematurely terminates scanning for

**Eval notes**: The TC180 dependency is a red flag. At TC≤130, postmark=0.000; at TC≥150, pagerduty drops from 0.597 to 0.232. The detector's actual detection is good but its interaction with the correlator is brittle. The 0.666 postmark score is the best of any detector on the hardest scenario.

---

### 4. E-Divisive — Avg F1: 0.290

**File**: `comp/observer/impl/metrics_detector_edivisive.go`

**How it works**: Scans all split points and finds the one that maximizes the reduction in total log-variance. The gain at split k is: `n*log(var_total) - k*log(var_left) - (n-k)*log(var_right)`. Applies a penalty of PenaltyFactor * log(n) to prevent overfitting. Verifies with MAD-based relative change filter.

**Key parameters**: MinSegment=15, MinPoints=30, PenaltyFactor=12.0, MinRelativeChange=4.0

**Critical note**: Despite the name "E-Divisive", this does NOT implement the energy statistic from Matteson & James (2014). It implements Gaussian binary segmentation (log-variance cost). The nonparametric claim in the docstring is incorrect.

**Strengths**:
- O(n) scan with cumulative sums
- Information-theoretic penalty has some theoretical grounding
- Segment advancement for multi-changepoint detection

**Weaknesses**:
- Mislabeled algorithm — Gaussian cost, not energy statistic
- Gaussian optimality criterion poor fit for non-Gaussian production metrics
- Single verification filter (MAD only) vs triple filter on ScanMW/ScanWelch

**Edge cases it misses**:
- Subtle mean shifts where variance is similar on both sides (log-variance gain is small)
- Non-Gaussian distributions where the Gaussian cost is suboptimal
- Postmark entirely (F1=0.000 — single scored prediction is a pure FP)

**Eval notes**: Decent on food_delivery (0.333) and pagerduty (0.187) but total failure on postmark (0.000). The single-filter verification lets too many false positives through on pagerduty (8 scored). Superseded by ScanMW and ScanWelch in every dimension.

---

### 5. MannWhitney — Avg F1: 0.215

**File**: `comp/observer/impl/metrics_detector_mannwhitney.go`

**How it works**: Streaming detector with a fixed baseline window (first 30 points after warmup) and a sliding recent window (circular buffer of 30 points). On each new point, updates the recent buffer. When the buffer is full, runs the Mann-Whitney U test comparing baseline vs recent. Uses 4-layer filter: p-value, effect size, MAD deviation, relative change. Has alert lifecycle with recovery.

**Key parameters**: MinPoints=10, WindowSize=30, WarmupPoints=30, SignificanceThreshold=1e-8, MinEffectSize=0.90, MinDeviationSigma=3.0, MinRelativeChange=0.20

**Strengths**:
- Non-parametric, robust to outliers
- 4-layer filter is theoretically sound
- Alert lifecycle with recovery prevents single-event storms

**Weaknesses**:
- **Fixed baseline never updates** — a metric that legitimately changes level triggers alerts forever (58 cascading filtered on postmark)
- **Missing series caching** — calls `ListSeries()` every Detect() without caching (line 129), unlike BOCPD/ScanMW/ScanWelch. At 95k+ series, significant overhead.
- O(n log n) sort per point per series (allocates new slice every call)
- Small window sizes (30) reduce statistical power

**Edge cases it misses**:
- Any legitimate regime change after the baseline window is set (fixed baseline = permanent alerts)
- Slow drift within the window size
- Seasonal patterns

**Eval notes**: food_delivery 0.358 is passable, but postmark 0.038 exposes the fundamental flaw — the fixed baseline produces runaway cascading alerts. Superseded by ScanMW (same statistical test but scans all split points).

---

### 6. EWM (Exponentially Weighted Mean) — ABANDONED

**File**: Removed.

**How it works**: Two variants were evaluated:
- **V1 (single-threshold)**: Single exponential mean/variance, z-score threshold. Avg F1=0.176 (food 0.527, postmark 0.000, pagerduty 0.000).
- **V2 (dual-timescale)**: Slow EWM (alpha=0.01) tracks baseline, fast EWM (alpha=0.10) tracks recent. Detects when fast and slow diverge. Added: frozen baseline stddev from warmup, slow EWM freeze on divergence, MinVariance=1.0 floor, 5% relative change filter. Avg F1=0.112 (food 0.147, postmark 0.136, pagerduty 0.053).

**Why abandoned**: V2 fixed the false positive problem (pagerduty: 11 FP→0, postmark: 17→1, food: 14→2) but recall remained poor. The fundamental issue is that exponential smoothing is inherently sluggish — it takes multiple points for the fast EWM to diverge enough, causing detection delay that kills temporal scoring. The architecture (O(1) per point, bounded memory) is attractive but the detection quality ceiling is too low. Both variants score well below BOCPD (0.164) on the hardest scenarios, and BOCPD already serves the "stateful incremental" role better with a principled probabilistic model.

---

### 7. BOCPD — Avg F1: 0.164

**File**: `comp/observer/impl/metrics_detector_bocpd.go`

**How it works**: Adams & MacKay (2007) Bayesian Online Changepoint Detection. Maintains a posterior distribution over run lengths — how long since the last changepoint. At each new observation, updates all run-length hypotheses using a normal-normal conjugate model. Triggers when either changepoint probability > 0.95 or short-run posterior mass > 0.95. Has warmup phase (120 points) using Welford's algorithm.

**Key parameters** (tuned): WarmupPoints=120, Hazard=0.005, CPThreshold=0.95, CPMassThreshold=0.95, MaxRunLength=200

**Strengths**:
- Truly online — O(1) amortized per point (O(MaxRunLength) technically)
- Principled probabilistic framework with uncertainty quantification
- Dual trigger (peak probability + mass) catches sudden spikes and sustained shifts
- Recovery mechanism prevents alert storms
- Pre-allocated swap buffers avoid per-point allocation

**Weaknesses**:
- Gaussian assumption — normal-normal conjugate model is a poor fit for skewed/heavy-tailed production metrics
- Fixed baseline from warmup (baselineMean/baselineStddev) never updates
- 120-point warmup = 2 minutes of blindness at startup
- Run-length truncation at 200 limits regime representation (~3 min at 1Hz)
- Even after aggressive tuning, precision remains terrible (0.08-0.33)

**Edge cases it misses**:
- Non-Gaussian changepoints (the Gaussian likelihood fires on normal outliers)
- Gradual drift (posterior adapts slowly)
- Very large datasets (fires 10+ predictions on 95k-series pagerduty)

**Eval notes**: High recall (0.7-0.97 baseline) but catastrophic precision. Tuning from 0.05/0.6/0.7 to 0.005/0.95/0.95 helped significantly (+0.112 avg F1) but precision remains the weakest of any evaluated detector. Postmark 0.444 (tuned) is its best result.

---

### 8. Adaptive CUSUM — Avg F1: ~0.143

**File**: `comp/observer/impl/metrics_detector_adaptive_cusum.go` (from headless session)

**How it works**: Bilateral CUSUM with online-learned baseline using median + MAD. Warmup phase accumulates statistics, monitoring phase runs two-sided cumulative sums against the learned baseline. After detection, resets and re-enters warmup for the new regime.

**Key parameters** (V3-strict): slack=1.5, threshold=10, MAD>8

**Strengths**:
- Regime-resetting addresses classic CUSUM's fixed-baseline problem
- Median/MAD baseline is robust to outliers
- Bilateral detection catches both increases and decreases

**Weaknesses**:
- Required very aggressive filtering (MAD>8) to get any non-zero F1
- Only evaluated on food_delivery
- Re-warmup period after detection creates blind spots

**Edge cases it misses**: Not enough eval data to characterize. food_delivery-only suggests it struggles with subtle shifts.

---

### 9. PELT — Avg F1: ~0.115

**File**: `comp/observer/impl/metrics_detector_pelt.go` (from headless session)

**How it works**: Penalized Exact Linear Time changepoint detection. Maintains a set of candidate changepoint locations and prunes those that can't improve the penalized cost. Uses a Gaussian cost function.

**Key parameters** (V4): PenaltyFactor=2.0, MinSegment=12, MAD>10

**Strengths**:
- Theoretically optimal for Gaussian data
- Can find multiple changepoints efficiently

**Weaknesses**:
- Had bugs in early versions (V1-V3 produced 0 changepoints)
- Required extreme MAD threshold (>10) to produce non-zero F1
- Only evaluated on food_delivery

**Edge cases it misses**: Same Gaussian limitations as E-Divisive. Insufficient eval data.

---

### 10. CorrShift — Avg F1: 0.087

**File**: `comp/observer/impl/metrics_detector_corrshift.go`

**How it works**: Tracks pairwise Pearson correlation matrices across sliding windows. Computes Frobenius norm of the difference between consecutive windows and a baseline average. Flags when norm exceeds threshold. Reports individual series from the most-changed correlation pairs.

**Key parameters**: WindowSize=15, StepSize=3, MaxSeries=80

**Strengths**:
- Detects structural changes in metric relationships (unique capability)
- Dual comparison (consecutive + baseline)

**Weaknesses**:
- O(MaxSeries² × windows) — prohibitive beyond ~100 series
- Pearson correlation sensitive to outliers
- Forward-fill alignment propagates stale values
- `firedSeries` dedup map pruned by complete replacement at 10k entries (burst of duplicates)

**Edge cases it misses**:
- Pagerduty entirely (F1=0.000) — scale kills it
- Changes in series not in the top-80 by variance
- Any change that doesn't alter pairwise correlations

---

### 11-13. CUSUM, RRCF, TopK — Avg F1: 0.000

All three produce zero scored output in streaming eval.

**CUSUM**: Fires during warmup, all predictions filtered. Missing Anomaly metadata fields. Legacy code.

**RRCF**: All-or-nothing metric alignment — zero output if any configured metric missing. Unbounded `allScores` memory growth. Would need metric auto-discovery to work with scenario data.

**TopK**: O(total_series × n_points) per call — reads every series. Conceptually a ranking/filtering layer, not a standalone detector. Service diversity bonus fabricates timestamps.

---

## Scenario Characteristics

| Scenario | Series | Difficulty | Best Detector | What Makes It Hard |
|----------|--------|------------|---------------|-------------------|
| food_delivery_redis | ~28k | Easy | ScanMW (0.739) | Short series (~130 pts), clear level shift |
| 353_postmark | ~25k | Hard | WinComp (0.666) | Subtle correlation shifts, not obvious level changes |
| 213_pagerduty | ~95k | Medium-Hard | ScanMW (0.974) | Sheer volume — noisy detectors produce thousands of FPs |

## Detection Gaps (No Detector Covers)

| Gap | Description | Impact |
|-----|-------------|--------|
| Gradual drift | Slowly increasing latency over 10+ min | Missed until it becomes a level shift |
| Seasonality | Periodic patterns (hourly, daily cycles) | False positives at cycle boundaries |
| Multi-modal distributions | Metrics alternating between modes (batch on/off) | Confuses all detectors |
| Variance-only changes | Mean constant but spread doubles (instability) | Missed by all except partially RRCF |
| Short transient spikes | <MinSegment duration spikes | Filtered by minimum segment requirements |
| Correlated multi-series shifts | Incident affects 50 series simultaneously | Each detected independently, no joint modeling |

## Final Selection

### Keep (3 detectors)

| Detector | Avg F1 | Role | Interface | Why selected |
|----------|--------|------|-----------|--------------|
| **ScanMW** | 0.746 | Primary — best overall F1 | Detector (streaming) | Champion. Non-parametric, rank-based, triple-filter verification. Best on pagerduty (0.974) and food_delivery (0.739). |
| **ScanWelch** | 0.655 | Primary — best precision | Detector (streaming) | Hybrid Welch+MW is the most selective detector (fewest scored predictions). Best balance across all 3 scenarios. |
| **BOCPD** | 0.164 | Stateful — lowest latency | Detector (streaming) | Only detector with a principled probabilistic model and per-point posterior updates. Detects faster than any scan-based detector. Precision is weak but the framework is sound. |

### Abandon (10 detectors)

| Detector | Avg F1 | Reason for abandonment |
|----------|--------|----------------------|
| WinComp | 0.509 | **SeriesDetector (batch)** — wrapped by adapter, re-reads all history every call. Good scores but wrong interface. Would need full streaming conversion to keep, and ScanWelch already covers the same Welch+verification approach natively. |
| E-Divisive | 0.290 | Mislabeled (Gaussian binary segmentation, not energy statistic). F1=0.000 on postmark. Superseded by ScanMW/ScanWelch. |
| MannWhitney | 0.215 | Fixed baseline never adapts (58 cascading on postmark). Missing series caching. Superseded by ScanMW which uses the same statistical test but scans all splits. |
| EWM | 0.176→0.112 | Two variants evaluated (single-threshold, dual-timescale). V2 fixed FP problem but recall remained poor. Exponential smoothing too sluggish for precise temporal detection. BOCPD serves the stateful incremental role better. |
| Adaptive CUSUM | ~0.143 | Required extreme filtering (MAD>8) for any non-zero F1. Only evaluated on food_delivery. Insufficient improvement over raw CUSUM to justify. |
| PELT | ~0.115 | Had bugs in V1-V3. Best version (V4) still only 0.115 on food_delivery. Gaussian cost function same as E-Divisive. |
| CorrShift | 0.087 | O(n²) in series count — unusable at production scale. Pagerduty=0.000. |
| CUSUM | 0.000 | Legacy. Fires during warmup, all predictions filtered. Missing Anomaly metadata fields. |
| RRCF | 0.000 | Zero output due to hardcoded metric names not in scenario data. Unbounded memory growth. Would need substantial rework. |
| TopK | 0.000 | O(total_series × n_points) per call. Not a detector — more of a ranking layer. Fabricates timestamps. |

### Rationale

The three selected detectors form two complementary tiers:

**Scan pair (ScanMW + ScanWelch)**: High F1, high precision, proven on all 3 scenarios. ScanMW is fully non-parametric (catches distribution-shape changes); ScanWelch is more selective (fewer false positives). Together they cover both sensitivity and specificity. Downside: both re-read segments from storage on every call — not truly incremental.

**Stateful (BOCPD)**: Truly incremental (processes each point once), bounded memory, low latency. The only detector with a principled probabilistic model and per-point posterior updates. Precision is weak but the framework is sound for a production agent that processes metrics continuously. EWM was evaluated as a lighter alternative but its detection quality ceiling is too low — exponential smoothing is fundamentally sluggish for changepoint timing.

**Why not WinComp**: Despite the best postmark score (0.666), WinComp implements `SeriesDetector` (batch). The `seriesDetectorAdapter` wraps it to satisfy the `Detector` interface, but this means re-reading the entire series history on every call — exactly the pattern we converted ScanMW/ScanWelch/E-Divisive away from. ScanWelch already covers the same Welch+verification approach with a proper streaming interface.
