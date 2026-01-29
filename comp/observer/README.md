# Observer Component

Observes data flowing through the agent for sampling and analysis.

## Components

**Component** is the entry point. Call `GetHandle(name)` to get a handle scoped to a named source (e.g. "dogstatsd", "otlp").

**Handle** is the lightweight interface passed to data pipelines. It has two methods:

- `ObserveMetric(MetricView)` — adds the metric to storage, then runs time series analyses on the updated series.
- `ObserveLog(LogView)` — runs all log analyses. Any metrics they produce are added to storage and trigger time series analysis.

**MetricView / LogView** are read-only interfaces to avoid data races. The underlying data may be reused after the call returns, so copy any values you need synchronously.

**Storage** accumulates metrics into per-second buckets, tracking sum/count/min/max for each series. When retrieved, you specify an aggregation (average, sum, count, min, max) to collapse each bucket to a single value.

**LogAnalysis** transforms logs into metrics and anomaly events:
```
Analyze(LogView) → LogAnalysisResult{Metrics[], Anomalies[]}
```
Implementations should be stateless and fast since they run synchronously on every log.

**TimeSeriesAnalysis** detects anomalies from accumulated time series:
```
Analyze(Series) → TimeSeriesAnalysisResult{Anomalies[]}
```
Receives a series of aggregated points. Implementations should be stateless and just do math on the points.

**AnomalyConsumer** receives and accumulates anomaly events from all analyses:
```
Consume(AnomalyOutput)
Report()
```
Unlike analyses, consumers are stateful. They accumulate events and process them when `Report()` is called.

## Threading Model

Handles are the only concurrent part. They copy data and send it over a channel. Everything else (storage, analyses, consumers) runs in a single dispatch goroutine, so no locks are needed in component implementations.

## Writing a New Analysis

Implement `LogAnalysis` or `TimeSeriesAnalysis`:

```go
type MyLogAnalysis struct{}

func (m *MyLogAnalysis) Name() string { return "my_analysis" }

func (m *MyLogAnalysis) Analyze(log observer.LogView) observer.LogAnalysisResult {
    // Extract what you need synchronously - don't store the view
    content := string(log.GetContent())

    // Return metrics and/or anomalies
    return observer.LogAnalysisResult{
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
    logProcessors: []observerdef.LogProcessor{
        &LogTimeSeriesAnalysis{},
        &MyLogProcessor{},  // add here
    },
    // ...
}
```

## Writing a New Consumer

Implement `AnomalyConsumer`:

```go
type MyConsumer struct {
    events []observer.AnomalyOutput
}

func (c *MyConsumer) Name() string { return "my_consumer" }

func (c *MyConsumer) Consume(anomaly observer.AnomalyOutput) {
    c.events = append(c.events, anomaly)
}

func (c *MyConsumer) Report() {
    // Do something with accumulated events
    for _, e := range c.events {
        fmt.Printf("Anomaly: %s\n", e.Title)
    }
    c.events = nil
}
```

Register in `observer.go`:

```go
obs := &observerImpl{
    consumers: []observerdef.AnomalyConsumer{
        &MemoryConsumer{},
        &MyConsumer{},  // add here
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
  capture_metrics: true
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
