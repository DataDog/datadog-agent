# Changepoint Detection — Headless Agent Iteration Plan

**Date**: 2026-03-16 (revised 2026-03-17)
**Branch**: `ella/changepoint-detection-results-2`
**Objective**: **Re-baseline all current detectors with the fixed scorer, then implement and evaluate NEW changepoint detection algorithms.** The scorer had bugs that have been fixed — all prior scores are stale. Re-run the full matrix first, then iterate on new algorithms guided by the fresh results.

**Interface constraint**: New detectors MUST implement `observerdef.Detector` (the flexible storage-pull interface in `comp/observer/def/component.go:476`). Do **not** use `SeriesDetector` — it is a simpler single-series interface and is less capable. `Detector` receives a `StorageReader` and can query any combination of series, enabling **multivariate detection** across multiple metrics simultaneously. This is underexplored and high-potential.

---

## 0. Headless Execution Rules

This plan is designed for autonomous execution. Follow these rules without exception.

### Autonomy

**Execute all steps autonomously without waiting for user approval.** Do not pause for confirmation between iterations. Make your own judgment calls about what to try next. If you hit ambiguity, make a decision, log your reasoning in `research-notes.md`, and keep going.

### Exit Condition

Stop when **any** of these are true:
- **Hard time limit (8 hours)**: At the very start of your session, record the current time with `date +%s` and save it to `iteration-start-time.txt`. Before starting each new variation, check elapsed time with `echo $(( $(date +%s) - $(cat iteration-start-time.txt) ))`. If more than 28800 seconds (8 hours) have elapsed, **stop immediately** — do not start another variation. Use the remaining time only to write your final summary.
- **Diminishing returns**: Your last 3 consecutive variations (across any detectors) each improved the best avg F1 by less than 0.005
- **All approaches explored**: You've attempted at least 3 new detector designs and no approach has obvious remaining headroom
- **Target reached**: A new detector achieves avg F1 > 0.500

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
- **Parallelization**: You can run up to **2 scenarios in parallel** safely. food_delivery (~8 min) and 353_postmark (~15-20 min) can be launched concurrently. Run 213_pagerduty (~20-27 min) as a third job only after checking `vm_stat` — if free memory is below ~8 GB, wait for one of the parallel jobs to finish first.
- Check with `vm_stat` or `top` before launching if unsure

---

## 1. Phase 0: Re-Baseline (Do This First)

**The scorer had bugs that have been fixed. All prior scores in `RESULTS/streaming-v2/comparison-matrix.md` are stale. Re-run the full eval matrix before starting any algorithm work.**

### 1.1 Registered detectors to re-baseline

From `comp/observer/impl/component_catalog.go`, the currently registered detectors are:

| Name | Catalog key |
|---|---|
| BOCPD | `bocpd` |
| ScanMW | `scanmw` |
| ScanWelch | `scanwelch` |

Run each detector × 3 scenarios = **9 runs total**. Parallelize where memory allows (see resource constraints above).

### 1.2 Re-baseline eval commands

**Build once at the start:**
```bash
go build -o bin/observer-testbench ./cmd/observer-testbench && go build -o bin/observer-scorer ./cmd/observer-scorer
```

**Per-detector template** (replace `<DETECTOR>` and `<SCENARIO>`):
```bash
./bin/observer-testbench \
  -headless <SCENARIO> \
  -output RESULTS/streaming-v2/rebaseline/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "<DETECTOR>,time_cluster" \
  -disable "bocpd,scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" \
  -verbose
```

Remove `<DETECTOR>` from the `-disable` list for each run.

**Score each result:**
```bash
./bin/observer-scorer \
  -input RESULTS/streaming-v2/rebaseline/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -sigma 30.0
```

### 1.3 Parallelization strategy for re-baseline

Launch food_delivery + postmark in parallel per detector, then pagerduty:

```bash
# Example: re-baseline bocpd with parallelism
./bin/observer-testbench -headless food_delivery_redis \
  -output RESULTS/streaming-v2/rebaseline/eval-bocpd-food_delivery_redis.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "bocpd,time_cluster" \
  -disable "scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" -verbose &

./bin/observer-testbench -headless 353_postmark \
  -output RESULTS/streaming-v2/rebaseline/eval-bocpd-353_postmark.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "bocpd,time_cluster" \
  -disable "scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" -verbose &

wait  # wait for both before launching pagerduty

./bin/observer-testbench -headless 213_pagerduty \
  -output RESULTS/streaming-v2/rebaseline/eval-bocpd-213_pagerduty.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "bocpd,time_cluster" \
  -disable "scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" -verbose
```

Repeat for scanmw and scanwelch. After all 9 runs are scored, write updated scores to `RESULTS/streaming-v2/rebaseline/comparison-matrix.md` and use those as the new baseline.

