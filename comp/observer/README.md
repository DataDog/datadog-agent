# Observer Component

The observer watches data flowing through the agent — metrics, logs, traces, profiles — and runs anomaly detection on it. The core idea is a pipeline:

```
                      ┌─────────────────────────────────────────────────────────────────────┐
                      │                                                                     │
┌────────────┐        │    ┌───────────────────────┐    ┌───────────────────────────┐       │
│   Handle   ├─Logs───┼─┬──▶  LogMetricExtractors  │    │         Detectors         │       │
└──────┬─────┘        │ │  └───────────┬───────────┘    │                           │       │
       │              │ │              │                │  ┌─────────────────────┐  │       │
       │              │ └──────────────┼────────────────┼──▶     LogObserver     │  ├───┐   │
       │              │                │                │  └─────────────────────┘  │   │   │
       │              │                │                │                           │   │   │
       │              │    ┌───────────▼───────────┐    │  ┌─────────────────────┐  │   │   │
       └───Metrics────┼────▶     MetricStorage     ├────▶  │   SeriesDetector    │  │   │   │
                      │    └───────────────────────┘    │  └─────────────────────┘  │   │   │
                      │                                 └───────────────────────────┘   │   │
                      │                                                                 │   │
                      │        ┌──────────────────┐                                     │   │
                      │        │   Correlators    ◀─────────────Anomalies───────────────┘   │
                      │        └────────┬─────────┘                                         │
                      │                 │                                                   │
                      │        ┌────────▼─────────┐                                         │
                      │        │    Reporters     │                                         │
                      │        └──────────────────┘                                         │
                      │                                                                     │
                      └─────────────────────────────────────────────────────────────────────┘
```

Data enters through lightweight **Handles** that copy and send observations over a channel. The framework handles scheduling, threading, and data flow — individual components (detectors, extractors, correlators) currently run sequentially in a single goroutine, so they don't need to be thread-safe or manage shared state. This may evolve (e.g. parallelizing independent detectors), but component implementations should not depend on the threading model.

## Pipeline Stages

### 1. Observe (Handles)

**Component** is the entry point. Call `GetHandle(name)` to get a handle scoped to a named source (e.g. "dogstatsd", "otlp", "trace-agent").

**Handle** is the lightweight interface passed to data pipelines. It accepts five signal types:

- `ObserveMetric(MetricView)` — a DogStatsD metric sample
- `ObserveLog(LogView)` — a log message
- `ObserveTrace(TraceView)` — a trace (collection of spans)
- `ObserveTraceStats(TraceStatsView)` — aggregated APM stats (decomposed into metrics at the handle layer)
- `ObserveProfile(ProfileView)` — a profiling sample

Each method copies the data synchronously, then does a **non-blocking send** to a buffered channel (capacity 1000). If the channel is full, the observation is silently dropped. This ensures the observer never blocks data ingestion, even if analysis is slow.

> **Note:** Trace and profile observation are wired up but analysis is not yet implemented — they are recorded but not yet used for detection. A fetcher periodically pulls traces and profiles from remote trace-agents via gRPC.

**View interfaces** (`MetricView`, `LogView`, `TraceView`, etc.) provide read-only access to prevent data races. The underlying data may be reused after the call returns, so handles copy everything before sending.

### 2. Store

**Storage** accumulates metrics into a sketch-like (but simplified) summary representation. Each metric series is bucketed into **one-second intervals**, and each bucket records four values: sum, count, min, and max. Multiple samples arriving within the same second are merged into a single bucket. This is lossy — we discard individual sample values — but it's compact and gives us five useful aggregation options at read time: **average** (sum/count), **sum**, **count**, **min**, and **max**.

Aggregation is chosen at **read time**, not write time. Storage always keeps the full summary, so detectors and queries can pick whichever aggregation is appropriate without needing to re-ingest data.

The observer is currently **metric-type agnostic** — it does not distinguish between Count, Gauge, Rate, etc. All metrics are treated uniformly as samples to be summarized. This is a bet on a simpler unified model; we'll see over time whether there's a good reason to introduce type-specific logic.

Metrics from logs are stored with a `_virtual.` prefix (e.g. `_virtual.log.field.duration`) to distinguish them from directly observed metrics.

### 3. Detect

Detection is **not** scheduled on a timer. It's triggered by data arrival: when data at time T arrives, detectors run on data up to T-1. This "complete seconds" rule ensures deterministic results whether data arrives in batch or streaming.

**Why determinism matters:** Because detection is driven entirely by data timestamps (not wall-clock time), replaying the same data always produces the same anomalies. This makes it cheap to simulate the full detection pipeline offline, which is key to iterating quickly on algorithms. The testbench leverages this — it loads a parquet file of recorded metrics, runs them through the same pipeline that runs live, and produces identical results every time. This gives us fast, repeatable iteration loops: change a detector, re-run the scenario, compare results.

