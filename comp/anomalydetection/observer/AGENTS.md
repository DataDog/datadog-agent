# comp/anomalydetection/observer — AI Agent Guide

Subsystem overview, wiring, and config: see `../AGENTS.md`.

## What This Component Does

The observer watches data flowing through the agent and runs anomaly detection
on it:

```
Handle → Storage → Detect → Correlate → Report
```

Data enters through lightweight **Handles** (non-blocking, copy-on-send).
The **engine** stores metrics, runs detectors and correlators, and emits
events to reporters injected via the `anomalydetection_reporters` Fx group.

## Architecture

### Two layers

| Layer | Code | Role |
|-------|------|------|
| **Component** (`observerImpl`) | `impl/observer.go` | Fx lifecycle, channel dispatch, Handle factory, agent-internal log tap |
| **Engine** (`engine`) | `impl/engine.go` | Storage, detection, correlation, replay — the shared core |

The engine is a plain Go struct, not an Fx component. Both the live observer
and the testbench use the same engine.

### Key files

| File | Purpose |
|------|---------|
| `def/component.go` | Component interface (GetHandle, RecordSamplerDropped, DumpMetrics) |
| `def/types.go` | Handle, View types, Detector, Correlator, StorageReader, Anomaly, etc. |
| `impl/engine.go` | Pipeline orchestration: ingest, advance, detect, correlate, replay |
| `impl/storage.go` | In-memory columnar time-series storage (1s buckets, read-time aggregation) |
| `impl/scheduler.go` | Scheduling policy: when to advance analysis |
| `impl/observer.go` | Fx component: lifecycle, channel loop, handle creation, log tap |
| `impl/component_catalog.go` | Registry of all detectors, correlators, extractors |
| `impl/agent_logs.go` | Agent-internal log tap (source: `agent-internal-logs`) |
| `impl/log_pattern_extractor.go` | Log → virtual metrics via pattern clustering |
| `impl/log_metrics_extractor.go` | Log → virtual metrics via regex extraction |
| `impl/anomaly_correlator_time_cluster.go` | Default time-proximity correlator |
| `impl/patterns/` | Tokenizer + clusterer used by log pattern extractor |

### Component catalog (defaults)

Registered in `impl/component_catalog.go`. Enabled by default unless noted:

| Kind | Name | Default |
|------|------|---------|
| Extractor | `log_metrics_extractor` | on |
| Extractor | `log_pattern_extractor` | on |
| Extractor | `connection_error_extractor` | off |
| Detector | `bocpd` | on |
| Detector | `rrcf` | on |
| Detector | `cusum`, `scanmw`, `scanwelch`, `holt_residual`, `tukey_biweight` | off |
| Correlator | `time_cluster` | on |
| Correlator | `cross_signal`, `passthrough` | off |

Toggle via `anomaly_detection.detectors.<name>.enabled` in datadog.yaml.

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

### Metric ingestion gate

When `anomaly_detection.metrics.enabled=false`, handles wrap with
`metricDropHandle` so external metrics are dropped at the edge. `ObserveLog`
still passes through; log-derived virtual metrics produced inside the engine
are unaffected.

## Common Pitfalls

1. **Don't call engine methods from multiple goroutines.** The engine assumes
   single-threaded advance.

2. **Event sinks must not block.** `emit()` is synchronous; a blocking sink
   stalls the entire ingestion loop.

3. **Detectors must not mutate storage.** They receive `StorageReader`
   (read-only). Violating this breaks deterministic replay.

4. **Extractor names must be unique.** The name is the storage namespace for
   derived metrics. Duplicates cause silent data collision.

5. **Agent-internal logs are not logssource.** The internal tap is wired in
   `impl/observer.go`, gated by `anomaly_detection.logs.internal.*`.

## Testing

```bash
dda inv test --targets=./comp/anomalydetection/observer/...
dda inv test --targets=./comp/anomalydetection/observer/impl/ -- -bench=.
```

**Testbench** (algorithm iteration + scenario replay):

```bash
dda inv anomalydetection.build-testbench
dda inv anomalydetection.launch-testbench
```

Reads scenarios from `observer/scenarios/`. See
`internal/qbranch/anomalydetection-testbench/README.md`.
