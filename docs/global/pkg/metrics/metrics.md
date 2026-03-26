# pkg/metrics

## Purpose

`pkg/metrics` defines the core metric data model used throughout the Datadog Agent. It is the
shared vocabulary between metric producers (checks, DogStatsD, the Python loader) and metric
consumers (the aggregator, the serializer, the forwarder).

The package covers three distinct signal types:

- **Metrics** — numeric time-series. Includes all aggregation types the agent supports:
  gauge, count, rate, monotonic count, counter, histogram, historate, set, distribution, and
  gauge/count with explicit timestamps.
- **Events** (`pkg/metrics/event`) — discrete moments in time forwarded directly to the Datadog
  event stream without aggregation.
- **Service checks** (`pkg/metrics/servicecheck`) — pass/fail health checks forwarded directly to
  Datadog without aggregation.

`pkg/metrics` lives in its own Go module (`pkg/metrics/go.mod`) so it can be imported by
lower-level packages (e.g. the Python check loader) without pulling in the full agent dependency
graph.

## Key elements

### MetricType and MetricSample

`MetricType` (an `int` constant set) enumerates every aggregation type the agent understands:

| Constant | String | Description |
|---|---|---|
| `GaugeType` | `"Gauge"` | Last value in the flush window. |
| `RateType` | `"Rate"` | Per-second rate between two consecutive flushes. |
| `CountType` | `"Count"` | Sum of all samples in the flush window. |
| `MonotonicCountType` | `"MonotonicCount"` | Delta between the current and previous raw counter value; resets are ignored. |
| `CounterType` | `"Counter"` | DogStatsD-only; behaves like a rate (count / interval). |
| `HistogramType` | `"Histogram"` | Distribution across configurable percentiles / aggregations. |
| `HistorateType` | `"Historate"` | Rate-based histogram (combines histogram and rate semantics). |
| `SetType` | `"Set"` | Count of unique values seen in the flush window. |
| `DistributionType` | `"Distribution"` | Global distribution (DDSketch-based percentile accuracy). |
| `GaugeWithTimestampType` | `"GaugeWithTimestamp"` | Gauge with an explicit, caller-supplied timestamp. |
| `CountWithTimestampType` | `"CountWithTimestamp"` | Count with an explicit, caller-supplied timestamp. |

`MetricSample` is the raw sample struct that flows from listeners (DogStatsD, checks) into the
aggregator:

```go
type MetricSample struct {
    Name            string
    Value           float64
    RawValue        string       // used by Set
    Mtype           MetricType
    Tags            []string
    Host            string
    SampleRate      float64
    Timestamp       float64      // seconds since epoch; 0 means "now"
    FlushFirstValue bool
    OriginInfo      taggertypes.OriginInfo
    ListenerID      string
    NoIndex         bool
    Source          MetricSource
}
```

`MetricSample` implements the `MetricSampleContext` interface, which the aggregator uses to extract
identity information (name, host, tags, source) without knowing the concrete type.

### MetricSampleContext interface

```go
type MetricSampleContext interface {
    GetName() string
    GetHost() string
    GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, tagger tagger.Component)
    GetMetricType() MetricType
    IsNoIndex() bool
    GetSource() MetricSource
}
```

Both `MetricSample` and `HistogramBucket` implement this interface, allowing the aggregator's
context resolver to handle them uniformly.

### Metric interface (internal)

```go
type Metric interface {
    addSample(sample *MetricSample, timestamp float64)
    flush(timestamp float64) ([]*Serie, error)
    isStateful() bool
}
```

Each concrete aggregation type (`Gauge`, `Rate`, `Count`, `MonotonicCount`, `Counter`,
`Histogram`, `Historate`, `Set`, `MetricWithTimestamp`) implements this internal interface.
`isStateful()` returns `true` for types that must remember their previous value across flushes
(e.g. `Rate`, `MonotonicCount`).

### ContextMetrics and CheckMetrics

`ContextMetrics` is a `map[ckey.ContextKey]Metric` that holds all active metrics keyed by their
context. It exposes:

- `AddSample(contextKey, sample, timestamp, interval, telemetry, config)` — creates a new
  `Metric` of the appropriate type on first sight of a context key, then feeds the sample to it.
- `Flush(timestamp)` — iterates all metrics, calls `flush()` on each, and returns the resulting
  `[]*Serie` plus a map of per-context errors.

`CheckMetrics` wraps `ContextMetrics` and adds expiration logic for the check sampler:

