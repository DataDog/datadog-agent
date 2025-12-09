# Metric History Cache - Prototype Spec

**Status:** In Progress
**Goal:** Prototype to explore local metric history for anomaly detection
**Branch:** `metric-cache-prototype`

## Overview

A local cache of metric history that enables running anomaly detection algorithms (e.g., changepoint detection) against recent metric data within the agent. This is a **prototype** - the goal is to make progress quickly and understand what's possible, not to engineer a final architecture.

## Goals

1. Observe the flow of metrics without affecting normal pipeline
2. Store configurable history with time-based rollup (recent data at high resolution, older data at lower resolution)
3. Provide a clean interface for plugging in anomaly detection algorithms
4. Enable future tag-stripping re-aggregation via sketch-based storage

## Non-Goals (for prototype)

- Production-ready performance tuning
- Persistence across agent restarts
- Concurrent/sharded operation (single worker for simplicity)
- Optimized memory footprint (will evaluate and improve later)

## Architecture

### Tap Point: Serializer Sink Wrapper

Intercept metrics at the serializer pipeline by wrapping the `SerieSink` in `flushToSerializer()`:

```
TimeSampler ──┐
              ├──► SerieSink ──► ObservingSink ──► Serializer
CheckSampler ─┘                       │
                                      ▼
                               MetricHistoryCache
```

**Location:** `pkg/aggregator/demultiplexer_agent.go:flushToSerializer()`

**Implementation:**
```go
type observingSink struct {
    delegate metrics.SerieSink
    cache    *MetricHistoryCache
}

func (s *observingSink) Append(serie *metrics.Serie) {
    s.cache.Observe(serie)  // must be fast
    s.delegate.Append(serie)
}
```

**Filtering:** The cache checks metric names against a configurable include list (prefix matching). Metrics not matching any prefix are ignored. Filtering happens inside `Observe()`.

**Concurrency:** For the prototype, we assume single-threaded flush (no concurrent `Append()` calls). This matches the current sequential flush model in `flushToSerializer()`. Production implementation would add per-series or sharded locking.

### Storage Model

#### Series Identity

Use `ckey.ContextKey` (128-bit hash) from the existing aggregator infrastructure to identify series:

```go
type SeriesKey struct {
    ContextKey ckey.ContextKey  // 128-bit hash for identity
    Name       string           // stored for debugging/display
    Tags       []string         // stored for debugging/display
}
```

This avoids collision risk from smaller hashes and reuses proven infrastructure.

#### Time Windows (configurable)

| Window | Resolution | Default Retention | Example |
|--------|------------|-------------------|---------|
| Recent | Flush interval (15s) | 5 minutes | 20 points |
| Medium | 1 minute | 1 hour | 60 points |
| Long | 1 hour | 24 hours | 24 points |

Older data rolls up into coarser buckets using sketch-based aggregation.

#### Data Structure

```go
type MetricHistory struct {
    Key     SeriesKey
    Type    metrics.APIMetricType  // gauge, count, rate

    Recent  RingBuffer[DataPoint]  // flush-resolution stats
    Medium  RingBuffer[DataPoint]  // 1-minute rollups
    Long    RingBuffer[DataPoint]  // 1-hour rollups
}

type DataPoint struct {
    Timestamp int64
    Stats     SummaryStats
}

// SummaryStats captures distribution summary without full sketch overhead.
// Designed for easy replacement with *quantile.Sketch later if percentiles needed.
type SummaryStats struct {
    Count int64
    Sum   float64
    Min   float64
    Max   float64
}

func (s *SummaryStats) Mean() float64 { return s.Sum / float64(s.Count) }

func (s *SummaryStats) Merge(other SummaryStats) {
    s.Count += other.Count
    s.Sum += other.Sum
    if other.Min < s.Min { s.Min = other.Min }
    if other.Max > s.Max { s.Max = other.Max }
}
```

**Why simple stats first:** ~40 bytes per point vs ~100+ bytes for full sketch. Mean/min/max is sufficient for most anomaly detection (changepoint, spike, drop). Uniform interface (`Mean()`, `Min`, `Max`) makes it easy to swap in `pkg/quantile.Sketch` later if we need percentiles for more sophisticated detection.

