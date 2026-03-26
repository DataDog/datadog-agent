# pkg/util/sync

Import path: `github.com/DataDog/datadog-agent/pkg/util/sync`

## Purpose

Provides type-safe wrappers around Go's `sync.Pool` so that callers get and put correctly typed values without unsafe type assertions scattered throughout the codebase. An optional variant adds Datadog telemetry counters (gets, puts, currently-active items) to a pool with zero changes to call sites.

`sync.Pool` is Go's built-in mechanism for reusing short-lived allocations. Using it correctly requires a `.(T)` type assertion on every `Get`, which is error-prone and hard to search for. `TypedPool` encapsulates that assertion behind a generic interface.

## Key elements

### Interfaces

```go
type PoolGetter[K any] interface { Get() *K }
type PoolReleaser[K any] interface { Put(*K) }
type Pool[K any] interface { PoolGetter[K]; PoolReleaser[K] }
```

`Pool[K]` is the minimal interface accepted by consumers. Using it instead of `*TypedPool[K]` directly allows the telemetry variant to be swapped in transparently.

### `TypedPool[K]` (struct)

A thin, type-safe wrapper over `sync.Pool`.

```go
func NewDefaultTypedPool[K any]() *TypedPool[K]
func NewSlicePool[K any](size, capacity int) *TypedPool[[]K]
func NewTypedPool[K any](f func() *K) *TypedPool[K]

func (t *TypedPool[K]) Get() *K
func (t *TypedPool[K]) Put(x *K)
```

| Constructor | When to use |
|---|---|
| `NewDefaultTypedPool[K]()` | Allocates with `new(K)`. Suitable for structs. |
| `NewSlicePool[K](size, capacity)` | Allocates `make([]K, size, capacity)`. Suitable for reusable byte buffers. |
| `NewTypedPool[K](f)` | Custom allocator — use when the zero value is insufficient (e.g. a pool of pre-configured `DDSketch` objects). |

### Telemetry variant

```go
func NewDefaultTypedPoolWithTelemetry[K any](
    tm telemetry.Component, module, name string,
) Pool[K]
```

Wraps a `TypedPool` and increments Prometheus-style counters (`sync__pool.get`, `sync__pool.put`) and a gauge (`sync__pool.active`) on every operation. `module` and `name` become label values, making per-pool metrics queryable. The underlying `poolTelemetry` instance is memoized globally per `telemetry.Component` to avoid duplicate metric registration.

### Test helper (build tag `test`)

```go
func ResetGlobalPoolTelemetry()
```

Resets the memoized telemetry state. Call this in `TestMain` or individual test setups that create pools with telemetry, to prevent counter bleed between tests.

## Usage

### Reusable byte buffers — network and process code

Most pools in the codebase are `[]byte` pools created with `NewSlicePool`:

```go
// pkg/process/procutil/process_linux.go
var fdDirentPool = ddsync.NewSlicePool[byte](blockSize, blockSize)

// comp/dogstatsd/listeners/uds_common.go
pool := ddsync.NewSlicePool[byte](getUDSAncillarySize(), getUDSAncillarySize())

// pkg/util/address.go
var IPBufferPool = ddsync.NewSlicePool[byte](net.IPv6len, net.IPv6len)
```

Callers `Get()` a `*[]byte`, use it, then `Put()` it back. Because the pool returns a pointer to the slice (not the slice header value), the caller can reslice without breaking the pool contract.

### Reusable structs — eBPF records and process stats

```go
// pkg/ebpf/perf.go
var recordPool = ddsync.NewDefaultTypedPool[perf.Record]()

// pkg/process/encoding/encoding.go
var statPool = ddsync.NewDefaultTypedPool[model.ProcStatsWithPerm]()
```

### Custom allocator — DogStatsD packets and DDSketch

```go
// comp/dogstatsd/packets/pool.go
pool: ddsync.NewTypedPool(func() *Packet {
    return &Packet{buffer: make([]byte, packetSize)}
})

// pkg/network/protocols/sketchespool.go
SketchesPool = ddsync.NewTypedPool[ddsketch.DDSketch](func() *ddsketch.DDSketch {
    sketch, _ := ddsketch.NewDefaultDDSketch(...)
    return sketch
})
```

### Reusable `bytes.Buffer` — trace-agent remote config

```go
// cmd/trace-agent/config/remote/config.go
var bufferPool = ddsync.NewDefaultTypedPool[bytes.Buffer]()
```

Note: `bytes.Buffer` must be reset before reuse (`buf.Reset()`). `NewDefaultTypedPool` allocates with `new`, so the returned `*bytes.Buffer` is a valid zero-value buffer. The caller is responsible for resetting it before putting it back.

## Usage in the network subsystem

`pkg/network` is one of the largest consumers of `pkg/util/sync`. High-frequency eBPF event paths use pools to avoid per-event allocations under kernel callbacks:

- `pkg/network/protocols/` (http, kafka, postgres, redis) — `NewSlicePool` for per-protocol stat entry slices.
- `pkg/network/protocols/sketchespool.go` — `NewTypedPool[ddsketch.DDSketch]` with a custom allocator so pre-configured sketch objects are reused across latency measurements.
- `pkg/network/tracer/connection/perf_batching.go` and `tcp_close_consumer.go` — pools of eBPF batch event buffers.
- `pkg/network/netlink/consumer.go` — pools byte buffers used by the netlink conntrack consumer.

For the full list of network-package pool usage see the imports in `pkg/network/tracer/connection/` and `pkg/network/protocols/`.

## Telemetry variant — when to use

Use `NewDefaultTypedPoolWithTelemetry` when:
- The pool is on a critical hot path and you need to verify that pooling is actually effective (i.e. puts balance gets and the active count stays low).
- You are instrumenting a new subsystem and want per-pool metrics queryable in Datadog itself.

Avoid it in test code unless you call `ResetGlobalPoolTelemetry()` in cleanup — the memoized global state persists across test cases in the same binary and can cause duplicate metric registration errors.

## Pool contract reminders

- `Get()` returns a non-nil pointer. For `NewDefaultTypedPool` the zero value is returned; for `NewSlicePool` the slice is pre-allocated to the requested `size` and `capacity`.
- `Put()` does not zero or reset the value. Callers must reset any mutable state before putting an object back. Common patterns: `buf.Reset()` for `bytes.Buffer`, manual field zeroing for structs that hold sensitive data.
- `sync.Pool` objects may be collected by the GC between GC cycles. Do not store pointers to pool objects in long-lived data structures.

## Cross-references

| Document | Relationship |
|---|---|
| [`pkg/network`](../network/network.md) | Heaviest consumer; uses `NewSlicePool` and `NewTypedPool` in eBPF event-processing hot paths across protocols and connection tracers |
| [`pkg/logs`](../logs/logs.md) | The logs pipeline uses standard `sync.Pool` internally for message buffers; it does not currently depend on `pkg/util/sync` directly |
