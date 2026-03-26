# pkg/util/stat

## Purpose

`pkg/util/stat` provides a lightweight, channel-based rate aggregator. It accumulates integer event values arriving from multiple goroutines over one-second windows, emits a `Stat` snapshot each second, and publishes the latest value to an `expvar.Int` for `/vars` scraping. It is used primarily by DogStatsD and the log pipeline to report per-second throughput counters.

---

## Key elements

### Types

**`Stat`** — a single aggregated snapshot:

```go
type Stat struct {
    Val int64     // aggregated value for the window
    Ts  time.Time // timestamp at the start of the window
}
```

**`Stats`** — the aggregator object. Create with `NewStats(sz uint32)` where `sz` is the capacity of the internal incoming-events channel. Fields of note:

| Field | Type | Description |
|---|---|---|
| `Aggregated` | `chan Stat` | Buffered channel (cap 2) that receives one `Stat` per second |

### Functions / Methods

| Symbol | Description |
|---|---|
| `NewStats(sz uint32) (*Stats, error)` | Constructs a `Stats` with an internal `expvar.Int` named `"pktsec"` and channels of size `sz` |
| `(*Stats).StatEvent(v int64)` | Non-blocking send of a value into the incoming channel; drops the value (with a debug log) when the buffer is full |
| `(*Stats).Process()` | Blocking loop (run in its own goroutine): drains incoming events every tick and emits an aggregated `Stat` to `Aggregated` each second |
| `(*Stats).Update(expStat *expvar.Int)` | Blocking loop (run in its own goroutine): reads from `Aggregated` each second and writes the value to an external `expvar.Int` |
| `(*Stats).Stop()` | Closes the `stopped` channel, terminating both `Process` and `Update` loops |

### Lifecycle

A typical setup spawns two goroutines:

```go
s, _ := stat.NewStats(512)
go s.Process()
go s.Update(myExpvarCounter)
// later:
s.Stop()
```

`Stop` is not idempotent in the sense that calling it twice panics (double close of a channel). Each `Stats` should be stopped exactly once.

---

## Usage

**`comp/dogstatsd/server/server.go`** — creates a `Stats` to track packets processed per second. The value is surfaced via `expvar` and read by the agent status page.

**`pkg/logs/sources/source.go`** — uses `Stats` to expose a per-source lines-per-second counter that appears in the `agent status` log section.

In both cases the pattern is: call `StatEvent(1)` on each processed item in the hot path, read aggregated throughput from the `Aggregated` channel or from the `expvar.Int` updated by `Update`.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/dogstatsd/server`](../../comp/dogstatsd/server.md) | `server.go` creates a `Stats` instance (channel size 512) to track packets-per-second throughput. The `expvar.Int` updated by `Update` is surfaced at `/debug/vars` and read by the agent status page. The DogStatsD server additionally exposes Prometheus-style telemetry counters (`dogstatsd_processed`, `dogstatsd_channel_latency`) that operate independently of `pkg/util/stat`. |
| [`pkg/logs`](../logs/logs.md) | `sources/source.go` creates a `Stats` instance per `LogSource` to expose a lines-per-second counter. This counter appears in the `agent status` logs section, alongside the `BytesRead` and `LatencyStats` fields on each `LogSource`. The `Stats.Aggregated` channel is consumed to update the source's status display. |