#### Metric Type Rollup Semantics

By the time Series reach the serializer, all values are **per-interval** (not cumulative):

| Type | Rollup Strategy | Rationale |
|------|-----------------|-----------|
| **Gauge** (`APIGaugeType`) | `SummaryStats.Merge()` | Captures min/max/mean of point-in-time values |
| **Count** (`APICountType`) | Sum counts, merge stats | Total events = sum of interval counts |
| **Rate** (`APIRateType`) | `SummaryStats.Merge()` | Rate over longer window via merged stats |

All types use `SummaryStats.Merge()` which correctly combines count/sum/min/max. The merged mean is the weighted average of the original means.

### Rollup Process

Triggered by flush cycle (not wall-clock), runs after observation completes:

1. Scan `Recent` buffer for points older than retention
2. Group by target bucket timestamp (1-minute boundary)
3. Merge stats using `SummaryStats.Merge()` into new `DataPoint`
4. Append to `Medium` buffer
5. Similarly roll `Medium` → `Long` on hourly boundaries

### Expiration

Remove `MetricHistory` entries when no data observed for a configurable number of flush cycles (default: 100 cycles ≈ 25 minutes). This prevents unbounded growth from metric churn.

### Query Interface

```go
type HistoryReader interface {
    // List all tracked series
    ListSeries() []SeriesKey

    // Get data points at each resolution tier
    GetRecent(key SeriesKey) []DataPoint
    GetMedium(key SeriesKey) []DataPoint
    GetLong(key SeriesKey) []DataPoint

    // Convenience: extract scalar time series from stats
    // aspect is one of: "mean", "min", "max", "count", "sum"
    // (add "p50", "p99" later if we swap in full sketches)
    GetScalarSeries(key SeriesKey, tier Tier, aspect string) []TimestampedValue

    // Scan all series for batch processing
    Scan(fn func(SeriesKey, *MetricHistory) bool)
}

type TimestampedValue struct {
    Timestamp int64
    Value     float64
}
```

### Anomaly Detection Interface

Clean interface for plugging in detection algorithms:

```go
// Detector analyzes metric history and reports anomalies.
// Implementations should be stateless between calls (state stored in Anomaly if needed).
type Detector interface {
    // Name returns a unique identifier for this detector
    Name() string

    // Analyze examines a single series and returns any detected anomalies.
    // Called for each series during the detection scan.
    Analyze(key SeriesKey, history *MetricHistory) []Anomaly
}

type Anomaly struct {
    SeriesKey   SeriesKey
    DetectorName string
    Timestamp   int64       // when the anomaly occurred
    Type        string      // e.g., "changepoint", "spike", "drop"
    Severity    float64     // 0.0-1.0, detector-specific
    Message     string      // human-readable description
}

// DetectorRegistry manages multiple detectors
type DetectorRegistry struct {
    detectors []Detector
}

func (r *DetectorRegistry) Register(d Detector)
func (r *DetectorRegistry) RunAll(reader HistoryReader) []Anomaly
```

#### Initial Detector: Simple Changepoint (PELT-lite)

Start with a simple changepoint detector based on mean shift:

```go
type MeanChangeDetector struct {
    // Minimum change in mean (as multiple of stddev) to trigger
    Threshold float64  // default: 2.0

    // Minimum points before and after change
    MinSegmentSize int  // default: 5
}

func (d *MeanChangeDetector) Analyze(key SeriesKey, history *MetricHistory) []Anomaly {
    values := extractMeans(history.Recent)  // or Medium for longer-term detection

    // Simple approach: sliding window comparing recent mean to historical mean
    // More sophisticated: PELT algorithm or Bayesian changepoint detection

    // Return anomaly if significant mean shift detected
}
```

This is intentionally simple. The interface allows swapping in more sophisticated algorithms (PELT, BOCPD, etc.) without changing the infrastructure.

## Configuration

```yaml
metric_history:
  enabled: true
  include_metrics:    # prefix matching (not glob)
    - "system."
    - "docker.cpu."
  retention:
    recent_duration: 5m
    medium_duration: 1h
    long_duration: 24h
  rollup:
    medium_resolution: 1m
    long_resolution: 1h
  expiry_flush_cycles: 100  # remove series after this many flushes without data

  anomaly_detection:
    enabled: true
    run_every_flush_cycles: 4  # run detection every N flushes (e.g., every minute)
    detectors:
      mean_change:
        enabled: true
        threshold: 2.0
        min_segment_size: 5
```