---

## 2. How to Run Evals (New Detectors)

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
  -disable "bocpd,scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" \
  -verbose
```

Remove the detector you're testing from the `-disable` list.

**Scenarios**: `food_delivery_redis` (~8 min), `353_postmark` (~15-20 min), `213_pagerduty` (~20-27 min)

**Currently registered detectors** (in `component_catalog.go`): `bocpd`, `scanmw`, `scanwelch`

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
  -disable "bocpd,scanmw,scanwelch,cross_signal,lead_lag,surprise,passthrough" \
  -verbose && \
./bin/observer-scorer \
  -input "RESULTS/streaming-v2/eval-${DETECTOR}-food_delivery_redis.json" \
  -scenarios-dir ./comp/observer/scenarios \
  -sigma 30.0
```

**IMPORTANT**: Remove `$DETECTOR` from the `-disable` list.

---

## 3. Research Focus: NEW Detector Algorithms

**CRITICAL RULE**: Do NOT tune existing detectors. Every iteration must involve writing a NEW detector file or a substantially NEW algorithm within an existing file. Parameter sweeps on BOCPD, ScanMW, ScanWelch are explicitly out of scope. If you find yourself adjusting thresholds on an existing detector, STOP and implement a new algorithm instead.

### 3.1 The Detector Interface

New detectors MUST implement `observerdef.Detector` (`comp/observer/def/component.go:476`):

```go
type Detector interface {
    Name() string
    // Detect is called periodically by the scheduler.
    // Queries StorageReader for whatever data it needs.
    // dataTime is the current data timestamp (only read data <= dataTime).
    Detect(storage StorageReader, dataTime int64) DetectionResult
}
```

`StorageReader` provides:
- `ListSeries(filter SeriesFilter) []SeriesKey` — discover all series matching a filter
- `GetSeriesRange(key, start, end, agg) *Series` — fetch time series data
- `PointCountUpTo(key, endTime) int` — count points (for cursor advancement)
- `WriteGeneration(key) int64` — detect new data without re-reading
- `SeriesGeneration() uint64` — detect new series without re-listing

**Do NOT use `SeriesDetector`** — it's a simpler single-series interface with less capability. `Detector` is strictly more powerful.

### 3.2 What makes a good detector here

A detector that works well on these scenarios will:
1. **Emit few, high-confidence predictions** — precision is the bottleneck across all scenarios (all detectors have recall > 0.8 but precision < 0.5 on pagerduty/postmark)
2. **Avoid warmup-period fires** — predictions during the warm-up window are filtered and hurt score
3. **Self-reset after detection** — re-learn the new regime; detectors that fire once then go quiet do well
4. **Handle the scale of pagerduty** (95k series) without exploding in memory or time

### 3.3 Implementation Pattern

All new detectors should follow the BOCPD pattern (`metrics_detector_bocpd.go`):

1. **Per-series state struct** (private): cursor fields + algorithm-specific state
2. **Detector struct** (exported): config params + `series map[string]*state` + `cachedKeys/cachedGen`
3. **`Detect(storage StorageReader, dataTime int64)`**: iterate series, gate on new data, process points incrementally
4. **`Reset()`**: clear all state
5. **`Name()`**: unique identifier
6. **Register in `component_catalog.go`**

Use the BOCPD iteration loop (lines 140-221) as a template — it handles series discovery caching, new-data gating via `PointCountUpTo`/`WriteGeneration`, and cursor advancement.

For multivariate detectors, the same pattern applies but `Detect()` calls `ListSeries()` across multiple namespaces/filters and correlates findings across series before emitting.

### 3.5 Iteration Strategy

**Goal: implement as many new algorithms as time allows.** Breadth over depth — a working prototype with 1-2 threshold tweaks is better than perfecting one algorithm for 5 hours.

1. **Complete Phase 0 re-baseline first** — don't start algorithm work without fresh numbers
2. **Implement the simplest approach first** (EWM or adaptive CUSUM) — write the new file, register it, get it scored
3. **Quick-eval on food_delivery** (~8 min) to validate it produces non-zero output
4. **At most 2 threshold tweaks** per algorithm — then move on to the NEXT new algorithm
5. **Run full 3-scenario eval** only on algorithms that score > 0.2 on food_delivery
6. **Move to the next algorithm** — do not spend more than 1.5 hours on any single approach
7. **Compare against re-baselined scores** — that's the new baseline to beat
8. **Aim for 3+ new algorithms evaluated** by the end of the session
9. **Try at least one multivariate approach** — it is underexplored and has high potential

---

## 4. Existing Detectors (Reference)

### 4.1 BOCPD (tuned) — Prior Avg F1: 0.261 (stale — re-baseline first)

