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
| **Component** (`observerImpl`) | `impl/observer.go` | Fx lifecycle, channel dispatch, Handle factory, `agent_logs` tap |
| **Engine** (`engine`) | `impl/engine.go` | Storage, detection, correlation, replay — the shared core |

The engine is a plain Go struct, not an Fx component. Both the live observer
and the testbench use the same engine.

### Key files

| File | Purpose |
|------|---------|
| `def/component.go` | Component interface (GetHandle, RecordSamplerDropped, DumpMetrics) |
| `def/types.go` | Handle, View types, Detector, Correlator, StorageReader, Anomaly, CorrelatorEvent, etc. |
| `impl/engine.go` | Pipeline orchestration: ingest, advance, detect, correlate, replay |
| `impl/storage.go` | In-memory columnar time-series storage (1s buckets, read-time aggregation) |
| `impl/scheduler.go` | Scheduling policy: when to advance analysis |
| `impl/observer.go` | Fx component: lifecycle, channel loop, handle creation, log tap |
| `impl/component_catalog.go` | Registry of all detectors, correlators, extractors |
| `impl/agent_logs.go` | Agent internal log tap (source: `agent_logs`) |
| `impl/log_pattern_extractor.go` | Log → virtual metrics via pattern clustering |
| `impl/log_metrics_extractor.go` | Log → virtual metrics via regex extraction |
| `impl/anomaly_correlator_time_cluster.go` | Default time-proximity correlator |
| `impl/anomaly_correlator_passthrough.go` | Passthrough correlator (one ActiveCorrelation per anomaly) |
| `impl/anomaly_scorer.go` | Unified EWMA anomaly scorer (Correlator + standalone replay); derives severity, delegates push subscriptions to `severityevents/impl.Dispatcher` |
| `impl/correlation_emitter.go` | Shared first-seen/recurrence helper used by all non-scorer correlators |
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
| Correlator | `anomaly_scorer` | off |

Toggle detectors/correlators/extractors via `anomaly_detection.detectors.<name>.enabled` in datadog.yaml.

The `anomaly_scorer` correlator has a **dedicated config namespace** under `anomaly_detection.anomaly_scorer.*` (not `detectors.*`) with an `output` sub-section controlling logs and correlation events:

```yaml
anomaly_detection:
  anomaly_scorer:
    enabled: true
    alpha: 0.3
    window_secs: 30
    low_threshold: 0.030
    high_threshold: 0.060
    output:
      logs: true
      correlation_events: false
      cooldown_secs: 300
```

The scorer is also available standalone (without the engine) via `NewAnomalyScorer` in `impl/` for testbench replay.

### Severity event subscriptions

The scorer does not own subscription state itself. Each `anomalyScorer` owns a
`severityeventsimpl.Dispatcher` (see `../AGENTS.md#severity-events-scorer-push-contract`
and `../severityevents/`); on every `Advance` tick the scorer derives the raw
severity level from its EWMA and feeds it to `dispatcher.Advance(sec, level)`, which
runs each subscription's own cooldown/filter state machine and calls listeners.
`anomalyScorer.Subscribe` and `observerImpl.SubscribeScorer` are thin pass-throughs
to `dispatcher.SubscribeScorer`. The scorer's own internal watcher (gauges, logs,
episode tracking for `EpisodeStarted`/`EpisodeEnded`) is itself just a self-subscribed
listener, registered in `newAnomalyScorerWithTelemetry`.

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

### Correlator-owned deduplication (`correlationEmitter`)

All correlation event deduplication lives **inside each correlator**, not in reporters.
`correlationEmitter` (`impl/correlation_emitter.go`) is a shared helper embedded in
every non-scorer correlator (`TimeCluster`, `CrossSignal`, `Passthrough`). It tracks
first-seen / recurrence state and produces `CorrelatorEventCorrelationDetected` events
via `PendingEvents()`. The engine collects pending events after each `Advance` call and
forwards them to reporters via `ReportOutput.CorrelatorEvents`.

**Recurrence rule:** a pattern that leaves the active set (evicted, timeout) is
removed from the seen-set, so it re-fires on the next occurrence. This means
`CorrelationDetected` fires at most once per active episode, and once more each time
the pattern vanishes and comes back.

**Usage in a correlator:**

```go
// 1. In Advance — observe BEFORE evicting so batch-evicted clusters still emit.
e.emitter.observe(e.ActiveCorrelations(), dataTime)
// 2. In PendingEvents — drain and return.
return e.emitter.drain()
// 3. In Reset — clear emitter state alongside correlator state.
e.emitter.reset()
```

The scorer uses a different path (`EpisodeStarted` / `EpisodeEnded` events) and does
not embed a `correlationEmitter`.

## Common Pitfalls

1. **Don't call engine methods from multiple goroutines.** The engine assumes
   single-threaded advance.

2. **Event sinks must not block.** `emit()` is synchronous; a blocking sink
   stalls the entire ingestion loop.

3. **Detectors must not mutate storage.** They receive `StorageReader`
   (read-only). Violating this breaks deterministic replay.

4. **Extractor names must be unique.** The name is the storage namespace for
   derived metrics. Duplicates cause silent data collision.

5. **Agent internal logs are not logssource.** The internal tap is wired in
   `impl/observer.go`, gated by `anomaly_detection.logs.internal.*`.

6. **`Dispatcher.Advance`/`Reset` are scorer-owned, single-writer.** Only the
   `anomalyScorer` that owns a given `Dispatcher` instance should call `Advance`/`Reset`
   on it — per-subscription state is mutated without its own lock outside of the
   `subs` slice snapshot, relying on the scorer never calling `Advance`/`Reset`
   concurrently with itself. Listener callbacks run synchronously on whichever
   goroutine calls `Advance`, with no panic recovery or timeout — a slow or panicking
   subscriber affects every other subscriber and the scorer's own tick.

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