There are two detector interfaces, from simple to flexible:

**SeriesDetector** analyzes a single time series:
```
Detect(Series) → DetectionResult{Anomalies[]}
```
Called once per series per detection cycle. Currently receives the **full accumulated history** for that series — all points from the beginning up to the current data time. We will need to develop a windowing strategy so this doesn't grow unboundedly; that's an open question. Each point is one second (matching the storage buckets), with timestamps in Unix seconds. There may be gaps where no data arrived — we currently do no fill (zero-fill, last-value hold, etc.). This could become important for metrics like monotonic counts that are reported at wider intervals than our one-second bucket size. Implementations should be stateless — just do math on the points you're given. Examples: CUSUM (change-point detection), BOCPD (Bayesian changepoint detection).

**Detector** pulls whatever it needs from storage:
```
Detect(StorageReader, dataTime int64) → DetectionResult{Anomalies[]}
```
`dataTime` is the current detection time in Unix seconds. The detector can query storage for any series and any time range it needs. Supports multivariate detection across multiple series. Example: RRCF (Robust Random Cut Forest) reads several metrics, aligns them by timestamp, and scores the combined signal.

**How they connect:** SeriesDetector implementations are automatically wrapped into Detectors via `seriesDetectorAdapter`. The adapter iterates all series in storage × a set of aggregations (avg, count by default), runs the SeriesDetector on each, and appends an aggregation suffix (`:avg`, `:count`) to the metric name. If you need per-series detection, implement SeriesDetector. If you need cross-series detection, implement Detector directly.

**LogObserver** is an optional interface that Detectors can implement to receive raw log observations directly, without going through the metrics extraction path:
```go
type LogObserver interface {
    ProcessLog(log LogView)
}
```

### 4. Correlate

**Correlator** accumulates anomaly events and looks for patterns:
```
Process(anomaly Anomaly)
Flush() → []ReportOutput
```
Correlators are stateful. They receive individual anomalies, accumulate them over a time window, and produce reports when patterns are detected. Example: `TimeClusterCorrelator` groups anomalies that occur close together in time into clusters, surfacing correlated bursts of anomalous behavior across different series.

### 5. Report

**Reporter** delivers results to their destination:
```
Report(report ReportOutput)
```
Reporters query state interfaces like `Correlator` and `RawAnomalyState` to access current system state, rather than relying solely on the report argument. Example: `StdoutReporter` queries the correlator's `ActiveCorrelations()` to print changes.

> **Note:** The Reporter interface hasn't seen much focus yet and may need some evolution as we learn more about what reporting needs look like in practice.

## Log Processing

When a log is observed, two things happen:

1. **LogMetricsExtractors** run synchronously, transforming the log into metrics:
   ```
   ProcessLog(log LogView) → []MetricOutput
   ```
   Each returned metric is stored as `_virtual.{name}` and flows through normal detection. Implementations should be stateless and fast.

2. **Detectors implementing LogObserver** receive the raw log directly, allowing log-based anomaly detection without the metrics extraction step.

## Framework vs. Component Responsibilities

The observer separates **framework concerns** from **component logic**. If you're writing a detector, extractor, or correlator, you don't need to think about threading, scheduling, or data flow — the framework handles all of that. Your code is just a function that receives data and returns results.

**The framework's job:**
- Copying data from handles and dispatching it to the right place
- Storing metrics and managing time series buckets
- Deciding when to run detectors (data-driven, not timer-based)
- Routing anomalies to correlators and flushing reports to reporters
- All threading and concurrency

**Your job as a component author:**
- Implement a simple interface (e.g. `Detect(Series) → DetectionResult`)
- Do your math or logic on the data you're given
- Return results — the framework takes it from there

Currently the framework runs everything in a single dispatch goroutine, but this is an implementation detail that could evolve (e.g. parallelizing independent detectors). Component implementations shouldn't depend on or worry about the threading model, but they are expected not to block — detectors should be doing local computation on the data points they're given, not making network calls or similar.

### Threading Details (for framework contributors)

Handles are the only concurrent part. They copy data and do a **non-blocking send** to a buffered channel (capacity 1000). If the buffer is full, observations are dropped — analysis should never back-pressure data ingestion. Drops are not yet tracked; we should add telemetry for this so we have visibility into when it happens. The buffer capacity could also become a useful knob for controlling resource usage.

Everything after the channel — storage, extractors, detectors, correlators, reporters — currently runs in a single dispatch goroutine:
```go
for obs := range o.obsCh {
    if obs.metric != nil { processMetric() }
    if obs.log != nil    { processLog() }
    if obs.trace != nil  { processTrace() }
    if obs.profile != nil { processProfile() }
}
```

## Writing Extensions

### New SeriesDetector

Implement `SeriesDetector` for stateless per-series anomaly detection. The adapter will automatically run it across all series and aggregations.