- Stateless metrics (e.g. `Gauge`) are removed immediately on `Expire()`.
- Stateful metrics (e.g. `Rate`) are kept for a configurable `statefulTimeout` after `Expire()`
  to handle checks that emit metrics intermittently, and are purged by `RemoveExpired()`.

### Serie and output types

`Serie` is the wire-ready timeseries struct serialised to the Datadog metrics API:

```go
type Serie struct {
    Name       string
    Points     []Point          // []{ Ts float64, Value float64 }
    Tags       tagset.CompositeTags
    Host       string
    MType      APIMetricType    // gauge | rate | count
    Interval   int64
    // v2-only fields (not JSON-serialised by default):
    NoIndex    bool
    Resources  []Resource
    Source     MetricSource
}
```

`APIMetricType` collapses the many internal `MetricType` values into the three types the API
accepts: `APIGaugeType`, `APIRateType`, `APICountType`. `SeriesAPIV2Enum()` converts to the
integer enum in the agent-payload protobuf.

`SketchSeries` / `SketchSeriesList` carry DDSketch-based distribution data (used for
`DistributionType`). The `SketchData` interface is satisfied by `*quantile.Sketch` and drives the
serialiser.

### Iterable sinks

For memory-efficient serialisation, the package provides sink/source pairs:

| Sink interface | Source interface | Backing type |
|---|---|---|
| `SerieSink` (`Append(*Serie)`) | `SerieSource` (`MoveNext`, `Current`, `Count`) | `IterableSeries` |
| `SketchesSink` (`Append(*SketchSeries)`) | — | `SketchSeriesList` |

`IterableSeries` (and its sibling `IterableSketches`) let the serialiser stream series to the
forwarder without buffering all of them in memory at once.

### MetricSamplePool

```go
type MetricSamplePool struct { ... }
func NewMetricSamplePool(batchSize int, isTelemetryEnabled bool) *MetricSamplePool
func (m *MetricSamplePool) GetBatch() MetricSampleBatch
func (m *MetricSamplePool) PutBatch(batch MetricSampleBatch)
```

A `sync.Pool`-backed pool of `MetricSampleBatch` slices. DogStatsD uses this to reuse batch
allocations across the high-throughput UDP/UDS receive path.

### MetricSource

`MetricSource` (`uint16`) is a large enum that records how a metric entered the agent (e.g.
`MetricSourceDogstatsd`, `MetricSourceKubernetesStateCore`, `MetricSourceInternal`, per-integration
constants). It is attached to every `MetricSample` and propagated to `Serie` / `SketchSeries` for
use by the v2 API serialiser to populate origin metadata.

### HistogramBucket

```go
type HistogramBucket struct {
    Name, Host  string
    Value       int64
    LowerBound, UpperBound float64
    Monotonic   bool
    Tags        []string
    Timestamp   float64
    Source      MetricSource
}
```

Represents a single Prometheus/OpenMetrics histogram bucket coming from a check. Implements
`MetricSampleContext` so the aggregator can handle it through the same path as `MetricSample`.

### Sub-packages

#### `pkg/metrics/event`

Defines `Event` (a single Datadog event), `Priority` (`"normal"` / `"low"`), `AlertType`
(`"error"` / `"warning"` / `"info"` / `"success"`), and `Events` (a `[]*Event` slice). Events are
forwarded without aggregation. JSON tags map directly to the Datadog v1 event intake format.

#### `pkg/metrics/servicecheck`

Defines `ServiceCheck`, `ServiceCheckStatus` (OK=0, Warning=1, Critical=2, Unknown=3), and
`ServiceChecks`. Service checks are also forwarded without aggregation.

## Usage

### Aggregator (`pkg/aggregator`)

The aggregator is the primary consumer. It resolves each `MetricSample` to a context key via
`pkg/aggregator/ckey`, then calls `ContextMetrics.AddSample()` or `CheckMetrics.AddSample()`.
At each flush tick it calls `ContextMetrics.Flush()` / `CheckMetrics.Flush()`, and pipes the
resulting `[]*Serie` and `SketchSeriesList` to the serialiser. The `Context` struct in
`pkg/aggregator` embeds `metrics.MetricType` and `metrics.MetricSource` to store per-context
metadata.

### Python check loader (`pkg/collector/python`)

The Python loader maps the check SDK metric submission types to `MetricType` constants and
constructs `MetricSample` values that are handed to the aggregator's sender interface.

