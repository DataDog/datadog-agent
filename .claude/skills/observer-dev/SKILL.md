---
name: observer-dev
description: >
  Development guide for comp/observer — the anomaly detection component. Use
  this skill when working on observer internals: adding new signal types,
  writing detectors or correlators, modifying the engine pipeline, adding Handle
  methods, creating HF runners, or understanding how metrics flow from checks
  through the observer. Also trigger on mentions of "observer component",
  "anomaly detection pipeline", "detector", "correlator", "Handle interface",
  or any work touching files in comp/observer/.
---

# Observer Development Guide

## Architecture

The observer is a self-contained anomaly detection engine that sits alongside
the normal agent pipeline. It receives copies of metrics, logs, traces, and
lifecycle events via lightweight handles, stores them in an internal time
series database, and runs detectors and correlators to find anomalies.

### Data flow

```
Check/DogStatsD/TraceAgent
  → handle.ObserveMetric(sample)        # non-blocking channel send
    → observation channel (cap 1000)
      → run() loop (single goroutine)
        → engine.IngestMetric(source, metricObs)
          → storage.Add(source, name, value, ts, tags)
          → scheduler.onObservation(ts, state)
            → advanceRequest{upToSec, reason}
              → engine.Advance(upToSec)
                → detector.Detect(storage, dataTime) for each detector
                → correlator.ProcessAnomaly() + Advance()
                → emit events to reporters
```

### Key types

| Type | File | Purpose |
|------|------|---------|
| `observation` | observer.go | Union struct: metric/log/trace/profile/lifecycle |
| `engine` | engine.go | Orchestrates storage, detection, correlation |
| `timeSeriesStorage` | storage.go | In-memory time series with summary stats |
| `Detector` | component.go | Pulls data from storage, returns anomalies |
| `SeriesDetector` | component.go | Simpler: receives a single Series, returns anomalies |
| `Correlator` | component.go | Clusters anomalies into patterns |
| `Handle` | component.go | Lightweight observation interface for data sources |

### Handle interface

Every data source gets a Handle via `observer.GetHandle("source-name")`. The
Handle has methods for each signal type:

- `ObserveMetric(MetricView)` — check metrics, DogStatsD, HF runner
- `ObserveLog(LogView)` — log messages
- `ObserveTrace(TraceView)` — APM traces
- `ObserveTraceStats(TraceStatsView)` — pre-aggregated APM stats
- `ObserveProfile(ProfileView)` — profiling samples
- `ObserveLifecycle(LifecycleView)` — container create/start/delete events

All sends are non-blocking. When the channel is full, both `dropCount`
(atomic, for parity debugging) and `dropCounter` (telemetry, tagged by source)
are incremented.

## Adding a new observation type

Follow this pattern (used for LifecycleView):

1. **Interface** — add to `comp/observer/def/component.go`:
   - New `FooView` interface with getter methods
   - New `ObserveFoo(FooView)` method on `Handle`

2. **Obs struct** — in `observer.go`:
   - `fooObs` struct copying FooView fields
   - Implement FooView getters on it
   - Add `foo *fooObs` field to `observation` struct

3. **Handle implementations** — update ALL of these:
   - `handle` — copy data, send to channel (with drop counters)
   - `noopObserveHandle` — empty body
   - `hfFilteredHandle` — pass through to inner
   - `storageHandle` (testbench.go) — empty body or store
   - `recordingHandle` (recorder.go) — forward + write to storage

4. **run() loop** — add `if obs.foo != nil { ... }` handling

5. **Recorder** — add a writer if the data should be persisted (JSONL for
   low-volume events, parquet for high-volume structured data)

## HF runner pattern

The high-frequency runner (`comp/observer/impl/hfrunner/`) runs existing agent
checks at 1-second intervals, routing output directly into the observer via a
custom `sender.Sender` implementation.

### Key components

- `observerSenderManager` — implements `sender.SenderManager`, returns
  `observerSender` instances
- `observerSender` — implements `sender.Sender`, routes Gauge/Rate/Count/etc.
  to `handle.ObserveMetric()`
- `Runner` — instantiates check factories, runs them on a 1s tick with
  retry/backoff

### Delta normalization

Rate and MonotonicCount checks emit cumulative values. The normal aggregator
converts these to per-second rates and deltas. The `observerSender` must do
the same via `prevSample` tracking:

- `Rate()` → `(current - previous) / elapsed_seconds`
- `MonotonicCount()` → `current - previous` (handles counter wraps)
- First sample silently dropped (no previous to diff against)

### Source-based filtering

When HF checks run at 1s, the 15s versions from the normal pipeline must be
suppressed so the scorer doesn't double-count. This uses:

- `MetricSource` enum (from `pkg/metrics/metricsource.go`)
- `sourceProvider` interface (type assertion on `*metrics.MetricSample`)
- `hfFilteredHandle` wrapping the `"all-metrics"` handle with a dynamic
  source set

The filter activates ONLY after the runner starts successfully — not on config
flag alone. This prevents suppressing 15s data when the HF replacement fails.

### Adding deps for container checks

System checks need no deps. Container checks need WMeta, FilterStore, Tagger:

```go
type ContainerDeps struct {
    WMeta       workloadmetadef.Component
    FilterStore workloadfilterdef.Component
    Tagger      taggerdef.Component
}
func NewContainer(handle, deps ContainerDeps) *Runner
```

These come from `option.Option[T]` fields in observer's `Requires`. The
options are provided by each dep's own fx module — don't add `ProvideOptional`
to the observer's `fx.go`.

## Key files

| File | What's there |
|------|-------------|
| `comp/observer/def/component.go` | All public interfaces |
| `comp/observer/impl/observer.go` | Handles, plumbing, run loop, NewComponent |
| `comp/observer/impl/engine.go` | Detection/correlation pipeline |
| `comp/observer/impl/storage.go` | Time series storage |
| `comp/observer/impl/telemetry.go` | Telemetry constants and handler |
| `comp/observer/impl/hfrunner/` | HF check runner + sender |
| `comp/observer/impl/component_catalog.go` | Detector/correlator registration |
| `comp/observer/impl/lifecycle_watcher.go` | WLM container event subscription |
| `comp/observer/fx/fx.go` | fx module |
| `pkg/config/setup/config.go` | Config keys (search `observer.`) |

## Config keys

```yaml
observer:
  analysis:
    enabled: true                              # enable detection pipeline
  high_frequency_system_checks:
    enabled: false                             # 1s system checks
  high_frequency_container_checks:
    enabled: false                             # 1s container checks
  recording:
    enabled: false                             # parquet recording
    parquet_output_dir: /tmp/observer-parquet
    parquet_flush_interval: 30s
  debug_dump_path: ""                          # JSON dump of storage
  debug_dump_interval: 0                       # dump interval
  components:
    bocpd:
      enabled: true                            # per-detector toggles
    rrcf:
      enabled: true
    scanwelch:
      enabled: false
    time_cluster:
      enabled: true
```

## Build and test

```bash
go build ./comp/observer/impl/...
go test -count=1 ./comp/observer/impl/...
```
