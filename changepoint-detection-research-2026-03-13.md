# Changepoint Detection — Headless Agent Iteration Plan

**Date**: 2026-03-13
**Branch**: `ella/changepoint-detection-algorithms-v2`
**Objective**: Improve L2 timestamp F1 scores across 3 production-derived scenarios by iterating on detector parameters and implementations. All detectors use the streaming `Detector` interface.

---

## 0. Headless Execution Rules

This plan is designed for autonomous execution. Follow these rules without exception.

### Autonomy

**Execute all steps autonomously without waiting for user approval.** Do not pause for confirmation between iterations. Make your own judgment calls about what to try next. If you hit ambiguity, make a decision, log your reasoning in `research-notes.md`, and keep going.

### Exit Condition

Stop when **any** of these are true:
- **Hard time limit (4 hours)**: At the very start of your session, record the current time with `date +%s` and save it to `iteration-start-time.txt`. Before starting each new variation, check elapsed time with `echo $(( $(date +%s) - $(cat iteration-start-time.txt) ))`. If more than 14400 seconds (4 hours) have elapsed, **stop immediately** — do not start another variation. Use the remaining time only to write your final summary.
- **Diminishing returns**: Your last 3 consecutive variations (across any detectors) each improved the best avg F1 by less than 0.005
- **All detectors explored**: You've attempted at least 3 variations for each of the 6 detectors and no detector has obvious remaining headroom
- **Target reached**: The best single-detector avg F1 exceeds 0.500

When you stop (for any reason), write a final summary entry in `iteration-log.md` with the heading `## FINAL SUMMARY` that includes the final results table, what worked best, and what you'd try next if continuing.

### Error Handling

- If a testbench run **hangs for more than 30 minutes** (pagerduty can legitimately take 27 min, so use 30 as the cutoff), kill it. Log the timeout in `iteration-log.md` and `research-notes.md`, then move on.
- If a build fails, **fix the build error** (you are allowed to modify code). Log the error and fix in `research-notes.md`.
- If a scorer run fails or produces garbled output, log the raw output and skip that scenario. Do not block on it.
- **Never let a single failure stop the whole iteration loop.** Log it and continue with the next detector or variation.

### Session Handoff

Each new Claude session MUST start by archiving the previous session's log files and creating fresh ones. This prevents confusion from stale state and gives each session a clean slate while preserving history.

**On session start, run this before doing anything else:**

```bash
cd /home/bits/go/src/github.com/DataDog/datadog-agent
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
for f in iteration-log.md iteration-results.md research-notes.md iteration-progress.log; do
  if [ -f "$f" ]; then
    mv "$f" "${f}.${TIMESTAMP}.old"
  fi
done
```

Then **create fresh log files** following the templates in Section 9.1. The fresh `iteration-results.md` should carry forward the **current best scores** from the archived file (read the old file first), so you don't lose the running best. The fresh `iteration-log.md` and `research-notes.md` start empty (with headers). The fresh `iteration-progress.log` starts empty.

**Reading prior context**: Before starting work, read the most recent `.old` files to understand what was tried, what worked, and what the current best scores are. This is your "memory" across sessions.

### Multi-Agent Coordination

This research plan is being given to a **second workspace** (`workspace-metrics-ad-bench-3-12`). Another agent on a different workspace is working from the same plan concurrently.

**Before starting any work**, read `iteration-log.md` (including any `.old` archived versions) to see what the other agent has already tried or is currently working on. **Do NOT duplicate their work.** If they are already iterating on a detector or exploring an approach, pick a **different innovation path** so we can maximize research coverage across both agents.