### Core checks (`pkg/collector/corechecks/`)

Go-based checks (GPU, Jetson, Kubernetes state, etc.) create `MetricSample` and `HistogramBucket`
values and submit them via the sender. `MetricSource` constants are set here to identify the
integration that produced the metric.

### Serialiser (`pkg/serializer/internal/metrics`)

The serialiser receives `SerieSink` / `SketchesSink` instances and iterates them via
`SerieSource.MoveNext()` / `Current()` to build compressed API payloads. It uses
`APIMetricType.SeriesAPIV2Enum()` and `MetricSource` to populate the v2 metric payload's
`origin` field. `Serie.PopulateDeviceField()` and `Serie.PopulateResources()` are called here to
extract `device:` and `dd.internal.resource:` tags into dedicated fields before serialisation.

### DogStatsD

DogStatsD creates `MetricSamplePool` at startup and recycles `MetricSampleBatch` slices across
the UDP receive loop to avoid GC pressure. Parsed metrics become `MetricSample` values with
`Source = MetricSourceDogstatsd`.

---

## Cross-references

### How pkg/metrics fits into the wider pipeline

```
DogStatsD / Checks
      │  MetricSample (OriginInfo, MetricType, Tags)
      ▼
pkg/aggregator  ──  ContextMetrics.AddSample()
      │              uses pkg/tagset for tag dedup
      │              uses pkg/aggregator/ckey for context key
      │  Flush() returns []*Serie / SketchSeriesList
      ▼
pkg/serializer  ──  SerieSource / SketchesSource
      │              iterates via IterableSeries / IterableSketches
      ▼
comp/forwarder/defaultforwarder  ──  BytesPayloads
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`pkg/aggregator`](../aggregator/aggregator.md) | Primary consumer. `ContextMetrics` and `CheckMetrics` live in the aggregator; they accumulate `MetricSample` values by context key and produce `[]*Serie` or `SketchSeriesList` at each flush. `MetricSampleContext` is the interface the aggregator uses to extract identity without depending on the concrete sample type. |
| [`pkg/aggregator/sender`](../aggregator/sender.md) | Checks submit metrics via `Sender.Gauge`, `Sender.Count`, etc. Each call creates a `MetricSample` with the appropriate `MetricType` and enqueues it to the aggregator. `HistogramBucket` (from OpenMetrics checks) is also routed through the same sender path. |
| [`pkg/tagset`](../tagset.md) | `Serie.Tags` and `SketchSeries.Tags` are `tagset.CompositeTags`. The aggregator builds these by merging a `HashingTagsAccumulator` (tagger-sourced tags) with another (metric-level tags), using `HashGenerator.Dedup2` before calling `NewCompositeTags`. |
| [`pkg/serializer`](../serializer.md) | Receives `SerieSource` and `SketchesSource` from the aggregator and serialises them using `IterableSeries` / `SketchSeriesList`. Uses `APIMetricType.SeriesAPIV2Enum()` and `MetricSource` to populate origin metadata in the v2/v3 payload. Calls `Serie.PopulateDeviceField()` and `Serie.PopulateResources()` before encoding. |

### Tag representation journey

A tag string starts as a plain `[]string` in `MetricSample.Tags`, is appended into a `tagset.HashingTagsAccumulator` alongside tagger-sourced tags, deduplicated, and stored as `tagset.CompositeTags` inside `Serie.Tags`. This avoids an extra allocation on the serialisation hot path — the serializer iterates tags via `CompositeTags.ForEach` without ever building a merged flat slice.

### MetricSample lifetime

| Stage | Owner | What happens |
|-------|-------|--------------|
| Creation | DogStatsD listener / check | `MetricSample` allocated from `MetricSamplePool.GetBatch()` |
| Enqueue | DogStatsD: `AggregateSamples(shard, batch)` | Batch sent to a `TimeSampler` shard |
| Aggregation | `TimeSampler` / `CheckSampler` | `ContextMetrics.AddSample()` routes by context key |
| Flush | `BufferedAggregator` flush tick | `ContextMetrics.Flush()` returns `[]*Serie` |
| Serialise | `pkg/serializer` | `IterableSeries.WriteCurrentItem()` encodes to JSON/protobuf |
| Forward | `comp/forwarder/defaultforwarder` | `BytesPayloads` sent over HTTPS to intake |
| Pool return | DogStatsD after `AggregateSamples` | Batch returned via `MetricSamplePool.PutBatch()` |
