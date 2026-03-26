> **TL;DR:** A memory-efficient rolling-window statistics tracker that computes all-time and windowed average and peak values over bucketed time windows, used primarily to surface log pipeline latency on the agent status page.

# pkg/util/statstracker

Import path: `github.com/DataDog/datadog-agent/pkg/util/statstracker`

## Purpose

Provides a memory-efficient, rolling-window statistics tracker that computes both all-time and windowed average and peak values for a stream of `int64` samples. Data is aggregated into fixed-size time buckets so memory usage is bounded regardless of the number of data points: a 24-hour window with 1-hour buckets never holds more than 24 aggregated points in memory.

The primary use in the codebase is measuring log pipeline latency, but the tracker is generic enough for any numeric time series.

## Key elements

### `Tracker`

```go
type Tracker struct { ... } // all fields unexported
```

Thread-safe; all methods acquire an internal `sync.Mutex`.

**Constructors:**

```go
// Production constructor — uses time.Now().UnixNano() as the clock.
func NewTracker(timeFrame time.Duration, bucketSize time.Duration) *Tracker

// Test constructor — injects a custom clock for deterministic tests.
func NewTrackerWithTimeProvider(
    timeFrame time.Duration,
    bucketSize time.Duration,
    timeProvider func() int64,
) *Tracker
```

`timeFrame` is the total rolling window (e.g. 24 hours). `bucketSize` controls aggregation granularity (e.g. 1 hour). At most `timeFrame / bucketSize` buckets are kept in memory at any one time.

**Methods:**

```go
func (s *Tracker) Add(value int64)        // Record a new sample.

func (s *Tracker) AllTimeAvg() int64      // Running average since creation.
func (s *Tracker) AllTimePeak() int64     // Maximum value ever seen.

func (s *Tracker) MovingAvg() int64       // Weighted average over the rolling window.
func (s *Tracker) MovingPeak() int64      // Maximum value within the rolling window.
```

`Add` updates all-time stats eagerly and manages the bucket-based window. Each bucket accumulates a count-weighted average (avg bucket) and the per-bucket peak (peak bucket). When a bucket ages past `bucketSize`, it is promoted to the aggregated list; buckets older than `timeFrame` are dropped.

`MovingAvg` computes a count-weighted average across the current (head) bucket and all unexpired aggregated buckets. `MovingPeak` scans all unexpired buckets for the largest value.

**`InfoKey` / `Info` — status page integration:**

```go
func (s *Tracker) InfoKey() string   // Returns "Pipeline Latency"
func (s *Tracker) Info() []string    // Returns formatted latency strings
```

These satisfy the `status.Provider` interface used by the Agent's status page, formatting values as Go `time.Duration` strings (the tracker stores nanosecond durations in the log pipeline).

### Bucket internals

```
taggedPoint { timeStamp int64; value int64; count int64 }
```

The "head" bucket (`avgPointsHead` / `peakPointsHead`) accumulates live data. When the head ages beyond `bucketSize`, it is appended to the `aggregatedAvgPoints` / `aggregatedPeakPoints` slices and a new head is started on the next `Add`. Expired buckets are sliced off the front of the aggregated lists on every `Add` or `Moving*` call.

## Usage

### Log pipeline latency — the main consumer

`pkg/logs/sources/source.go` creates one tracker per log source to monitor the age of log messages as they move through the pipeline:

```go
LatencyStats: statstracker.NewTracker(time.Hour*24, time.Hour),
```

The 24-hour window with 1-hour buckets means at most 24 aggregated points are kept per source, regardless of log volume. The tracker's `Info()` output is surfaced on the Agent status page under "Pipeline Latency".

### Writing tests with a controlled clock

Use `NewTrackerWithTimeProvider` to inject a deterministic clock:

```go
now := int64(0)
tracker := statstracker.NewTrackerWithTimeProvider(
    24*time.Hour,
    1*time.Hour,
    func() int64 { return now },
)
tracker.Add(100)
now += int64(2 * time.Hour) // advance time by 2 bucket periods
tracker.Add(200)
```

This avoids timing-dependent flakiness in tests that need to exercise bucket rotation and expiry.

## Relationship to `pkg/util/utilizationtracker`

Both packages measure runtime behavior of a component over time, but they serve different purposes:

| | `pkg/util/statstracker` | `pkg/util/utilizationtracker` |
|---|---|---|
| **What is measured** | Arbitrary `int64` samples (e.g. nanosecond latency values) | Fraction of time a worker is busy (0.0–1.0) |
| **Output** | All-time avg/peak + rolling-window avg/peak via method calls | Streaming `float64` values on an `Output` channel |
| **Window** | Bucket-based rolling window; configurable `timeFrame` / `bucketSize` | Fixed sampling `interval` with EWMA smoothing |
| **Concurrency** | Thread-safe via internal `sync.Mutex` | Single-goroutine producer; background goroutine serializes events |
| **Status page** | `InfoKey()` / `Info()` satisfy `status.Provider`; shown as "Pipeline Latency" | Published as `collector.worker_utilization` expvar / telemetry gauge |
| **Primary user** | Log pipeline latency per `LogSource` | Check runner worker utilization per `Worker` |

Use `statstracker.Tracker` when you need a historical record of sample values with windowed statistics. Use `utilizationtracker.UtilizationTracker` when you need a continuous busy-fraction signal from a start/stop pair around a work unit.

## Cross-references

| Document | Relationship |
|---|---|
| [`pkg/logs/sources`](../logs/sources.md) | `LogSource.LatencyStats` holds a `*Tracker` initialized with a 24 h window and 1 h buckets; its `Info()` output appears on the Agent status page under "Pipeline Latency" |
| [`pkg/util/utilizationtracker`](utilizationtracker.md) | Companion tracker measuring worker busy-fraction rather than sample statistics; used by the check runner instead of the logs pipeline |
