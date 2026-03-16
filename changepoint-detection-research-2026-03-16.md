# Changepoint Detection — Headless Agent Iteration Plan

**Date**: 2026-03-16 (revised)
**Branch**: `ella/changepoint-detection-results-2`
**Objective**: **Implement and evaluate NEW stateful changepoint detection algorithms.** Do NOT tune existing detectors — the goal is to write new detector code. We want detectors that maintain learned per-series representations (like BOCPD's posterior distribution), as opposed to the scan-based detectors (ScanMW, ScanWelch, E-Divisive) which re-scan raw data on each call.

**Mandate**: You MUST spend your time implementing new algorithms, not tuning parameters on existing ones. Tuning is done. The existing detectors are frozen. Write new Go code for new detection approaches.

**Current results**: `RESULTS/streaming-v2/comparison-matrix.md`

---

## 0. Headless Execution Rules

This plan is designed for autonomous execution. Follow these rules without exception.

### Autonomy

**Execute all steps autonomously without waiting for user approval.** Do not pause for confirmation between iterations. Make your own judgment calls about what to try next. If you hit ambiguity, make a decision, log your reasoning in `research-notes.md`, and keep going.

### Exit Condition

Stop when **any** of these are true:
- **Hard time limit (5 hours)**: At the very start of your session, record the current time with `date +%s` and save it to `iteration-start-time.txt`. Before starting each new variation, check elapsed time with `echo $(( $(date +%s) - $(cat iteration-start-time.txt) ))`. If more than 18000 seconds (5 hours) have elapsed, **stop immediately** — do not start another variation. Use the remaining time only to write your final summary.
- **Diminishing returns**: Your last 3 consecutive variations (across any detectors) each improved the best avg F1 by less than 0.005
- **All approaches explored**: You've attempted at least 3 stateful detector designs and no approach has obvious remaining headroom
- **Target reached**: A new stateful detector achieves avg F1 > 0.500

When you stop (for any reason), write a final summary entry in `iteration-log.md` with the heading `## FINAL SUMMARY` that includes the final results table, what worked best, and what you'd try next if continuing.

### Error Handling

- If a testbench run **hangs for more than 30 minutes** (pagerduty can legitimately take 27 min, so use 30 as the cutoff), kill it. Log the timeout in `iteration-log.md` and `research-notes.md`, then move on.
- If a build fails, **fix the build error** (you are allowed to modify code). Log the error and fix in `research-notes.md`.
- If a scorer run fails or produces garbled output, log the raw output and skip that scenario. Do not block on it.
- **Never let a single failure stop the whole iteration loop.** Log it and continue with the next detector or variation.

### Feedback Loop

Before starting each new iteration cycle, **check the file `human-feedback.md`** in the repo root. This is a document where the human reviewer will write feedback for you to incorporate — things like "stop working on X", "try Y approach instead", "focus on Z detector", etc.

- If `human-feedback.md` does not exist, create it as a blank file with a header:
  ```markdown
  # Human Feedback
  _Check this file before each iteration cycle. The human reviewer will add feedback here._
  ```
- If it contains new feedback you haven't acted on yet, incorporate it into your next iteration decision
- After reading and acting on feedback, append a note at the bottom: `> [YYYY-MM-DD HH:MM] Acknowledged: <brief summary of what you read and how you're adjusting>`

### What to Capture Per Eval Run

For every testbench + scorer run, record ALL of these in `iteration-log.md`:

| Metric | Source | Why |
|---|---|---|
| F1, Precision, Recall | scorer output | Primary metrics |
| num_scored | scorer output | How many predictions survived filters |
| num_warmup_filtered | scorer output | Detects "fires too early" problem |
| num_cascading_filtered | scorer output | Detects redundant firing |
| Total raw anomalies | scorer or verbose JSON | Selectivity indicator — fewer is usually better |
| Testbench wall-clock time | measure it (`time` or timestamps) | Track if changes cause perf regressions |

If a run OOMs or is killed, note the approximate memory usage if visible (e.g., from `dmesg` or process output).

### Scope

You ARE expected to modify detector implementations, tune parameters, write new detectors, and register them in the component catalog. The only things you should NOT do:
- Do not modify the scorer (`observer-scorer`) — it's the ground truth
- Do not modify scenario data (`comp/observer/scenarios/`)
- Do not modify the testbench harness itself (`testbench.go`) unless you have a clear engineering reason, and log your reasoning if you do
- Do not push to remote or create PRs — work locally on the branch

### Resource Constraints

- Each testbench run consumes ~3-5 GB RAM (parquet loading)
- Use caution when running evals in parallel. The machine has limited memory.
- Check with `vm_stat` or `top` before launching if unsure

---

## 1. Current Baseline (2026-03-16)

All scores from isolated per-detector streaming eval. Full details in `RESULTS/streaming-v2/comparison-matrix.md`.

| Detector | Type | food_delivery | postmark | pagerduty | Avg F1 |
|---|---|---|---|---|---|
| **scanwelch** | scan (re-scan) | 0.946 | 0.619 | 0.473 | **0.679** |
| **scanmw** | scan (re-scan) | 0.946 | 0.578 | 0.315 | **0.613** |
| **bocpd** (tuned) | **stateful** | 0.185 | 0.444 | 0.153 | **0.261** |
| edivisive | scan (re-scan) | 0.527 | 0.000 | 0.187 | 0.238 |
| mannwhitney | stateful (sliding window) | 0.358 | 0.038 | 0.248 | 0.215 |
| bocpd (baseline) | **stateful** | 0.146 | 0.195 | 0.105 | 0.149 |

### Key observation: stateful gap

The scan-based detectors (ScanWelch 0.679, ScanMW 0.613) significantly outperform the stateful ones (BOCPD 0.261, MannWhitney 0.215). But scan detectors re-read the entire segment on every call — they don't learn. Stateful detectors that accumulate a learned representation (like BOCPD's posterior) should theoretically be more efficient and potentially more accurate for sequential changepoint detection. The gap suggests opportunity.

