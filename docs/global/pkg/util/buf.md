> **TL;DR:** Provides `BufferedChan`, a batching channel that accumulates values into fixed-size slices before sending, reducing synchronisation overhead for high-throughput producer-consumer pipelines.

# pkg/util/buf

## Purpose

`pkg/util/buf` provides `BufferedChan`, a batching channel that amortises the overhead of Go channel operations when large numbers of small values are produced and consumed by separate goroutines. Instead of one channel send and one channel receive per value, `BufferedChan` accumulates values into a slice of size `bufferSize`, then sends the entire slice as a single channel write, reducing synchronisation operations by a factor of `bufferSize`.

This package lives in its own Go module (`pkg/util/buf/go.mod`) and has no dependencies on the rest of the agent.

## Key elements

### Key types

#### `BufferedChan` (`buffered_chan.go`)

```go
type BufferedChan struct { ... }
```

| Function / Method | Description |
|---|---|
| `NewBufferedChan(ctx context.Context, chanSize int, bufferSize int) *BufferedChan` | Creates a `BufferedChan`. `chanSize` is the capacity of the inner `chan []interface{}`. `bufferSize` is the number of values accumulated before a channel write is issued. `ctx` cancels both `Put` and `Get`. |
| `(*BufferedChan).Put(value interface{}) bool` | Appends `value` to the write buffer. Flushes the buffer to the channel when it is full. Returns `false` if the context is cancelled. **Not safe for concurrent callers.** |
| `(*BufferedChan).Close()` | Flushes any remaining buffered values and closes the underlying channel. Must be called by the producer when it is done. |
| `(*BufferedChan).Get() (interface{}, bool)` | Returns the next value. Blocks until a value is available, the channel is closed, or the context is cancelled. Returns `(nil, false)` when exhausted. **Not safe for concurrent callers.** |
| `(*BufferedChan).WaitForValue() bool` | Blocks until a value is available (returns `true`) or until the channel is closed or context is cancelled (returns `false`). Useful for callers that want to pre-check availability before calling `Get`. |

#### Thread-safety contract

- At most one goroutine may call `Put` at a time.
- At most one goroutine may call `Get` / `WaitForValue` at a time.
- `Put` and `Get` may run concurrently in separate goroutines.

Slices are recycled through a `sync.Pool` to reduce allocator pressure. `Get` clears element references after reading to avoid retaining objects longer than necessary.

## Usage

`BufferedChan` is used in the metrics aggregation pipeline where high-throughput metric series are passed between goroutines:

- `pkg/metrics/iterable_metrics.go` — the `iterableMetrics` type (backing `IterableSeries` and `IterableSketches`) wraps a `*buf.BufferedChan` directly. The aggregator appends `*Serie` and `*SketchSeries` values via `Append`, which calls `bc.Put`; the serialiser reads them via `MoveNext` / `Current`, which consume from `bc.Get`. This lets the aggregator's flush goroutine and the serialiser's encode goroutine run concurrently without buffering the entire series list in memory.
- `pkg/aggregator/aggregator.go` and `pkg/aggregator/demultiplexer_agent.go` — configure the `chanSize` and `bufferSize` for `iterableMetrics` via `FlushAndSerializeInParallel` options, which are read from `aggregator_flush_metrics_and_serialize_in_parallel_chan_size` and `aggregator_flush_metrics_and_serialize_in_parallel_buffer_size`.

### Sizing guidance

- `chanSize` (inner channel capacity): controls how many full `bufferSize`-element batches can
  be in flight simultaneously between producer and consumer. A larger value reduces backpressure
  on the producer at the cost of peak memory usage.
- `bufferSize` (batch accumulation size): controls the amortisation factor. A value of 256
  means the channel write rate is 256× lower than the `Put` call rate, which is the dominant
  cost reduction. Increasing this beyond the typical number of series per check run provides
  diminishing returns.

Typical usage:

```go
bc := buf.NewBufferedChan(ctx, 100 /*chanSize*/, 256 /*bufferSize*/)

// Producer goroutine
go func() {
    for _, item := range items {
        if !bc.Put(item) {
            break // context cancelled
        }
    }
    bc.Close()
}()

// Consumer goroutine
for {
    v, ok := bc.Get()
    if !ok {
        break
    }
    process(v)
}
```

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/metrics` | [../metrics/metrics.md](../metrics/metrics.md) | `pkg/metrics/iterable_metrics.go` embeds `*buf.BufferedChan` inside `iterableMetrics`, which backs `IterableSeries` (`SerieSink` / `SerieSource`) and `IterableSketches`. Every `*Serie` and `*SketchSeries` produced by `ContextMetrics.Flush()` flows through a `BufferedChan` on its way to the serialiser. |
| `pkg/aggregator` | [../aggregator/aggregator.md](../aggregator/aggregator.md) | `BufferedAggregator` creates `IterableSeries` (and therefore `BufferedChan`) instances at each flush tick and passes them to `serializer.SendIterableSeries`. The `FlushAndSerializeInParallel` struct configures the `chanSize` and `bufferSize` values used when constructing the `BufferedChan`. |