## Implementation Phases

### Phase 1: Tap and Store
- [ ] Implement `observingSink` wrapper in `flushToSerializer()`
- [ ] Basic `MetricHistoryCache` with `Recent` buffer only
- [ ] Metric name filtering (prefix matching)
- [ ] Series identification using `ckey.ContextKey`
- [ ] Configuration loading
- [ ] Basic telemetry (series count, observation rate)

### Phase 2: Rollup and Expiration
- [ ] Add `Medium` and `Long` buffers
- [ ] Rollup logic triggered by flush (with metric type semantics)
- [ ] Series expiration after inactivity
- [ ] `HistoryReader` implementation with `Scan()`

### Phase 3: Anomaly Detection
- [ ] `Detector` interface and `DetectorRegistry`
- [ ] `MeanChangeDetector` implementation
- [ ] Detection scheduling (every N flushes)
- [ ] Anomaly output (logs initially, internal metrics later)

## Future: Upgrading to Full Sketches

If we later need percentiles (p50, p99) for anomaly detection or tag-stripping re-aggregation:

1. Replace `SummaryStats` with `*quantile.Sketch` from `pkg/quantile`
2. Update `Merge()` to use `sketch.Merge()`
3. Add "p50", "p99" options to `GetScalarSeries()`
4. Storage increases from ~40 bytes to ~100-500 bytes per point

The `DataPoint` wrapper and `HistoryReader` interface remain unchanged.

## Files to Modify/Create

**New files:**
- `pkg/aggregator/metric_history/cache.go` - core storage
- `pkg/aggregator/metric_history/observer.go` - sink wrapper
- `pkg/aggregator/metric_history/rollup.go` - rollup logic
- `pkg/aggregator/metric_history/detector.go` - detection interface
- `pkg/aggregator/metric_history/detectors/mean_change.go` - initial detector

**Modified files:**
- `pkg/aggregator/demultiplexer_agent.go` - wire up observer in `flushToSerializer()`
- `pkg/config/setup/config.go` - add configuration keys

## Success Criteria (Prototype)

1. Can observe metrics flowing through the agent without affecting throughput
2. Can query historical values for a specific metric series
3. Can detect an obvious changepoint (e.g., metric doubles) and log it
4. Memory usage is reasonable for ~10K tracked series (measure, don't optimize)

## Prototype Implementation Decisions

These decisions were made to expedite prototype development:

1. **Prefix matching instead of glob patterns** - Simpler to implement, sufficient for prototype. Can upgrade to glob later if needed.
2. **Anomaly logging** - Use WARNING level logs with distinctive prefix (e.g., `[ANOMALY DETECTED]`) to make them easy to spot. Will be replaced with formal telemetry system later.
3. **No build tags** - Feature controlled entirely via runtime config (`metric_history.enabled`).
4. **Unit tests** - Include tests where they clarify behavior or validate correctness. Skip tests for obviously correct or non-critical code.
5. **Working branch** - `metric-cache-prototype`

## Demonstration

To see the prototype in action:

### Quick Start

1. Build the agent:
   ```bash
   invoke agent.build --build-exclude=systemd,python --exclude-rtloader
   ```

2. Run the demo:
   ```bash
   ./docs/design/metric-history-demo.sh
   ```

3. In another terminal, generate a CPU spike:
   ```bash
   ./docs/design/cpu-spike.sh
   ```

4. Watch for `[ANOMALY DETECTED]` in the agent output (~1-2 minutes after the spike)

### What to Expect

After running for ~2 minutes with stable CPU, then introducing a spike, you should see:
```
[ANOMALY DETECTED] mean_change | system.cpu.user: Significant mean increase detected: 5.23 -> 45.67 (3.2 stddev)
```

### Manual Testing

You can also test by:
- Running a build command (`make` in a large project)
- Starting/stopping a memory-intensive process
- Creating I/O load with `dd if=/dev/zero of=/dev/null`

Any significant change in system metrics should trigger detection.