### What "stateful" means here

BOCPD maintains a **posterior distribution** over run lengths — it processes each point once, updates its belief about when the last changepoint occurred, and emits when posterior mass concentrates on short run lengths. This is fundamentally different from ScanMW/ScanWelch which re-scan `[segmentStart, now]` looking for the best split point every time new data arrives.

The goal of this session is to explore **new stateful algorithms** that:
1. Process each data point incrementally (O(1) per point, not O(n) re-scan)
2. Maintain a compact learned state per series
3. Can detect multiple sequential changepoints
4. Achieve competitive F1 vs the scan-based detectors

---

## 2. How to Run Evals

### 2.1 Build

```bash
go build -o bin/observer-testbench ./cmd/observer-testbench && go build -o bin/observer-scorer ./cmd/observer-scorer
```

### 2.2 Run Testbench (per detector, per scenario)

**CRITICAL**: Always run detectors in **isolation** — enable only ONE detector plus `time_cluster`. Cross-detector interference in TimeCluster causes a 0.2-0.3 F1 penalty when multiple detectors are enabled.

```bash
# Template — replace <DETECTOR> and <SCENARIO>
./bin/observer-testbench \
  -headless <SCENARIO> \
  -output RESULTS/streaming-v2/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "<DETECTOR>,time_cluster" \
  -disable "cusum,bocpd,rrcf,mannwhitney,corrshift,topk,scanmw,scanwelch,edivisive,cross_signal,lead_lag,surprise,passthrough" \
  -verbose
```

Remove the detector you're testing from the `-disable` list.

**Scenarios**: `food_delivery_redis` (~8 min), `353_postmark` (~15-20 min), `213_pagerduty` (~20-27 min)

**Registered detectors** (in `component_catalog.go`): `cusum`, `bocpd`, `rrcf`, `mannwhitney`, `corrshift`, `topk`, `edivisive`, `scanmw`, `scanwelch`

**Registered correlators**: `cross_signal`, `time_cluster`, `lead_lag`, `surprise`, `passthrough`

### 2.3 Score Results

```bash
./bin/observer-scorer \
  -input RESULTS/streaming-v2/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -sigma 30.0
```

### 2.4 Quick Eval Loop

Always use food_delivery_redis first (~8 min). Only run full 3-scenario if promising.

