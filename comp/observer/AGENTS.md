# comp/observer — AI Agent Guide

## What This Component Does

The observer watches data flowing through the agent — metrics, logs, traces,
profiles — and runs anomaly detection on it. It is a pipeline:

```
Handle → Storage → Detect → Correlate → Report
```

Data enters through lightweight **Handles** (non-blocking, copy-on-send).
The **engine** stores metrics, runs detectors and correlators, and emits
events to reporters. See `README.md` for the full pipeline diagram and
extension guide.

## Architecture

### Two layers

| Layer | Code | Role |
|-------|------|------|
| **Component** (`observerImpl`) | `impl/observer.go` | Fx lifecycle, channel dispatch, Handle factory, HF check runners |
| **Engine** (`engine`) | `impl/engine.go` | Storage, detection, correlation, replay — the shared core |

The engine is a plain Go struct, not an Fx component. Both the live
observer and the testbench (`impl/testbench.go`) use the same engine.

### Key files

| File | Purpose |
|------|---------|
| `def/component.go` | Public interfaces: Component, Handle, View types, Detector, Correlator, etc. |
| `impl/engine.go` | Pipeline orchestration: ingest, advance, detect, correlate, replay |
| `impl/storage.go` | In-memory columnar time-series storage (1s buckets, read-time aggregation) |
| `impl/scheduler.go` | Scheduling policy: when to advance analysis |
| `impl/stateview.go` | Read-only introspection over engine state |
| `impl/events.go` | Engine event system (advance completed, anomaly created, correlation updated) |
| `impl/component_catalog.go` | Registry of all detectors, correlators, extractors |
| `impl/observer.go` | Fx component: lifecycle, channel loop, handle creation |
| `impl/testbench.go` | Offline replay and evaluation harness |

### Detectors (in `impl/`)

| File | Algorithm | Type |
|------|-----------|------|
| `metrics_detector_bocpd.go` | Bayesian Online Changepoint Detection | SeriesDetector (incremental) |
| `metrics_detector_cusum.go` | Cumulative Sum control chart | SeriesDetector (stateless) |
| `metrics_detector_rrcf.go` | Robust Random Cut Forest | Detector (multivariate) |
| `metrics_detector_scanmw.go` | Scan + Mann-Whitney U test | SeriesDetector |
| `metrics_detector_scanwelch.go` | Scan + Welch's t-test | SeriesDetector |
| `log_detector_connection_errors.go` | Connection error pattern matching | LogMetricsExtractor |

### Correlators (in `impl/`)

| File | Algorithm |
|------|-----------|
| `anomaly_correlator_time_cluster.go` | Time-proximity clustering |
| `anomaly_processor_correlator.go` | Cross-signal pattern matching |
| `anomaly_correlator_passthrough.go` | Pass-through (testing) |

### Extractors (in `impl/`)

| File | Purpose |
|------|---------|
| `log_metrics_extractor.go` | Numeric field extraction + pattern signatures from logs |
| `log_pattern_extractor.go` | Drain-style log pattern clustering |
| `log_detector_connection_errors.go` | Connection error counting |

## Allium Specification

The file `observer-engine.allium` is the **behavioral specification** for
the engine, written in [Allium](../../.agents/skills/allium/SKILL.md) v3.

### What it captures

- **17 rules** covering ingestion, scheduling, advance, anomaly lifecycle,
  correlation, replay, reset, and component hot-swap
- **5 contracts** defining the obligations between the engine and its
  pluggable components (DetectorContract, CorrelatorContract,
  ExtractorContract, StorageReaderContract, SchedulerPolicy)
- **5 invariants** (AnomalyUniqueness, CorrelationBound, NonNegativeProgress,
  UniqueSeriesRefs, AnomalyCountNonNegative)
- **2 surfaces** (StateView, ComponentCatalog)
- **4 open questions** with specific evidence and proposed solutions

### How to use it

**The spec is authoritative for behavior.** If the code disagrees with
the Allium spec, one of them is wrong. The transition graph, preconditions,
and invariants in the `.allium` file define correct behavior. `@guidance`
blocks describe implementation sequences — if the code's sequence differs,
investigate before assuming the code is right.

**Open questions are unresolved ambiguities**, not documentation. Each one
contains specific evidence from the codebase, concrete options, and (where
applicable) the reason the decision is blocked. Resolving them requires a
design decision, not just code.

**Reading order:**
1. `README.md` — pipeline overview, extension guide, configuration
2. `observer-engine.allium` — behavioral spec (rules, contracts, invariants)
3. `def/component.go` — public interfaces
4. `impl/engine.go` — implementation of the spec

### Allium quick reference

- `rule` = what happens when a trigger fires (preconditions + postconditions)
- `contract` = obligations between the engine and a pluggable component
- `invariant` = property that must always hold over entity state
- `surface` = boundary contract (what's exposed, what operations are available)
- `@guidance` = non-normative implementation advice
- `open question` = unresolved design ambiguity with evidence
- `deferred` = spec exists elsewhere

Full language reference: `.agents/skills/allium/references/language-reference.md`

## Key Design Decisions

### Data-driven scheduling ("complete seconds" rule)

Detection is NOT on a timer. When data at time T arrives, the engine
advances analysis to T-1. This ensures deterministic replay: same data →
same anomalies. See `ScheduleOnObservation` rule in the spec and
`currentBehaviorPolicy` in `scheduler.go`.

### Read-time aggregation

Storage keeps full summary stats (sum/count/min/max) per 1-second bucket.
Aggregation kind (avg, sum, count, min, max) is chosen when reading, not
when writing. This means detectors can pick any aggregation without
re-ingesting data.

### Non-blocking ingestion

Handles do non-blocking sends to a buffered channel (cap 1000). If the
channel is full, observations are silently dropped. Analysis never
back-pressures data ingestion.

### Single dispatch goroutine

All processing after the channel runs sequentially in `run()`. This is
currently a structural requirement (see open question in the spec about
whether this is intentional).

## Common Pitfalls

1. **Don't call engine methods from multiple goroutines.** The engine
   assumes single-threaded advance. See the sequential advance open
   question in the spec.

2. **Event sinks must not block.** `emit()` is synchronous. A blocking
   sink stalls ingestion.

3. **Detectors must not mutate storage.** They receive `StorageReader`
   (read-only). Violating this breaks deterministic replay.

4. **Extractor names must be unique.** The name is used as the storage
   namespace for derived metrics. Duplicates cause silent data collision.

5. **Trace stats are not processed.** `ObserveTraceStats` is a no-op.
   Trace stats data is still recorded to parquet by the recorder but
   the observer does not derive metrics from it.

## Testing

```bash
# Run observer tests
dda inv test --targets=./comp/observer/...

# Run benchmarks
dda inv test --targets=./comp/observer/impl/ -- -bench=.
```

The testbench (`impl/testbench.go`) replays parquet scenario data through
the same engine used live. Scenario data lives in `scenarios/`.
