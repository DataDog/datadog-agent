# comp/anomalydetection/observer — AI Agent Guide

## What This Component Does

The observer watches data flowing through the agent and runs anomaly detection
on it. It is a pipeline:

```
Handle → Storage → Detect → Correlate → Report
```

Data enters through lightweight **Handles** (non-blocking, copy-on-send).
The **engine** stores metrics, runs detectors and correlators, and emits
events to reporters. See `README.md` for the full pipeline diagram and
extension guide.

## Component structure

The observer lives inside `comp/anomalydetection/` alongside three sibling
components it depends on at runtime:

```
comp/anomalydetection/
  observer/
    def/        ← public interfaces (own go.mod)
    fx/         ← production Fx wiring
    impl/       ← engine, detectors, correlators, extractors, telemetry
      hfrunner/ ← high-frequency check runner subpackage
      patterns/ ← log pattern tokenizer/clusterer subpackage
  logssource/   ← container log source feeding the observer
    def/ fx/ impl/
  reporter/     ← anomaly event reporter
    def/
    fx/           ← production (Datadog notify + events API)
    fx-noop/      ← stub wired in the main agent build
    fx-testbench/ ← SSE debug reporter wired in the testbench
    impl/
    impl-testbench/
    mock/
  recorder/     ← parquet recorder for scenario capture
    def/
    fx/         ← full implementation (parquet, heavy deps)
    fx-noop/    ← stub wired in the main agent build
    impl/
    impl-noop/
```

The **production agent** wires `reporter/fx-noop` and `recorder/fx-noop` to
keep those heavy dependencies out of the agent binary. The
**testbench** (`internal/qbranch/anomalydetection-testbench/`) wires the full
`fx` + `impl` variants.

## Architecture

### Two layers

| Layer | Code | Role |
|-------|------|------|
| **Component** (`observerImpl`) | `impl/observer.go` | Fx lifecycle, channel dispatch, Handle factory, HF check runners |
| **Engine** (`engine`) | `impl/engine.go` | Storage, detection, correlation, replay — the shared core |

The engine is a plain Go struct, not an Fx component. Both the live observer
and the testbench use the same engine.

### Key files

| File | Purpose |
|------|---------|
| `def/component.go` | Public interfaces: Component, Handle, View types, Detector, Correlator, etc. |
| `impl/engine.go` | Pipeline orchestration: ingest, advance, detect, correlate, replay |
| `impl/storage.go` | In-memory columnar time-series storage (1s buckets, read-time aggregation) |
| `impl/scheduler.go` | Scheduling policy: when to advance analysis |
| `impl/observer.go` | Fx component: lifecycle, channel loop, handle creation |
| `impl/component_catalog.go` | Registry of all detectors, correlators, extractors |

## Allium Specification

`observer-engine.allium` is the **behavioral specification** for the engine,
written in [Allium](../../.agents/skills/allium/SKILL.md).

The spec is authoritative for behavior. If the code disagrees with the spec,
one of them is wrong. Open questions in the spec are unresolved design
ambiguities, not documentation — resolving them requires a decision, not just
code.

**Reading order:**
1. `README.md` — pipeline overview, extension guide, configuration
2. `observer-engine.allium` — behavioral spec (rules, contracts, invariants)
3. `def/component.go` — public interfaces
4. `impl/engine.go` — implementation of the spec

## Key Design Decisions

### Data-driven scheduling ("complete seconds" rule)

Detection is NOT on a timer. When data at time T arrives, the engine advances
analysis to T-1. This ensures deterministic replay: same data → same anomalies.

### Read-time aggregation

Storage keeps full summary stats (sum/count/min/max) per 1-second bucket.
Aggregation kind (avg, sum, count, min, max) is chosen when reading, not when
writing. Detectors can pick any aggregation without re-ingesting data.

### Non-blocking ingestion

Handles do non-blocking sends to a buffered channel. If the channel is full,
observations are silently dropped. Analysis never back-pressures data ingestion.

## Common Pitfalls

1. **Don't call engine methods from multiple goroutines.** The engine assumes
   single-threaded advance.

2. **Event sinks must not block.** `emit()` is synchronous; a blocking sink
   stalls the entire ingestion loop.

3. **Detectors must not mutate storage.** They receive `StorageReader`
   (read-only). Violating this breaks deterministic replay.

4. **Extractor names must be unique.** The name is the storage namespace for
   derived metrics. Duplicates cause silent data collision.

## Testing

```bash
dda inv test --targets=./comp/anomalydetection/observer/...
dda inv test --targets=./comp/anomalydetection/observer/impl/ -- -bench=.
```