```bash
DETECTOR="mydetector"
go build -o bin/observer-testbench ./cmd/observer-testbench && \
./bin/observer-testbench \
  -headless food_delivery_redis \
  -output "RESULTS/streaming-v2/eval-${DETECTOR}-food_delivery_redis.json" \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "${DETECTOR},time_cluster" \
  -disable "cusum,bocpd,rrcf,mannwhitney,corrshift,topk,scanmw,scanwelch,edivisive,cross_signal,lead_lag,surprise,passthrough" \
  -verbose && \
./bin/observer-scorer \
  -input "RESULTS/streaming-v2/eval-${DETECTOR}-food_delivery_redis.json" \
  -scenarios-dir ./comp/observer/scenarios \
  -sigma 30.0
```

**IMPORTANT**: Remove `$DETECTOR` from the `-disable` list.

---

## 3. Research Focus: NEW Stateful Detector Algorithms

**CRITICAL RULE**: Do NOT tune existing detectors. Every iteration must involve writing a NEW detector file or a substantially NEW algorithm within an existing file. Parameter sweeps on BOCPD, MannWhitney, ScanMW, etc. are explicitly out of scope. If you find yourself adjusting thresholds on an existing detector, STOP and implement a new algorithm instead.

### 3.1 What makes a good stateful detector

A stateful detector processes points incrementally and maintains a **learned representation** that captures the series' behavior. Examples of learned state:

- **BOCPD**: posterior over run lengths, per-run-length sufficient statistics (mean, precision)
- **CUSUM**: cumulative sum relative to a baseline — but too simple (no distributional learning)
- **EWM (Exponentially Weighted Moving)**: online mean/variance with exponential forgetting — simple but adaptive
- **Online kernel methods**: running MMD or KDE estimates of distribution shift

The ideal stateful detector:
1. **O(1) per point** — constant work to process a new observation
2. **Bounded memory** — fixed-size state regardless of series length
3. **Adaptive baseline** — learns what "normal" looks like and detects departure
4. **Self-resetting** — after detecting a change, resets to learn the new regime

### 3.2 Algorithm Ideas to Explore (Priority Order)

#### A. Online CUSUM with Learned Baseline — Priority: HIGH

CUSUM currently scores 0.000 because it fires too early (all predictions warmup-filtered). The fix is to make it truly adaptive:

- **Phase 1 (warmup)**: accumulate running median + MAD using Welford-like online algorithm
- **Phase 2 (monitoring)**: run bilateral CUSUM (detect both increases and decreases) against the learned baseline
- **Phase 3 (after fire)**: reset cumulative sum and re-enter a brief re-warmup to learn the new regime

This is essentially what BOCPD does but with a simpler accumulator (CUSUM) instead of a full Bayesian posterior. Should be faster and potentially more precise.

**Key parameters**: warmup length, threshold factor, reset warmup length

**File**: modify `metrics_detector_cusum.go` or create `metrics_detector_cusum_adaptive.go`

#### B. Exponentially Weighted Mean/Variance Detector — Priority: HIGH

A lightweight alternative to BOCPD: maintain exponentially-weighted running mean and variance. Detect changepoints when the current observation deviates significantly from the EWM prediction.

State per series:
```go
type ewmState struct {
    ewmMean     float64  // exponentially weighted mean
    ewmVar      float64  // exponentially weighted variance
    alpha       float64  // smoothing factor (e.g., 0.05)
    warmupCount int
    initialized bool
}
```

On each point:
1. Update EWM mean and variance
2. Compute z-score: `z = |x - ewmMean| / sqrt(ewmVar)`
3. If `z > threshold` for consecutive points, emit anomaly

This is similar to BOCPD's trigger mechanism but without the Bayesian posterior machinery. Much simpler, much faster.

**Key advantage**: Naturally adapts to slow drift without re-scanning. The exponential weighting "forgets" old data automatically.

**File**: create `metrics_detector_ewm.go`

#### C. Page's CUSUM with Regime Learning — Priority: MEDIUM

Classical Page's CUSUM but with two innovations:
1. **Pre-change and post-change distributions learned online** (not assumed Gaussian)
2. **Bilateral**: separate accumulators for mean increase and decrease