**File**: `comp/observer/impl/metrics_detector_bocpd.go`
**Params**: Hazard=0.005, CPThreshold=0.95, CPMassThreshold=0.95, WarmupPoints=120
**State**: full Bayesian posterior over run lengths (arrays of probabilities, means, precisions per run length)
**Strength**: Principled probabilistic framework, naturally handles uncertainty
**Weakness**: Heavy state (O(MaxRunLength) arrays per series), slow to react after warmup

### 4.2 ScanMW — Prior Avg F1: 0.613 (stale — re-baseline first)

**File**: `comp/observer/impl/metrics_detector_scanmw.go`
**State**: segment-based sliding window, re-scans on each call
**Strength**: Non-parametric rank test, robust to distribution shape
**Weakness**: Re-scans full segment each call (O(n)); fixed baseline never adapts

### 4.3 ScanWelch — Prior Avg F1: 0.679 (stale — re-baseline first)

**File**: `comp/observer/impl/metrics_detector_scanwelch.go`
**State**: segment-based, parametric Welch t-test variant
**Strength**: Best overall prior score; high selectivity (1-3 scored predictions per scenario)
**Weakness**: Re-scans full segment each call; parametric (assumes approx. normality)

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
| ScanMW (reference) | `comp/observer/impl/metrics_detector_scanmw.go` |
| ScanWelch (reference) | `comp/observer/impl/metrics_detector_scanwelch.go` |
| Shared helpers | `comp/observer/impl/metrics_detector_util.go` |
| Component catalog | `comp/observer/impl/component_catalog.go` |
| Testbench | `comp/observer/impl/testbench.go` |
| Scorer | `comp/observer/impl/score.go`, `cmd/observer-scorer/` |
| Scenarios | `comp/observer/scenarios/` |
| Interface defs | `comp/observer/def/component.go` |
| Prior results (stale) | `RESULTS/streaming-v2/comparison-matrix.md` |
| Re-baseline results | `RESULTS/streaming-v2/rebaseline/comparison-matrix.md` (create this) |

---

## 6. Success Criteria

| Metric | Prior Best (stale) | Target | Stretch |
|---|---|---|---|
| Best detector avg F1 | 0.679 (scanwelch) | Beat re-baselined scanwelch | — |
| Best new-algorithm avg F1 | — | 0.400 | 0.500 |
| New algorithms with avg F1 > 0.2 | 0 | 3 | 4 |
| Multivariate approach evaluated | No | Yes | F1 > 0.300 |

---

## 6.5 Important Caveat: Prior Eval Results Are Unreliable

**All scores in `RESULTS/streaming-v2/comparison-matrix.md` and all earlier iteration logs were produced by a buggy scorer.** The eval framework had large bugs that have since been fixed. Treat every historical F1/precision/recall number as potentially wrong — do not use them as evidence when making algorithm decisions.

Concretely:
- Do NOT cite prior F1 scores as a reason to pursue or abandon an approach
- Do NOT assume that a detector which scored high previously is actually better
- Do NOT assume the relative ranking of detectors (e.g. "scanwelch > bocpd") reflects ground truth
- DO re-baseline everything (Phase 0) before drawing any conclusions
- After re-baseline, use ONLY the fresh numbers from `RESULTS/streaming-v2/rebaseline/` as evidence

The re-baseline is the most important output of this session. Everything else depends on it.

---

## 7. Key Lessons from Prior Iterations

0. **Prior iteration results are invalid**: All scores from previous sessions were produced by a buggy eval framework. They are listed here as historical context only — do NOT use them as evidence when making decisions. Re-baseline first (Section 1) and treat those as the only trustworthy numbers.
1. **Selectivity > sophistication**: Fewer, higher-confidence anomalies consistently improve F1 more than improving the underlying test. When in doubt, make it more selective.
2. **BOCPD tuning worked**: Reducing hazard 10x and raising thresholds to 0.95 cut scored predictions in half, improving avg F1 by +0.112. Stateful detectors benefit from strict emission criteria.
3. **Scan detectors dominate on food_delivery/pagerduty**: Because they can re-scan the full segment and find the optimal split. Stateful detectors process points once — they need to be smarter about when to emit.
4. **Postmark is the differentiator**: Scan detectors score 0.578-0.619 on postmark; BOCPD scores 0.444. The gap is smallest here — novel approaches may be competitive on subtle shifts.
5. **Precision is the bottleneck**: Every detector has recall > 0.8 but precision < 0.5 on pagerduty. The winning strategy is emitting fewer predictions, not catching more ground truth.
6. **Quick-eval first**: food_delivery_redis (~8 min) for fast feedback. Full 3-scenario only when promising.
7. **One change at a time**: So you know what helped.
8. **Multivariate is unexplored**: The `Detector` interface supports querying multiple series — co-changepoint detection (K series fire simultaneously) is a natural precision filter that has never been tried.

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