For example:
- If the other agent is tuning BOCPD parameters, you should work on a different detector (e.g., fixing MW zero-output, or registering an unregistered prototype)
- If the other agent is focused on parameter tuning, you should explore architectural changes (e.g., rewriting a detector's streaming logic, implementing a new algorithm)
- If the other agent is working on high-priority detectors, pick the medium/low-priority ones they haven't touched

**You MUST keep `iteration-results.md` updated after every variation.** This is critical — it is the shared scoreboard that both agents (and the human reviewer) use to track progress across workspaces. If you don't update it, the other agent may unknowingly duplicate your work or miss your findings.

### Feedback Loop

Before starting each new iteration cycle, **check the file `human-feedback.md`** in the repo root. This is a document where the human reviewer will write feedback for you to incorporate — things like "stop working on X", "try Y approach instead", "focus on Z detector", etc.

- If `human-feedback.md` does not exist, create it as a blank file with a header:
  ```markdown
  # Human Feedback
  _Check this file before each iteration cycle. The human reviewer will add feedback here._
  ```
- If it contains new feedback you haven't acted on yet, incorporate it into your next iteration decision
- After reading and acting on feedback, append a note at the bottom: `> [YYYY-MM-DD HH:MM] Acknowledged: <brief summary of what you read and how you're adjusting>`

### Progress Tracking

After each eval run (testbench + scorer for one detector on one scenario), append a progress line to `iteration-progress.log`:

```
[PROGRESS] YYYY-MM-DD HH:MM:SS | <detector>/<scenario> | F1=X.XXX | variation: <name> | <N>/<total> complete
```

This file is append-only. It lets an external observer `tail -f iteration-progress.log` to monitor progress in real time.

At the start of each new detector iteration round, also log:
```
[START] YYYY-MM-DD HH:MM:SS | Starting <detector> iteration round N
```

### Heartbeat During Long Evals

Testbench runs can take 8–27 minutes per scenario. During any eval run, **log a heartbeat every 2 minutes** so an external observer knows the process is alive and not hung:

```
[WAITING] YYYY-MM-DD HH:MM:SS | <detector>/<scenario> | eval running (~Xm elapsed) | variation: <name>
```

Implementation: When launching a testbench run, start a background heartbeat loop that appends to `iteration-progress.log` every 120 seconds. Kill it when the eval finishes. Example:

```bash
# Start heartbeat in background
(while true; do
  echo "[WAITING] $(date '+%Y-%m-%d %H:%M:%S') | bocpd/food_delivery_redis | eval running | variation: V2" >> iteration-progress.log
  sleep 120
done) &
HEARTBEAT_PID=$!

# Run the actual eval
./bin/observer-testbench ... && ./bin/observer-scorer ...

# Kill heartbeat
kill $HEARTBEAT_PID 2>/dev/null
```

This is especially important when running evals in the background — without heartbeats, a `tail -f iteration-progress.log` watcher has no way to distinguish "still running" from "silently crashed".

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

You ARE expected to modify detector implementations, tune parameters, write new detectors, and register prototypes in the component catalog. That is the point of this plan. The only things you should NOT do:
- Do not modify the scorer (`observer-scorer`) — it's the ground truth
- Do not modify scenario data (`comp/observer/scenarios/`)
- Do not modify the testbench harness itself (`testbench.go`) unless you have a clear engineering reason, and log your reasoning if you do
- Do not push to remote or create PRs — work locally on the branch

---

## 1. Current Streaming Baseline (2026-03-13)

This is the ground truth. All scores are from isolated per-detector streaming eval with `time_cluster` correlator only.

| Detector | food_delivery_redis | 353_postmark | 213_pagerduty | Avg F1 |
|---|---|---|---|---|
| **bocpd** | F1=0.146 P=0.079 R=0.946 | F1=0.195 P=0.108 R=0.974 | F1=0.119 P=0.064 R=0.896 | **0.153** |
| **corrshift** | F1=0.211 P=0.123 R=0.739 | F1=0.049 P=0.026 R=0.765 | F1=0.000 P=0.000 R=0.000 | **0.087** |
| **cusum** | F1=0.000 | F1=0.000 | F1=0.000 | **0.000** |
| **rrcf** | F1=0.000 | F1=0.000 | F1=0.000 | **0.000** |
| **mannwhitney** | F1=0.000 | F1=0.000 | F1=0.000 | **0.000** |
| **topk** | F1=0.000 | F1=0.000 | F1=0.000 | **0.000** |

### Historical Batch Scores (for reference — these were pre-streaming)

| Detector | pagerduty | postmark | food_delivery | Avg F1 |
|---|---|---|---|---|
| TopK V8 | 0.597 | 0.253 | 0.945 | **0.598** |
| Mann-Whitney V1 | 0.630 | 0.230 | 0.816 | **0.559** |
| RRCF V5 | 0.739 | 0.194 | 0.640 | **0.524** |
| MMD V4 | 0.421 | 0.181 | 0.945 | **0.516** |
| CorrShift V2 | 0.121 | 0.230 | 0.739 | **0.363** |
| BOCPD V5 | 0.198 | 0.202 | 0.189 | **0.196** |
| CUSUM V6 | 0.240 | 0.112 | 0.210 | **0.187** |

---

## 2. How to Run Evals

### 2.1 Build

```bash
cd /home/bits/go/src/github.com/DataDog/datadog-agent
dda inv observer.build
```

This builds both `./bin/observer-testbench` and `./bin/observer-scorer`.

### 2.2 Run Testbench (per detector, per scenario)

**CRITICAL**: Always run detectors in **isolation** — enable only ONE detector plus `time_cluster`. Cross-detector interference in TimeCluster causes a 0.2-0.3 F1 penalty when multiple detectors are enabled.

```bash
# Template
./bin/observer-testbench \
  -headless <SCENARIO> \
  -output /tmp/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "<DETECTOR>,time_cluster" \
  -disable "cusum,bocpd,rrcf,mannwhitney,corrshift,topk,cross_signal,lead_lag,surprise,passthrough" \
  -verbose
```

Replace `<DETECTOR>` and `<SCENARIO>` with the appropriate values. The `-disable` list should include ALL other detectors and all non-time_cluster correlators. Remove the detector you're testing from the disable list.

**Scenarios**: `food_delivery_redis`, `353_postmark`, `213_pagerduty`

**Registered detectors** (in `component_catalog.go`): `cusum`, `bocpd`, `rrcf`, `mannwhitney`, `corrshift`, `topk`

**Registered correlators**: `cross_signal`, `time_cluster`, `lead_lag`, `surprise`, `passthrough`

**Example** (BOCPD on pagerduty):
```bash
./bin/observer-testbench \
  -headless 213_pagerduty \
  -output /tmp/eval-bocpd-213_pagerduty.json \
  -scenarios-dir ./comp/observer/scenarios \
  -enable "bocpd,time_cluster" \
  -disable "cusum,rrcf,mannwhitney,corrshift,topk,cross_signal,lead_lag,surprise,passthrough" \
  -verbose
```

### 2.3 Score Results

```bash
./bin/observer-scorer \
  -input /tmp/eval-<DETECTOR>-<SCENARIO>.json \
  -scenarios-dir ./comp/observer/scenarios \
  -sigma 30.0
```

The scorer outputs: F1, Precision, Recall, num_predictions, num_scored, num_warmup_filtered, num_cascading_filtered.

**What the numbers mean**:
- **F1**: Gaussian-weighted harmonic mean of precision and recall (sigma=30s). This is the primary metric.
- **Precision**: Fraction of predictions that overlap ground truth (Gaussian-weighted, sigma=30s)
- **Recall**: Fraction of ground truth timestamps covered by predictions
- **Scored**: Predictions that survived warmup and cascading filters
- **Warmup filtered**: Predictions in the early baseline period (before changepoint could be real)
- **Cascading filtered**: Duplicate predictions near an already-scored prediction

### 2.4 Full Eval Matrix Script

To eval one detector across all 3 scenarios:

```bash
DETECTOR="bocpd"
dda inv observer.build && \
for SCENARIO in food_delivery_redis 353_postmark 213_pagerduty; do
  echo "=== ${DETECTOR} / ${SCENARIO} ==="
  ./bin/observer-testbench \
    -headless "$SCENARIO" \
    -output "/tmp/eval-${DETECTOR}-${SCENARIO}.json" \
    -scenarios-dir ./comp/observer/scenarios \
    -enable "${DETECTOR},time_cluster" \
    -disable "cusum,bocpd,rrcf,mannwhitney,corrshift,topk,cross_signal,lead_lag,surprise,passthrough" \
    -verbose && \
  ./bin/observer-scorer \
    -input "/tmp/eval-${DETECTOR}-${SCENARIO}.json" \
    -scenarios-dir ./comp/observer/scenarios \
    -sigma 30.0
  echo ""
done
```

**IMPORTANT**: Remove the detector being tested from the `-disable` list. The script above disables ALL detectors — you must remove `$DETECTOR` from that list. For example, if `DETECTOR="bocpd"`, the disable list should be `"cusum,rrcf,mannwhitney,corrshift,topk,cross_signal,lead_lag,surprise,passthrough"`.

### 2.5 Timing and Scenario Characteristics

| Scenario | Approx Duration | Series Count | Notes |
|---|---|---|---|
| food_delivery_redis | ~8-10 min | ~28k | Fastest. Use as quick-eval. |
| 353_postmark | ~15-20 min | ~25k | Fewer series than pagerduty but slower per-series (longer time range, denser data). |
| 213_pagerduty | ~20-27 min | ~95k | Slowest by far — 3-4x the series of food_delivery. Expect long waits. |

Total for one detector across all 3: ~45-60 min. Plan accordingly.

**Scenario-specific expectations**:

- **food_delivery_redis**: Short series (~130 data points each). Changepoint is a clear redis latency spike. Detectors that use a baseline fraction (first 25%) have very few baseline points to work with. Streaming detectors with stability checks (e.g., MW streaming's "wait N ticks for p-value to stabilize") may time out because the series is too short — the stability window consumes most of the data. This scenario rewards detectors that can detect with minimal data.

- **353_postmark**: The hardest scenario across all prior iterations. Best-ever score is MW Streaming 0.526 (batch: Spectral 0.445). Most detectors score <0.25. The changepoints involve subtle correlation structure shifts rather than obvious level changes — detectors that only look at magnitude (TopK, CUSUM) struggle. CorrShift's window-size tradeoff is most visible here: too large and it misses fast shifts, too small and it false-positives. Don't over-optimize for postmark at the expense of the other two — diminishing returns are steep.

- **213_pagerduty**: Largest scenario by far (95k series). The sheer volume means noisy detectors (BOCPD, CUSUM) produce thousands of raw anomalies, which overwhelm TimeCluster and degrade precision. Selective detectors (TopK, MW) shine here because they naturally limit output. Expect this scenario to take 2-3x longer than food_delivery per run. If you're iterating quickly, skip pagerduty until you have a promising variation from the other two, then confirm on pagerduty as the final check.

### 2.6 Resource Constraints

- Each testbench run consumes ~3-5 GB RAM (parquet loading)
- Max ~8 concurrent eval runs on the workspace (61 GB RAM)
- If running parallel evals, cap at `available_GB / 5` concurrent processes
- Check with `free -h` before launching multiple parallel evals

---

## 3. Iteration Methodology

### 3.1 Core Principle: Selectivity > Sophistication

Every successful iteration in prior work reduced raw anomaly counts. **Emitting fewer, higher-confidence anomalies consistently improves F1 more than improving the underlying statistical test.** When in doubt, make the detector more selective.

### 3.2 Iteration Protocol

For each detector variation:

1. **Make ONE change at a time** (1-2 parameters or one small code change)
2. **Build**: `dda inv observer.build`
3. **Quick eval on ONE scenario first** (food_delivery_redis is fastest at ~8 min)
4. **If promising** (F1 improved or held steady), run the other 2 scenarios
5. **If clearly worse** (F1 dropped significantly on the quick scenario), revert and try something else
6. **Record results** in a table: variation name, what changed, F1/P/R per scenario, avg F1, delta from baseline
7. **Keep the best variation's code** — revert to it before trying the next variation

### 3.3 What to Track Per Variation

```
| Variation | Change | food_delivery | postmark | pagerduty | Avg F1 | Delta |
|-----------|--------|:---:|:---:|:---:|:---:|:---:|
| Baseline | — | X.XXX | X.XXX | X.XXX | X.XXX | — |
| V1 | <description> | X.XXX | X.XXX | X.XXX | X.XXX | +/-X.XXX |
```

### 3.4 When to Stop Iterating on a Detector

- Diminishing returns: last 3 variations each improved avg F1 by <0.01
- Overfitting signal: improving one scenario at the expense of others (e.g., pagerduty up 0.2, postmark down 0.15)
- Reached batch-era target: streaming score within 0.05 of the best batch score from Section 1

### 3.5 Simplest Changes Win

Prior iteration found that the simplest 1-2 parameter changes consistently outperformed complex multi-filter approaches:
- Mann-Whitney: 2 params changed → +16.7% avg F1 (best MW iteration)
- CUSUM: 2 params changed → +75% avg F1 (best CUSUM iteration)
- Complex post-detection filter chains were counterproductive for CUSUM and marginal for BOCPD

---

## 4. Detector-Specific Iteration Instructions

### 4.1 BOCPD — Priority: HIGH

**File**: `comp/observer/impl/metrics_detector_bocpd.go`
**Current streaming avg F1**: 0.153
**Batch target**: 0.196 (V5)
**Problem**: High recall (0.90-0.97) but terrible precision (0.06-0.11). Fires on too many false positives.

**Proposed changes to try (in order)**:

| # | Parameter | Current | Try | Rationale |
|---|-----------|:---:|:---:|---|
| 1 | Hazard | 0.05 | **0.01** | Fewer changepoint hypotheses per tick |
| 2 | CPThreshold | 0.6 | **0.8** | Require higher posterior probability |
| 3 | CPMassThreshold | 0.7 | **0.8** | Require more mass on short run lengths |

**Iteration direction**: BOCPD's problem is precision, not recall. All changes should make it MORE selective (higher thresholds, lower hazard). Do NOT try to increase recall — it's already at 0.90+.

**What worked in batch iteration**:
- V5 (best balanced): strict post-detection filters + baseline suppression → 0.196
- V6: very aggressive filters → 0.544 on pagerduty but killed postmark (0.073)
- OR logic for filters (pass if EITHER deviation sigma OR relative change exceeds threshold) outperformed AND
- Baseline suppression (don't emit during first 25% of data) was critical

**What to avoid**:
- Don't add complex multi-filter chains — keep BOCPD's core Bayesian mechanism clean
- Don't try to make it more sensitive — it already fires too much

### 4.2 CUSUM — Priority: MEDIUM

**File**: `comp/observer/impl/metrics_detector_cusum.go`
**Current streaming avg F1**: 0.000
**Batch target**: 0.187 (V6)
**Problem**: All predictions land in the warmup window and get filtered. Fires too early — at the first threshold crossing rather than at the actual changepoint.

**Proposed changes to try**:

| # | Change | Current | Try | Rationale |
|---|--------|---------|-----|-----------|
| 1 | Baseline estimation | mean + stddev | **median + MAD** | Robust to outlier contamination |
| 2 | ThresholdFactor | 4.0 | **5.0** | Higher threshold, later trigger |

**Iteration direction**: The fundamental issue is that CUSUM is a sequential accumulator — it triggers at the first threshold crossing, which happens during early noise. The solution is either:
1. More robust baselines (median/MAD) so early noise doesn't spike the cumulative sum
2. Higher thresholds to delay triggering until a real shift accumulates
3. Both

**What worked in batch iteration**:
- V6 (best): median/MAD + ThresholdFactor=5.0 → 0.187 (+75%)
- Post-detection filters were COUNTERPRODUCTIVE — CUSUM is sequential; resetting/filtering disrupts the core mechanism
- Baseline suppression was HARMFUL — scorer window is narrow (ground truth ±60s), CUSUM needs to fire near the changepoint, not be silenced
- Simplest change gave best result

**What to avoid**:
- Do NOT add post-detection filters (deviation sigma, relative change). These work for MW but hurt CUSUM.
- Do NOT suppress emissions during baseline — it pushes TPs out of the scoring window
- ThresholdFactor beyond 6.0 showed diminishing returns in batch

### 4.3 Mann-Whitney — Priority: HIGH

**File**: `comp/observer/impl/metrics_detector_mannwhitney.go`
**Current streaming avg F1**: 0.000
**Batch target**: 0.559 (V1)
**Problem**: Currently producing zero output in streaming mode. Need to diagnose why — the batch SeriesDetector version works well, so the issue is likely in how the seriesDetectorAdapter wraps it for streaming.

**Proposed changes to try**:

| # | Parameter | Current | Try | Rationale |
|---|-----------|:---:|:---:|---|
| 1 | MinEffectSize | 0.95 | **0.98** | Near-perfect rank separation required |
| 2 | MinDeviationSigma | 3.0 | **4.0** | Larger robust deviation required |

**IMPORTANT**: Before tuning parameters, **diagnose why MW produces zero output in streaming mode**. Possible causes:
- All predictions are warmup-filtered (like CUSUM)
- The seriesDetectorAdapter is re-running the full batch every tick, causing O(n^2) behavior and timeout
- Filter thresholds that work in batch are too strict when applied to partial data

**Debugging approach**: Run with `-verbose` and examine the JSON output. Check:
- How many raw anomalies are produced?
- How many are warmup-filtered vs cascading-filtered vs scored?
- What are the actual p-values and effect sizes being computed?

**What worked in batch iteration**:
- V1: MinEffectSize 0.95→0.98, MinDeviationSigma 3.0→4.0 → 0.559 (+16.7%)
- The simplest 2-parameter change was the best of all MW iterations
- Larger windows (90, 45) were worse. Finer stepping (1) was worse.
- Disabling MinRelativeChange filter was slightly worse

**Streaming prototype context**: A dedicated streaming MW (`metrics_detector_mannwhitney_streaming.go`) exists as an unregistered prototype. It uses progressive scan + stability check (emit when best p-value hasn't improved for N ticks). It scored 0.482 avg in batch-era eval (0.921 pagerduty, 0.526 postmark, 0.000 food_delivery). Consider registering this as a separate detector if the wrapped batch MW can't be fixed.

### 4.4 TopK — Priority: HIGH

**File**: `comp/observer/impl/metrics_detector_topk.go`
**Current streaming avg F1**: 0.000
**Batch target**: 0.598 (V8)
**Problem**: Like MW, producing zero output. TopK is fundamentally a batch algorithm (needs full-picture ranking). The seriesDetectorAdapter wraps it but streaming behavior may not work.

**Proposed changes to try**:

| # | Parameter | Current | Try | Rationale |
|---|-----------|:---:|:---:|---|
| 1 | TopK | 20 | **10** | Fewer emissions, higher selectivity |
| 2 | TopFraction | 0.02 | **0.01** | Stricter fraction cap |
| 3 | MinRelativeChange | 3.0 | **5.0** | Require 5x baseline dispersion |
| 4 | TopPerService | 1 | **0** (disabled) | Global ranking only |
| 5 | Scoring | median/MAD only | **max(mean/stddev, median/MAD)** | Dual scoring |
| 6 | Timestamp | Window midpoint | **CUSUM onset detection** | Precise changepoint timestamps |

**IMPORTANT**: Same as MW — **diagnose zero output first**. TopK should at minimum produce SOMETHING when wrapped by seriesDetectorAdapter.

**What worked in batch iteration**:
- V8 (best overall batch detector): all 6 changes above → 0.598 (+84%)
- CUSUM onset detection was the biggest single improvement (timestamps went from window-midpoint to exact inflection point)
- TopPerService=0 (global ranking) outperformed service diversity bonus
- Clean eval (only topk+time_cluster) scored 0.581 vs 0.325 with all detectors — cross-detector interference is massive

**Streaming conversion note**: A streaming prototype (`metrics_detector_topk_streaming.go`) exists but performed poorly (all variants worse than batch). TopK may need to remain as periodic deferred batch. If the wrapped batch version produces zero, consider:
1. Increasing the seriesDetectorAdapter's data accumulation before first run
2. Running TopK on a timer (every 60s) rather than every tick

### 4.5 RRCF — Priority: MEDIUM

**File**: `comp/observer/impl/metrics_detector_rrcf.go`
**Current streaming avg F1**: 0.000
**Batch target**: 0.524 (V5)
**Problem**: Zero output. RRCF already implements the streaming `Detector` interface directly (not wrapped by seriesDetectorAdapter), so this is a different class of problem.

**Proposed changes to try**:

| # | Change | Current | Try | Rationale |
|---|--------|---------|-----|-----------|
| 1 | Metric selection | Hardcoded `cgroup.v2` | **Auto-discover top-6 by variance** | Hardcoded metrics don't exist in scenario data |
| 2 | NumTrees | 100 | **30** | Sufficient, 3x less compute |
| 3 | TreeSize | 256 | **32** | Smaller window, faster adaptation |
| 4 | ShingleSize | 4 | **2** | Lower dimensionality |
| 5 | Alignment | Strict | **Forward-fill** | Handles irregular-interval data |

**Root cause from batch iteration**: The original RRCF hard-codes `cgroup.v2` metrics which don't exist in scenario data. The V5 fix auto-discovers top-N series by variance and forward-fills missing timestamps. This was entirely an engineering fix, not algorithmic — it went from 0.000 to 0.524.

**What to avoid**:
- ThresholdSigma too high (3.5 produced zero output in V2)
- OOM: auto-discover mode with too many series caused OOM on postmark. Cap at 6 series.

### 4.6 CorrShift — Priority: LOW

**File**: `comp/observer/impl/metrics_detector_corrshift.go`
**Current streaming avg F1**: 0.087
**Batch target**: 0.363 (V2)
**Problem**: Pagerduty=0.000, postmark=0.049. Has an unresolved tradeoff — smaller windows help postmark but hurt pagerduty.

**Proposed changes to try**:

| # | Parameter | Current | Try | Rationale |
|---|-----------|:---:|:---:|---|
| 1 | WindowSize | 15 | **12** | Catches faster correlation shifts |
| 2 | StepSize | 3 | **2** | Finer granularity |
| 3 | ThresholdSigma | 2.0 | **1.9** | Slightly more sensitive |
| 4 | BaselineThresholdFraction | 0.25 | **0.22** | Less data for threshold estimation |

**What worked in batch iteration**:
- V2 (best): above changes → 0.363 (+2.5%). Postmark unblocked (0.000→0.230) but pagerduty regressed (0.350→0.121)
- No variation solved the pagerduty/postmark tradeoff
- Fast-window dual pass was explored but fragile and didn't help

---

## 5. Goals and Approach

### What we want

Maximize the avg F1 across all 3 scenarios. The current baseline is weak — only 2 detectors produce any output at all, and the best avg F1 is 0.153. There is massive room for improvement. The batch-era results (Section 1) show what's *possible* — several detectors reached 0.5+ in batch mode.

### What's on the table

Everything. The current detector set and their parameters are a starting point, not a commitment. You are free to:

- **Fix existing detectors** — diagnose why 4 of 6 produce zero output and get them working
- **Tune parameters** — the proposed changes in Section 4 are guiding ideas from batch-era iteration, not prescriptions
- **Rewrite detector internals** — if a detector's architecture is fundamentally wrong for streaming, redesign it
- **Implement new algorithms** — if you find an approach that fits the streaming `Detector` interface and the eval framework, build it. Unregistered prototypes already exist for MMD, Spectral Residual, streaming MW, streaming CorrShift, and streaming TopK (see Section 6 key files).
- **Register unregistered prototypes** — the `metrics_detector_*_streaming.go` and `metrics_detector_mmd.go` files exist but aren't in `component_catalog.go`. Register and eval them.

### How to research

Before diving into code changes, build context:

1. **Read the detector you're about to change** — understand its `Detect()` method, what state it maintains, how it decides to emit anomalies
2. **Read `comp/observer/impl/component_catalog.go`** — understand how detectors are registered and instantiated
3. **Read the `seriesDetectorAdapter`** — if the detector implements `SeriesDetector` (batch), understand how the adapter wraps it for streaming. This is likely the root cause for zero-output detectors.
4. **Read existing tests** for the detector (`metrics_detector_*_test.go`) — understand what invariants are already tested
5. **Look at the streaming prototypes** (`*_streaming.go` files) — these were research implementations that may contain useful ideas or be ready to register
6. **Look at the scorer output** (`-verbose` JSON) — the raw prediction counts, warmup/cascading filter counts, and timestamps tell you exactly what the detector is doing wrong
7. **Search for relevant algorithms in the codebase and literature** — if a known technique (e.g., PELT, binary segmentation, online kernel methods) seems promising, research it and implement it

### How to iterate

1. **Start with the biggest wins** — 4 detectors produce zero output. Getting ANY of them to produce non-zero F1 is higher leverage than tuning BOCPD from 0.153 to 0.170.
2. **Quick-eval loop** — food_delivery_redis (~8 min) for fast feedback. Only run the full 3-scenario eval when you have a promising variation.
3. **One change at a time** — so you know what helped and what didn't.
4. **Record everything** — update the results table in Section 9 after each meaningful variation.
5. **Stop when diminishing returns** — if the last 3 attempts each improved avg F1 by <0.01, move to a different detector or approach.
6. **Don't be afraid to abandon a detector** — if after several attempts a detector seems fundamentally unsuited for streaming, move on. Time is better spent on detectors with higher ceiling.

### Starting points (not a strict order)

These are suggestions based on where the biggest gaps are. Follow your judgment.

| Detector | Current | Batch Reference | Observation |
|---|---|---|---|
| **topk** | 0.000 | 0.598 | Best batch detector. Zero streaming output — likely engineering issue. |
| **mannwhitney** | 0.000 | 0.559 | Strong batch. Zero streaming. Streaming prototype exists (0.482 avg in batch eval). |
| **rrcf** | 0.000 | 0.524 | Known root cause: hardcoded metrics that don't exist in scenario data. Engineering fix. |
| **bocpd** | 0.153 | 0.196 | Already works. Needs precision tuning (recall is 0.90+, precision is 0.06-0.11). |
| **cusum** | 0.000 | 0.187 | Fires too early, all predictions warmup-filtered. Needs robust baselines. |
| **corrshift** | 0.087 | 0.363 | Works but weak. Unresolved pagerduty/postmark tradeoff. |

---

## 6. Architecture Reference

### Detector Interfaces

```go
// Streaming (called every tick as data arrives)
type Detector interface {
    Detect(storage StorageReader, dataTime int64) DetectionResult
}

// Batch (called once with full series — wrapped by seriesDetectorAdapter for streaming)
type SeriesDetector interface {
    Detect(series Series) DetectionResult
}
```

The `seriesDetectorAdapter` wraps batch `SeriesDetector` implementations: on each tick, it calls `storage.GetSeriesRange(key, 0, dataTime, agg)` to get the full range, then delegates to the batch `Detect()`. This means batch detectors re-analyze all data every tick — O(n^2) total work.

### Key Files

| Category | Files |
|---|---|
| Detector implementations | `comp/observer/impl/metrics_detector_*.go` |
| Component catalog | `comp/observer/impl/component_catalog.go` |
| Testbench | `comp/observer/impl/testbench.go`, `testbench_registry.go` |
| Scorer | `comp/observer/impl/score.go`, `cmd/observer-scorer/` |
| Scenarios | `comp/observer/scenarios/{213_pagerduty,353_postmark,food_delivery_redis}/` |
| Streaming prototypes (unregistered) | `metrics_detector_mannwhitney_streaming.go`, `metrics_detector_corrshift_streaming.go`, `metrics_detector_topk_streaming.go` |
| New algorithms (unregistered) | `metrics_detector_mmd.go`, `metrics_detector_spectral.go` |

### Component Catalog Registration

Detectors are registered in `comp/observer/impl/component_catalog.go` in `defaultCatalog()`. To register a new detector, add a `componentEntry` to the catalog. The `testbenchCatalog()` function overrides defaults for testbench (disables cross_signal, enables time_cluster).

---

## 7. Success Criteria

| Metric | Current | Target | Stretch |
|---|---|---|---|
| Best single-detector avg F1 | 0.153 (bocpd) | 0.400 | 0.500 |
| Number of detectors with avg F1 > 0.1 | 2 (bocpd, corrshift) | 4 | 6 |
| Number of detectors with avg F1 > 0.3 | 0 | 2 | 3 |
| Best pagerduty F1 | 0.119 (bocpd) | 0.400 | 0.600 |
| Best postmark F1 | 0.195 (bocpd) | 0.300 | 0.450 |
| Best food_delivery F1 | 0.211 (corrshift) | 0.500 | 0.800 |

---

## 8. Key Lessons from Prior Iterations

1. **Simplest changes win**: 1-2 parameter tweaks consistently outperformed complex multi-filter approaches
2. **Selectivity > sophistication**: Reduce raw anomaly count before trying better statistics
3. **Clean eval is essential**: Always disable all other detectors — cross-detector interference costs 0.2-0.3 F1
4. **Test one thing at a time**: Change one parameter, eval, record. Don't batch multiple changes.
5. **Quick-eval first**: Run food_delivery_redis (~8 min) before committing to the full 3-scenario eval (~45-60 min)
6. **Postmark is hardest**: Most detectors score <0.25 on postmark. Don't over-optimize for it at the expense of the other two.
7. **Watch for overfitting**: All tuning is on 3 scenarios. If a parameter change helps one scenario a lot but hurts another, it's likely overfitting.
8. **Check scorer output carefully**: Look at num_scored, num_warmup_filtered, num_cascading_filtered. If most predictions are warmup-filtered, the detector fires too early. If most are cascading-filtered, it fires redundantly.

---

## 9. Logging and Reporting

**You MUST log all your work to files.** Your conversation context will be lost between sessions. The only way to preserve what you learned, tried, and concluded is to write it down. Treat these log files as your persistent memory.

### 9.1 Log Files

Maintain these files in the repo root. Create them on first use, append to them as you go.

**`iteration-log.md`** — Append an entry every time you try a variation. This is the detailed lab notebook. Each entry should include:

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

**`iteration-results.md`** — The summary scorecard. Update this after every variation that produces results. This is the quick-reference table someone can glance at to see the current best scores.

```markdown
# Iteration Results — Streaming Eval

Last updated: YYYY-MM-DD HH:MM

## Current Best Scores

| Detector | Best Variation | food_delivery | postmark | pagerduty | Avg F1 | Delta vs Baseline |
|---|---|---|---|---|---|---|
| bocpd | baseline | 0.146 | 0.195 | 0.119 | 0.153 | — |
| corrshift | baseline | 0.211 | 0.049 | 0.000 | 0.087 | — |
| cusum | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |
| rrcf | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |
| mannwhitney | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |
| topk | baseline | 0.000 | 0.000 | 0.000 | 0.000 | — |

## All Variations Tried

| # | Detector | Variation | Avg F1 | Delta | Keep? |
|---|---|---|---|---|---|
| 1 | ... | ... | ... | ... | ... |
```

**`research-notes.md`** — Log any research, diagnosis, or investigation that doesn't result in a scored variation. Examples:
- "Read seriesDetectorAdapter — it calls Detect() every tick with growing data range, confirmed O(n^2)"
- "MW produces 0 raw anomalies because WindowSize > series length on food_delivery"
- "Found that RRCF's TestBenchRRCFMetrics() returns empty slice for scenario data"
- Ideas for new approaches, algorithms you considered, why you chose one over another

### 9.2 When to Log

- **Before starting work on a detector**: Log your research/diagnosis in `research-notes.md`
- **After every eval run**: Log the full results in `iteration-log.md`, update `iteration-results.md`
- **After a revert**: Note it in `iteration-log.md` (so you don't re-try the same thing)
- **When you discover something non-obvious**: Log it in `research-notes.md` immediately — don't wait

### 9.3 Why This Matters

- These logs are how you avoid repeating failed experiments
- They're how the next session (or a human reviewer) understands what was tried and why
- The iteration-results table is the deliverable — it shows the impact of your work at a glance
- If you don't log it, it didn't happen
