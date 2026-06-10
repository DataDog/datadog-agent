# Anomaly Scoring Pipeline

This document describes the anomaly scoring and severity-detection pipeline that
converts a raw stream of per-metric anomaly events into a single smoothed
intensity signal and a severity level (Low / Medium / High).

The pipeline is implemented in Go inside the observer
(`comp/anomalydetection/observer/impl/anomaly_scorer.go`) and its output is
streamed to the testbench UI via `/api/scores` and `/api/scores/replay`.

---

## Pipeline overview

```
Raw anomalies (one or more detectors, any series)
      │
   Step 0 ── Deduplication (per series per second)
      │
   Step 1 ── Level assignment (detector + score → VeryLow … XHigh)
      │
   Step 2 ── 1-second bucketing (histogram of levels: bins[0..4])
      │
   Step 3 ── Weight projection  (weighted count: Σ bins[l] × weight[l])
      │
   Step 4 ── Saturation         (f(x) = x / (x + k);  EWMA feed)
      │
   Step 5 ── EWMA smoothing     (v_t = α·f(x) + (1-α)·v_{t-1})
      │
   Step 5b── Severity state machine (Low / Medium / High with hysteresis + cooldown)
      │
   ScoreState (ScoreBucket[] + SeverityEvent[])  ──→  testbench UI
```

---

## Step 0 — Deduplication

Multiple anomaly events for the **same series** may arrive within the same
second (e.g. from different rollup windows).  Only the one with the highest
`anomalyLevel` is kept per (series, second) pair.

Series are identified by `SourceRef.CompactID()` when available, falling back
to `Source.Key()`.

---

## Step 1 — Level assignment

Each surviving anomaly is mapped to one of five integer levels:

| Level | Value | Meaning      | Criteria (holt\_residual / tukey\_biweight / …) |
|-------|-------|--------------|--------------------------------------------------|
| 0     | 0.2w  | VeryLow      | score < 6 (or nil score)                        |
| 1     | 0.5w  | Low          | 6 ≤ score < 12                                  |
| 2     | 1.0w  | Medium       | 12 ≤ score < 20; also fixed for bocpd / unknown |
| 3     | 2.0w  | High         | 20 ≤ score < 35                                 |
| 4     | 3.0w  | XHigh        | score ≥ 35                                      |

`bocpd` and unrecognised detectors are always assigned **Medium** regardless of
the numeric score.

---

## Step 2 — 1-second bucketing

Within each one-second window the scorer accumulates a histogram
`bins[0..4]` where `bins[l]` is the number of deduplicated anomalies at level `l`.

The raw count `Σ bins[l]` is also stored.

---

## Step 3 — Weight projection

A weighted sum is computed:

```
weightSum = Σ  bins[l] × weight[l]
           l=0..4
```

with `weight = [0.2, 0.5, 1.0, 2.0, 3.0]`.

---

## Step 4 — Saturation

The weighted sum is fed through a soft saturation function that maps
\[0, ∞) → [0, 1):

```
f(x) = x / (x + k)
```

The saturation constant `k` (default `5`) controls at which count the signal
reaches 50%.

---

## Step 5 — EWMA smoothing

The saturated value is smoothed with an exponential moving average:

```
v_t = α · f(x_t) + (1 − α) · v_{t-1}
```

Default `α = 0.014` (strong smoothing; high-frequency spikes barely move the
signal, a sustained elevated period takes ~70 s to reach half-amplitude).

The EWMA value is stored in `ScoreBucket.ewma`.

---

## Step 5b — Severity state machine

The EWMA signal is compared to two thresholds to drive a three-state machine:

```
State: Low (0) ←──────────────── Medium (1) ←────── High (2)
                                      │                  │
         ewma < low_threshold         │                  │
         (after cooldown)             │                  │
                                      ▼                  ▼
                                  ewma ≥ low        ewma ≥ high
                                  → Medium          → High
```

### Hysteresis

Transitions **down** require a margin:

```
hysteresis_low  = low_threshold  × (1 − margin_pct)
hysteresis_high = high_threshold × (1 − margin_pct)
```

A downward transition from High → Medium only fires when
`ewma < hysteresis_high`, not merely `ewma < high_threshold`.

From High, the state can only drop one step per second
(High → Medium, never High → Low directly).

### Cooldown

`cooldown_secs` (default 300 s) prevents a downward transition from Medium →
Low until at least that many seconds have passed since the last upward
transition into Medium.

### Default parameter values

| Parameter      | Default | Meaning                           |
|----------------|---------|-----------------------------------|
| alpha          | 0.014   | EWMA smoothing factor             |
| saturation_k   | 5       | Saturation constant               |
| low_threshold  | 0.040   | EWMA level to enter Medium        |
| high_threshold | 0.060   | EWMA level to enter High          |
| margin_pct     | 0.20    | Hysteresis margin for downward    |
| cooldown_secs  | 300     | Seconds before Medium → Low drop  |

---

## ScoreState — telemetry contract (Go → UI)

```go
// observer/def/types.go

type ScoreBucket struct {
    Second    int64                   // Unix second
    Bins      [5]int                  // histogram: [VeryLow, Low, Medium, High, XHigh]
    Count     int                     // total anomalies in this second
    WeightSum float64                 // Σ bins[l] × weight[l]
    EWMA      float64                 // smoothed value after this bucket
}

type SeverityEvent struct {
    Timestamp int64                   // Unix second of transition
    FromLevel observer.SeverityLevel
    ToLevel   observer.SeverityLevel
}

type ScoreState struct {
    Buckets []ScoreBucket
    Events  []SeverityEvent
    Config  ScorerConfig
}
```

One `ScoreBucket` is emitted per second, even for seconds with no anomalies
(the EWMA decays naturally; `Count = 0`).

`SeverityEvent` is appended whenever the state machine transitions.

---

## Go implementation mapping

| Pipeline step | Go symbol(s)                                         |
|---------------|------------------------------------------------------|
| Step 0 dedup  | `anomalyScorer.pending` map, keyed by `seriesID()`   |
| Step 1 level  | `anomalyLevel(a)` → int 0–4                          |
| Step 2 bucket | `anomalyScorer.pending` map value: `pendingEntry`     |
| Step 3 weight | `weightedSum` in `flushSecond()`                     |
| Step 4 sat    | `saturate(x, k)` in `flushSecond()`                  |
| Step 5 EWMA   | `s.ewma = alpha*sat + (1-alpha)*s.ewma`              |
| Step 5b state | `nextSeverityLevel()` + `rawSeverityLevel()`          |
| Timer (live)  | Caller calls `Advance(unixSec)` once per second; in  |
|               | online mode this is driven by a real `time.Ticker`;  |
|               | the testbench replay calls `Advance` per scenario ts |
| Config        | `ScorerConfig` / `DefaultScorerConfig()`             |
| Telemetry out | `ScoreState()` snapshot                              |

### Online (live agent) vs testbench timer

- **Live agent**: the engine's main loop will contain a `time.Ticker` that fires
  every second and calls `engine.advanceScorers(time.Now().Unix())`.  This is
  not yet wired (planned); the `Advance` API is already stable.

- **Testbench replay**: `bench/api.go handleScoresReplay` sorts anomalies by
  timestamp and calls `scorer.ProcessAnomaly` + `scorer.Advance(sec)` for each
  unique second in the scenario, simulating the 1-second timer without a real
  clock.

---

## Testbench API

| Endpoint              | Method | Description                                     |
|-----------------------|--------|-------------------------------------------------|
| `/api/scores`         | GET    | Returns live `ScoreState` from the running scorer |
| `/api/scores/replay`  | POST   | Re-runs a fresh scorer over retained anomalies with a posted `ScorerConfig`; returns resulting `ScoreState` |

The replay endpoint is what the UI sliders use: each slider change POSTs the
updated config and re-renders the timeline without re-running detectors.
