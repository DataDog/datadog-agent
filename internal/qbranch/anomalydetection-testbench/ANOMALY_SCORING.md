# Anomaly Scoring & Severity Pipeline

This document describes the post-detector scoring pipeline that aggregates
raw anomaly events from multiple detectors into a single smoothed intensity
signal and derives discrete severity state transitions from it.

Steps 0–6 form the **core algorithm** that must be replicated in the live
Go agent. Step 7 (display-window aggregation) is **testbench UI only** and
has no equivalent in a live deployment.

---

## Part 1 — Overview (visual)

### 1.1 High-level data flow

```mermaid
flowchart LR
    D1([holt_residual\nraw score])  --> S[Score → Level]
    D2([tukey_biweight\nraw score]) --> S
    D3([bocpd\nno score])           --> S

    S -->|level 0‥4| Dedup[Deduplication\nsame series × same second]
    Dedup -->|one anomaly\nper series/sec| Bucket[1-second bucketing]
    Bucket -->|mean weight\n× saturation| EWMA[EWMA smoother\n1-second ticks]
    EWMA -->|smoothed\nintensity| SM[Severity state machine]
    SM --> E([Events\nLow / Medium / High])

    EWMA -.->|testbench display only| Agg[W-second bar\naggregation]
```

### 1.2 Scoring: raw score → unified level

```mermaid
flowchart TD
    A[Anomaly arrives] --> B{Detector has\na score?}
    B -- yes, holt_residual\nor tukey_biweight --> C[Apply score thresholds]
    B -- no, bocpd --> D[Fixed level: Medium]
    B -- unknown detector --> D2[Default level: Medium]

    C --> T{score}
    T -- < 6    --> L0[VeryLow · w=0.2]
    T -- 6‥11   --> L1[Low · w=0.5]
    T -- 12‥19  --> L2[Medium · w=1.0]
    T -- 20‥34  --> L3[High · w=2.0]
    T -- ≥ 35   --> L4[XHigh · w=3.0]
```

### 1.3 From 1-second buckets to severity events

```mermaid
flowchart LR
    subgraph "Per 1-second bucket (live agent tick)"
        B1[sum weights\nin second] -->|÷ count| MW[mean weight]
        MW -->|× saturation\n1−e^−n/k| SI[saturated input]
    end

    SI -->|α=0.16| EW[EWMA]

    subgraph "Severity state machine"
        EW --> CMP{compare\nto thresholds}
        CMP -- EWMA ≥ high+margin  --> H[High]
        CMP -- EWMA ≥ low+margin   --> M[Medium]
        CMP -- EWMA < low−margin   --> L[Low]

        H -->|after cooldown\n−1 level at a time| M
        M -->|after cooldown| L
    end
```

### 1.4 Severity state machine — transition rules

```mermaid
stateDiagram-v2
    [*] --> Low : initial state from\nrawLevel(ewma₀)

    Low --> Medium : EWMA ≥ low + margin
    Low --> High   : EWMA ≥ high + margin

    Medium --> High : EWMA ≥ high + margin
    Medium --> Low  : EWMA < low − margin\nAND cooldown elapsed

    High --> Medium : EWMA < high − margin\nAND cooldown elapsed
    note right of High : can only drop one level\nat a time (no High→Low)
```

---

## Part 2 — Full algorithm specification

### 2.1 Step 0 — Deduplication

When multiple detectors fire on the **same series** at the **same Unix second**,
only the highest-level anomaly is kept.  This prevents one physical incident
from inflating the EWMA count.

```
key = floor(anomaly.timestamp) + ":" + anomaly.sourceSeriesId
For each key, keep the anomaly with the highest level.
Anomalies with no sourceSeriesId are never merged.
```

---

### 2.2 Step 1 — Level assignment

Each anomaly is mapped to an integer level **L ∈ {0, 1, 2, 3, 4}** and a
corresponding weight **w**.

| Level | Name     | Weight w |
|-------|----------|----------|
| 0     | VeryLow  | 0.2      |
| 1     | Low      | 0.5      |
| 2     | Medium   | 1.0      |
| 3     | High     | 2.0      |
| 4     | XHigh    | 3.0      |

**Scored detectors** (`holt_residual`, `tukey_biweight`) — apply threshold table:

| Score range | Level |
|-------------|-------|
| score < 6   | 0 — VeryLow |
| 6 ≤ score < 12  | 1 — Low |
| 12 ≤ score < 20 | 2 — Medium |
| 20 ≤ score < 35 | 3 — High |
| score ≥ 35  | 4 — XHigh |

Thresholds were calibrated from 3 scenarios (dns-upstream-outage,
kafka-partition-saturation, postmark):
- Baseline: mean = 8.3, P95 = 15.8
- Disruption: P50 = 13.1, P95 = 36.8, P99 = 49.4

**Fixed-level detectors:**

| Detector | Level | Reason |
|----------|-------|--------|
| `bocpd`  | 2 — Medium (w = 1.0) | Emits no score; change-point detection is a reliable signal |

**Default (unknown detector):** Level 2 — Medium (w = 1.0)

---

### 2.3 Step 2 — 1-second bucketing

Anomalies are placed into 1-second integer buckets by `floor(timestamp)`.

For each 1-second bucket `s`:

```
bins[s][L]   += 1          for each anomaly at level L
weightSum[s] += w(L)       for each anomaly
count[s]     += 1
```

---

### 2.4 Step 3 — Saturated input (per second)