```go
type MyDetector struct{}

func (d *MyDetector) Name() string { return "my_detector" }

func (d *MyDetector) Detect(series observer.Series) observer.DetectionResult {
    // Analyze series.Points (already aggregated to one value per second)
    for _, p := range series.Points {
        if p.Value > 100 {
            return observer.DetectionResult{
                Anomalies: []observer.Anomaly{{
                    Title:     "Spike detected",
                    Timestamp: p.Timestamp,
                }},
            }
        }
    }
    return observer.DetectionResult{}
}
```

Register in `observer.go`:
```go
detectors: []observerdef.Detector{
    newSeriesDetectorAdapter(&MyDetector{}, defaultAggregations),
    // ...
},
```

### New Detector (Multivariate)

Implement `Detector` for cross-series analysis. You pull data from storage directly.

```go
type MyDetector struct{}

func (d *MyDetector) Name() string { return "my_detector" }

func (d *MyDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
    // Find series you care about
    keys := storage.ListSeries(observer.SeriesFilter{NamePattern: "cpu"})

    // Read their data
    for _, key := range keys {
        series := storage.GetSeriesRange(key, 0, dataTime, observer.AggregateAverage)
        // ... analyze across series
    }
    return observer.DetectionResult{}
}
```

Register in `observer.go`:
```go
detectors: []observerdef.Detector{
    &MyDetector{},
    // ...
},
```

### New LogMetricsExtractor

```go
type MyExtractor struct{}

func (m *MyExtractor) Name() string { return "my_extractor" }

func (m *MyExtractor) ProcessLog(log observer.LogView) []observer.MetricOutput {
    content := string(log.GetContent())
    // Extract what you need synchronously — don't store the view
    return []observer.MetricOutput{{
        Name:  "my.metric",    // stored as _virtual.my.metric
        Value: 1,
        Tags:  log.GetTags(),
    }}
}
```

Register in `observer.go`:
```go
extractors: []observerdef.LogMetricsExtractor{
    &MyExtractor{},
    // ...
},
```

### New Correlator

```go
type MyCorrelator struct {
    events []observer.Anomaly
}

func (c *MyCorrelator) Name() string { return "my_correlator" }

func (c *MyCorrelator) Process(anomaly observer.Anomaly) {
    c.events = append(c.events, anomaly)
}

func (c *MyCorrelator) Flush() []observer.ReportOutput {
    var reports []observer.ReportOutput
    for _, e := range c.events {
        reports = append(reports, observer.ReportOutput{Title: e.Title})
    }
    c.events = nil
    return reports
}
```

Register in `observer.go`:
```go
correlators: []observerdef.Correlator{
    &MyCorrelator{},
    // ...
},
```

## Configuration

```yaml
observer:
  # Main switches
  analysis:
    enabled: true                    # Enable the anomaly detection pipeline

  # Agent internal log capture
  capture_agent_internal_logs:
    enabled: true
    sample_rate_info: 0.1            # Sample 10% of info logs
    sample_rate_debug: 0.0           # Drop debug logs
    sample_rate_trace: 0.0           # Drop trace logs

  # Parquet metric recording
  capture_metrics:
    enabled: true
  parquet_output_dir: "/var/log/datadog/observer-metrics"
  parquet_flush_interval: 60s        # File rotation interval
  parquet_retention: 24h             # Automatic cleanup (0 = disabled)

  # Debug
  debug_dump_path: "/tmp/observer-dump.json"
  debug_dump_interval: 30s
```

## Metric Recording to Parquet Files

The observer can record all observed metrics to Parquet files for long-term storage and analysis.

Files are rotated at the flush interval with UTC-timestamped names:

```
/var/log/datadog/observer-metrics/
├── observer-metrics-20260129-133045Z.parquet
├── observer-metrics-20260129-133145Z.parquet
└── observer-metrics-20260129-133245Z.parquet
```

- **Compression**: Zstd (typically 4-5x compression ratio)
- **Rotation**: New file created every flush interval
- **Validity**: Each file is properly closed and immediately readable

### Parquet Schema

Schema is compatible with **FGM (Flare Graph Metrics)** format:

| Column Name  | Type           | Description                                      |
|--------------|----------------|--------------------------------------------------|
| `RunID`      | `string`       | Metric source/namespace (e.g., "all-metrics")    |
| `Time`       | `int64`        | Timestamp in milliseconds since Unix epoch       |
| `MetricName` | `string`       | Full metric name (e.g., "system.cpu.idle")       |
| `ValueFloat` | `float64`      | Metric value                                     |
| `Tags`       | `list<string>` | Array of tags in "key:value" format              |

- Time is in **milliseconds** (divide by 1000 for seconds)
- Tags format: `["host:server1", "env:prod"]`
- All timestamps are UTC
- Bloom filters enabled on Tags and MetricName for fast queries
