# Observer Component

Observes data flowing through the agent for sampling and analysis.

## Components

**Component** is the entry point. Call `GetHandle(name)` to get a handle scoped to a named source (e.g. "dogstatsd", "otlp").

**Handle** is the lightweight interface passed to data pipelines. It has two methods:

- `ObserveMetric(MetricView)` — adds the metric to storage, then runs metrics detection on the updated series.
- `ObserveLog(LogView)` — runs all log detectors. Any metrics they produce are added to storage and trigger metrics detection.

**MetricView / LogView** are read-only interfaces to avoid data races. The underlying data may be reused after the call returns, so copy any values you need synchronously.

**Storage** accumulates metrics into per-second buckets, tracking sum/count/min/max for each series. When retrieved, you specify an aggregation (average, sum, count, min, max) to collapse each bucket to a single value.

**LogDetector** transforms logs into metrics and anomaly events:
```
Process(log LogView) → LogDetectionResult{Metrics[], Anomalies[]}
```
Implementations should be stateless and fast since they run synchronously on every log.

**MetricsDetector** detects anomalies from accumulated time series:
```
Detect(Series) → MetricsDetectionResult{Anomalies[]}
```
Receives a series of aggregated points. Implementations should be stateless and just do math on the points.

**Correlator** receives and accumulates anomaly events from all analyses:
```
Process(anomaly Anomaly)
Flush() []ReportOutput
```
Unlike analyses, correlators are stateful. They accumulate events and produce reports when `Flush()` is called.

## Threading Model

Handles are the only concurrent part. They copy data and send it over a channel. Everything else (storage, analyses, consumers) runs in a single dispatch goroutine, so no locks are needed in component implementations.

## Writing a New Log Detector

Implement `LogDetector` or `MetricsDetector`:

```go
type MyLogDetector struct{}

func (m *MyLogDetector) Name() string { return "my_detector" }

func (m *MyLogDetector) Process(log observer.LogView) observer.LogDetectionResult {
    // Extract what you need synchronously - don't store the view
    content := string(log.GetContent())

    // Return metrics and/or anomalies
    return observer.LogDetectionResult{
        Metrics: []observer.MetricOutput{{
            Name:  "my.metric",
            Value: 1,
            Tags:  log.GetTags(),
        }},
    }
}
```

Register in `observer.go`:

```go
obs := &observerImpl{
    logDetectors: []observerdef.LogDetector{
        &MyLogDetector{},  // add here
    },
    // ...
}
```

## Writing a New Correlator

Implement `Correlator`:

```go
type MyCorrelator struct {
    events []observer.Anomaly
}

func (c *MyCorrelator) Name() string { return "my_correlator" }

func (c *MyCorrelator) Process(anomaly observer.Anomaly) {
    c.events = append(c.events, anomaly)
}

func (c *MyCorrelator) Flush() []observer.ReportOutput {
    // Do something with accumulated events
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
obs := &observerImpl{
    correlators: []observerdef.Correlator{
        &MemoryCorrelator{},
        &MyCorrelator{},  // add here
    },
    // ...
}
```

## Metric Recording to Parquet Files

The observer can record all observed metrics to Parquet files for long-term storage and analysis.

### Configuration

Enable Parquet recording in `datadog.yaml`:

```yaml
observer:
  capture_metrics.enabled: true
  parquet_output_dir: "/var/log/datadog/observer-metrics"
  parquet_flush_interval: 60s  # File rotation interval
  parquet_retention: 24h        # Automatic cleanup (0 = disabled)
```

### File Format

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

**Arrow Schema Definition:**

```go
arrow.NewSchema([]arrow.Field{
    {Name: "RunID", Type: arrow.BinaryTypes.String},
    {Name: "Time", Type: arrow.PrimitiveTypes.Int64},
    {Name: "MetricName", Type: arrow.BinaryTypes.String},
    {Name: "ValueFloat", Type: arrow.PrimitiveTypes.Float64},
    {Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
}, nil)
```

**Important Notes:**
- Time is in **milliseconds** (divide by 1000 for seconds)
- Tags format: `["host:server1", "env:prod"]`
- All timestamps are UTC
- **Bloom filters enabled** on Tags and MetricName for fast queries
