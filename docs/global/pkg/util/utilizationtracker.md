> **TL;DR:** Measures the fraction of time a component is busy (0.0–1.0) using EWMA smoothing over fixed intervals, designed for check runner workers that wrap each execution with `Started()`/`Finished()` calls.

# pkg/util/utilizationtracker

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/utilizationtracker`

## Purpose

`pkg/util/utilizationtracker` measures the fraction of time a component is busy doing work, expressed as a value between 0.0 and 1.0. It is designed for components that have a clear start/stop boundary around their "working" period (e.g. a check runner worker executing a check). The utilization is computed per fixed time interval and then smoothed with an exponential weighted moving average (EWMA) to avoid spiky output.

The implementation runs a single background goroutine that serializes all events (start, stop, tick), which keeps the tracker free of locks and safe to use from a single producer goroutine.

## Key Elements

### `UtilizationTracker`

The central type. Created with `NewUtilizationTracker`.

**Fields accessible by callers:**
- `Output chan float64` — a buffered channel (capacity 1) that receives the latest utilization value after every event. The caller must drain this channel or the tracker will block.

**Constructor:**
```go
func NewUtilizationTracker(interval time.Duration, alpha float64) *UtilizationTracker
```
- `interval` — the sampling window over which busy time is accumulated before computing a utilization fraction.
- `alpha` — EWMA smoothing factor (0 < alpha <= 1). Smaller values smooth more aggressively; `alpha = 0.25` converges to 99.98 % of a constant signal in ~30 iterations.

### Methods

| Method | When to call |
|---|---|
| `Started()` | Immediately before the component begins a unit of work |
| `Finished()` | Immediately after the component finishes a unit of work |
| `Tick()` | Periodically (typically by an external ticker at the same cadence as `interval`) to advance time when no work is happening |
| `Stop()` | When the component is shutting down; closes `Output` and terminates the background goroutine |

Each call produces exactly one value on `Output`.

### Internal algorithm

For each elapsed `interval`, the tracker computes:

```
raw = busy_time / interval
value = value * (1 - alpha) + raw * alpha
```

If a work period spans an interval boundary, the busy time is split correctly across the two intervals.

## Usage

### Check runner worker (`pkg/collector/worker/worker.go`)

The only production caller. Each `Worker` creates one tracker per `Run()` invocation and wraps every check execution with `Started()`/`Finished()`:

```go
utilizationTracker := utilizationtracker.NewUtilizationTracker(pollingInterval, 0.25)
defer utilizationTracker.Stop()

// A background goroutine reads Output and publishes to expvars and telemetry.
startUtilizationUpdater(workerName, utilizationTracker)
// A ticker goroutine calls utilizationTracker.Tick() at pollingInterval.
cancel := startTrackerTicker(utilizationTracker, pollingInterval)
defer cancel()

for check := range pendingChecksChan {
    utilizationTracker.Started()
    check.Run()
    utilizationTracker.Finished()
}
```

The resulting utilization value is published both to the `expvar` endpoint and as the `collector.worker_utilization` telemetry gauge, tagged with `worker_name`.

## Contrast with related packages

`pkg/util/utilizationtracker` and `pkg/util/statstracker` both produce smoothed time-series statistics from a stream of events, but serve different purposes:

| Package | Measurement | Output | Consumer |
|---------|-------------|--------|----------|
| `utilizationtracker` | Fraction of time busy (0–1), EWMA smoothed | `chan float64` (one value per event) | Check runner workers → `collector.worker_utilization` gauge |
| `statstracker` | Rolling-window average and peak of `int64` samples | Methods (`MovingAvg`, `MovingPeak`) | Log pipeline latency → agent status page |

Use `utilizationtracker` when you need a real-time busy/idle ratio; use `statstracker` when you need windowed averages and peaks over a bounded history.

## Cross-references

| Topic | See also |
|-------|----------|
| Check runner worker — the only production caller; creates one tracker per `Run()` invocation and wraps every check execution | [pkg/collector/worker](../collector/worker.md) |
| `collector.worker_utilization` telemetry gauge published by the worker using this tracker | [pkg/telemetry](../telemetry.md) |
| `pkg/util/statstracker` — a related rolling-window stats package for pipeline latency | [pkg/util/statstracker](statstracker.md) |