State: running quantile estimates (P2 algorithm or simpler binned histogram) for baseline distribution, plus CUSUM accumulators.

#### D. Online Kernel Two-Sample Test — Priority: MEDIUM

Maintain two sliding windows (baseline and recent) and compute an online approximation of the Maximum Mean Discrepancy (MMD). This is a streaming version of the batch MMD detector that already exists as a prototype.

Key: use random Fourier features to approximate the kernel, keeping computation O(1) per point.

**File**: `metrics_detector_mmd.go` exists as unregistered prototype — read it first.

#### E. Recursive Least Squares (RLS) Detector — Priority: LOW

Fit an online linear model to each series. When the prediction error exceeds a threshold consistently, emit a changepoint. The RLS filter maintains a covariance matrix that adapts to the data.

This catches trend changes and level shifts. State is a small matrix (2x2 for linear, 3x3 for quadratic).

### 3.3 Implementation Pattern

All new detectors should follow the BOCPD pattern (`metrics_detector_bocpd.go`):

1. **Per-series state struct** (private): cursor fields + algorithm-specific state
2. **Detector struct** (exported): config params + `series map[string]*state` + `cachedKeys/cachedGen`
3. **`Detect(storage StorageReader, dataTime int64)`**: iterate series, gate on new data, process points incrementally
4. **`Reset()`**: clear all state
5. **`Name()`**: unique identifier
6. **Register in `component_catalog.go`**

Use the BOCPD iteration loop (lines 140-221) as a template — it handles series discovery caching, new-data gating via `PointCountUpTo`/`WriteGeneration`, and cursor advancement.

### 3.4 Iteration Strategy

**Goal: implement as many new algorithms as time allows.** Breadth over depth — a working prototype with 1-2 threshold tweaks is better than perfecting one algorithm for 5 hours.

1. **Implement the simplest approach first** (EWM or adaptive CUSUM) — write the new file, register it, get it scored
2. **Quick-eval on food_delivery** (~8 min) to validate it produces non-zero output
3. **At most 2 threshold tweaks** per algorithm — then move on to the NEXT new algorithm
4. **Run full 3-scenario eval** only on algorithms that score > 0.2 on food_delivery
5. **Move to the next algorithm** — do not spend more than 1.5 hours on any single approach
6. **Compare against BOCPD tuned (0.261)** — that's the stateful baseline to beat
7. **Aim for 3+ new algorithms evaluated** by the end of the session

---

## 4. Existing Stateful Detectors (Reference)

### 4.1 BOCPD (tuned) — Avg F1: 0.261

**File**: `comp/observer/impl/metrics_detector_bocpd.go`
**Params**: Hazard=0.005, CPThreshold=0.95, CPMassThreshold=0.95, WarmupPoints=120
**State**: full Bayesian posterior over run lengths (arrays of probabilities, means, precisions per run length)
**Strength**: Principled probabilistic framework, naturally handles uncertainty
**Weakness**: Heavy state (O(MaxRunLength) arrays per series), slow to react after warmup

### 4.2 Mann-Whitney Streaming — Avg F1: 0.215

**File**: `comp/observer/impl/metrics_detector_mannwhitney.go`
**State**: fixed baseline window + sliding recent window (circular buffer)
**Strength**: Non-parametric, robust to distribution shape
**Weakness**: Fixed baseline never updates — can't adapt to regime changes after the first

### 4.3 CUSUM — Avg F1: 0.000

**File**: `comp/observer/impl/metrics_detector_cusum.go`
**State**: cumulative sum + baseline mean/stddev
**Problem**: Fires during warmup, all predictions filtered. Baseline estimation is too fragile.

---

## 5. Architecture Reference

### Detector Interface

```go
type Detector interface {
    Name() string
    Detect(storage StorageReader, dataTime int64) DetectionResult
}
```

### StorageReader API

```go
type StorageReader interface {
    ListSeries(filter SeriesFilter) []SeriesKey
    GetSeriesRange(key SeriesKey, start, end int64, agg Aggregate) *Series
    PointCount(key SeriesKey) int
    PointCountUpTo(key SeriesKey, endTime int64) int
    SeriesGeneration() uint64
    WriteGeneration(key SeriesKey) int64
}
```