For each 1-second bucket `s`, compute the saturated EWMA input:

```
meanWeight_s = weightSum[s] / count[s]     (0 if count[s] = 0)

input_s = meanWeight_s × (1 − exp(−count[s] / k))
```

The saturation factor `(1 − exp(−n/k))` dampens the mean when `n` is small
(sparse seconds) and approaches 1 as `n → ∞`.

**Default constant:** `k = 5`

| n (anomaly count) | saturation factor (k=5) |
|-------------------|------------------------|
| 1  | 0.18 |
| 3  | 0.45 |
| 5  | 0.63 |
| 10 | 0.86 |
| 20 | 0.98 |

---

### 2.5 Step 4 — EWMA (1-second ticks)

```
ewma[0] = input[0]
ewma[i] = α × input[i] + (1 − α) × ewma[i−1]
```

`i` indexes 1-second buckets; one EWMA update per second in the live agent.

**Default constant:** `α = 0.16`

Recent inputs are weighted exponentially more than older ones.  Higher `α`
makes the signal react faster to new data; lower `α` smooths over longer
windows.  At α = 0.16, the effective memory half-life is roughly
`−1 / log₂(1 − α) ≈ 4 seconds`.

---

### 2.6 Step 5 — Severity state machine

The EWMA stream drives a 3-state machine: **Low (0)**, **Medium (1)**,
**High (2)**.

The **initial state** is computed directly from `ewma[0]` using the raw
thresholds (no hysteresis): `≥ high → High`, `≥ low → Medium`, else `Low`.
In practice `ewma[0] = input[0]` which is near zero, so the machine
always starts at Low.  If a scenario opens mid-incident (EWMA seed already
elevated), the correct state is entered immediately.

#### Thresholds

| Parameter | Default | Description |
|-----------|---------|-------------|
| `low`     | 0.25    | EWMA level that defines the Low/Medium boundary |
| `high`    | 0.50    | EWMA level that defines the Medium/High boundary |
| `margin`  | 0.15    | Hysteresis half-width (avoids chattering at boundaries) |
| `cooldown`| 300 s   | Minimum time to spend in any elevated state before stepping down |

> **Note on threshold calibration:** the default values above were tuned
> interactively on the testbench with 1-second EWMA ticks.  They will need
> re-validation once the algorithm runs on live production traffic.

#### Transition logic

From state `cur`, the **target** state for EWMA value `v` is:

```
cur = Low (0):
  v ≥ high + margin  →  High (2)      # direct jump to High allowed on way up
  v ≥ low  + margin  →  Medium (1)
  otherwise          →  Low (0)        # no change

cur = Medium (1):
  v ≥ high + margin  →  High (2)
  v <  low  − margin →  Low (0)
  otherwise          →  Medium (1)     # no change

cur = High (2):
  v <  high − margin →  Medium (1)    # one step down only — never High→Low directly
  otherwise          →  High (2)      # no change
```

#### Cooldown enforcement

A **decrease** transition (target < cur) is **suppressed** if:

```
now − lastStateEntryTimestamp < cooldown
```

`lastStateEntryTimestamp` is updated on **every** transition (increases and
decreases alike), so the cooldown timer resets each time a new state is
entered.  This ensures the cascade `High → Medium → Low` takes at minimum
`2 × cooldown` total time.

`lastStateEntryTimestamp` is initialised to `−∞`, which means the first
decrease from the initial state is **never** blocked — correct when a
scenario opens in an already-elevated state that the pipeline did not
itself cause.

---

### 2.7 Step 6 — Display-window aggregation *(testbench UI only)*

> **This step does not exist in the live Go agent.**  It is purely a
> rendering optimisation: the testbench replays a full recorded scenario at
> once and needs to fit potentially hours of data into a fixed-width chart.

1-second buckets and the already-computed `ewmaPerSecond` array are
aggregated into display bars of width `W` seconds.

```
W = user-selected seconds/bar   (default: auto-fit to ~80 bars)

For display bar i spanning [t_i, t_i + W):
  bins_i[L]    = Σ  bins[s][L]     for s in [t_i, t_i+W)
  total_i      = Σ  count[s]       for s in [t_i, t_i+W)
  ewmaValue_i  = ewmaPerSecond[last s in window]   # most-recent EWMA in bar
```

The bar chart renders `bins_i` as stacked colours and the EWMA line renders
`ewmaValue_i` as the representative EWMA for that bar.  Severity event
triangles are pinned to their exact 1-second timestamps, not to bar centres.

---

### 2.8 Constants summary

| Constant | Default | Notes |
|----------|---------|-------|
| Score thresholds | `[6, 12, 20, 35]` | Calibrated from 3 scenarios |
| `LEVEL_WEIGHTS`  | `[0.2, 0.5, 1.0, 2.0, 3.0]` | Per-level EWMA weight |
| `bocpd` fixed level | 2 (Medium, w=1.0) | No score emitted |
| Saturation k | 5 | Count at which saturation ≈ 63 % |
| EWMA α | 0.16 | Smoothing factor; half-life ≈ 4 s at 1-second tick rate |
| Low threshold | 0.25 | EWMA units (tuned on testbench, re-validate on live data) |
| High threshold | 0.50 | EWMA units (tuned on testbench, re-validate on live data) |
| Hysteresis margin | 0.15 | EWMA units |
| Cooldown | 300 s (5 min) | Minimum dwell time per elevated state |