### Key Files

| Category | Files |
|---|---|
| BOCPD (reference pattern) | `comp/observer/impl/metrics_detector_bocpd.go` |
| CUSUM (to improve) | `comp/observer/impl/metrics_detector_cusum.go` |
| Mann-Whitney (reference) | `comp/observer/impl/metrics_detector_mannwhitney.go` |
| Shared helpers | `comp/observer/impl/metrics_detector_util.go` |
| Component catalog | `comp/observer/impl/component_catalog.go` |
| Testbench | `comp/observer/impl/testbench.go` |
| Scorer | `comp/observer/impl/score.go`, `cmd/observer-scorer/` |
| Scenarios | `comp/observer/scenarios/` |
| Existing results | `RESULTS/streaming-v2/comparison-matrix.md` |

---

## 6. Success Criteria

| Metric | Current Best (stateful) | Target | Stretch |
|---|---|---|---|
| Best stateful detector avg F1 | 0.261 (bocpd tuned) | 0.400 | 0.500 |
| Stateful detectors with avg F1 > 0.2 | 2 (bocpd, mannwhitney) | 3 | 4 |
| Best stateful food_delivery F1 | 0.358 (mannwhitney) | 0.500 | 0.800 |
| Best stateful postmark F1 | 0.444 (bocpd) | 0.500 | 0.600 |

---

## 7. Key Lessons from Prior Iterations

1. **Selectivity > sophistication**: Fewer, higher-confidence anomalies consistently improve F1 more than improving the underlying test. When in doubt, make it more selective.
2. **BOCPD tuning worked**: Reducing hazard 10x and raising thresholds to 0.95 cut scored predictions in half, improving avg F1 by +0.112. Stateful detectors benefit from strict emission criteria.
3. **Scan detectors dominate on food_delivery/pagerduty**: Because they can re-scan the full segment and find the optimal split. Stateful detectors process points once — they need to be smarter about when to emit.
4. **Postmark is the differentiator**: Scan detectors score 0.578-0.619 on postmark; BOCPD scores 0.444. The gap is smallest here — stateful approaches may be competitive on subtle shifts.
5. **Precision is the bottleneck**: Every detector has recall > 0.8 but precision < 0.5 on pagerduty. The winning strategy is emitting fewer predictions, not catching more ground truth.
6. **Quick-eval first**: food_delivery_redis (~8 min) for fast feedback. Full 3-scenario only when promising.
7. **One change at a time**: So you know what helped.

---

## 8. Logging and Reporting

**You MUST log all your work to files.** Your conversation context will be lost between sessions. Treat these log files as your persistent memory.

### 8.1 Log Files

Maintain these files in the repo root:

**`iteration-log.md`** — Detailed lab notebook. Each entry:

```markdown
## <Detector> — <Variation Name> (YYYY-MM-DD HH:MM)

**What changed**: <exact parameters or code changes, with file paths and line numbers>

**Hypothesis**: <why you think this will help>

**Results**:
| Scenario | F1 | Precision | Recall | Scored | Warmup | Cascading |
|---|---|---|---|---|---|---|
| food_delivery_redis | X.XXX | X.XXX | X.XXX | N | N | N |
| 353_postmark | X.XXX | X.XXX | X.XXX | N | N | N |
| 213_pagerduty | X.XXX | X.XXX | X.XXX | N | N | N |
| **Avg F1** | **X.XXX** | | | | | |

**Delta vs baseline**: +/-X.XXX avg F1

**Analysis**: <what you learned — why it helped/hurt, what to try next>

**Keep or revert**: <keep / revert>
```

**`iteration-results.md`** — Summary scorecard. Update after every variation.

**`research-notes.md`** — Research, diagnosis, investigation notes. Log ideas, algorithm analysis, why you chose one approach over another.

### 8.2 When to Log

- **Before starting work on a detector**: Log your research in `research-notes.md`
- **After every eval run**: Log in `iteration-log.md`, update `iteration-results.md`
- **After a revert**: Note it so you don't re-try the same thing
- **When you discover something non-obvious**: Log immediately
