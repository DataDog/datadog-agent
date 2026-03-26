> **TL;DR:** Low-level memory-management primitives (zero-allocation pool, shared-ownership pool manager, batching buffer, and assembler) that move raw DogStatsD datagrams from the socket read loop to the server's parse workers without heap allocation on the hot path.

# comp/dogstatsd/packets — Packet Buffer Pools for DogStatsD

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/packets`
**Team:** agent-metric-pipelines
**Importers:** dogstatsd server, all DogStatsD listeners (UDP, UDS datagram, UDS stream, named pipe), replay capture/writer

## Purpose

`comp/dogstatsd/packets` provides the core data structures and memory-management primitives that move raw DogStatsD datagrams through the pipeline, from the moment a listener reads bytes off a socket until the server worker parses them into metric samples.

Three concerns are addressed in one place:

1. **Zero-allocation packet reuse** — `Pool` wraps a `sync.Pool` of pre-allocated `Packet` objects so that the hot path of reading a UDP datagram does not allocate.
2. **Shared-ownership pool management** — `PoolManager` extends a pool with reference counting so a single `Packet` can be handed to two consumers (e.g. server + replay capture) and returned to the pool only when both are done.
3. **Batching and backpressure** — `Buffer` accumulates packets from a listener and flushes them as a `Packets` slice to the server's input channel either when full or on a timer tick. `Assembler` packs multiple small UDP datagrams into a single `Packet` before handing it to a `Buffer`.

## Key elements

### Key types

#### `Packet`

```go
type Packet struct {
    Contents   []byte     // slice of Buffer pointing to the valid message(s)
    Buffer     []byte     // full underlying byte buffer (pre-allocated by the Pool)
    Origin     string     // container ID from origin detection, or NoOrigin ("")
    ProcessID  uint32     // PID from UDS credentials
    ListenerID string     // identifies which listener produced the packet
    Source     SourceType // UDP | UDS | NamedPipe
}
```

`Contents` is always a sub-slice of `Buffer` to avoid copies. Because `Buffer` is reused after `Pool.Put`, callers must finish reading or copy any data they need **before** returning the packet to the pool. Strings extracted via `string(Contents[n:m])` are safe because Go strings have independent storage.

#### `Packets`

`type Packets []*Packet` — a batch of packet pointers flushed together to the server channel. Both types implement `size.HasSizeInBytes` for memory-budget accounting.

#### `SourceType`

```go
const (
    UDP       SourceType = iota
    UDS
    NamedPipe
)
```

#### `Pool`

```go
func NewPool(bufferSize int, packetsTelemetry *TelemetryStore) *Pool
func (p *Pool) Get() *Packet
func (p *Pool) Put(packet *Packet)
```

`Get` returns a packet with a pre-allocated `Buffer` of `bufferSize` bytes. `Put` resets `Origin` to `NoOrigin` and returns the object to the underlying `sync.Pool`. Telemetry counters (`dogstatsd.packet_pool_get`, `dogstatsd.packet_pool_put`, `dogstatsd.packet_pool`) are updated when the telemetry component is enabled.

#### `PoolManager[K]`

```go
func NewPoolManager[K managedPoolTypes](gp genericPool[K]) *PoolManager[K]
func (p *PoolManager[K]) Get() *K
func (p *PoolManager[K]) Put(x *K)
func (p *PoolManager[K]) SetPassthru(b bool)
func (p *PoolManager[K]) IsPassthru() bool
func (p *PoolManager[K]) Count() int
func (p *PoolManager[K]) Flush()
```

In **passthru mode** (the default) `Put` returns the object immediately to the pool. When passthru is disabled, `Put` uses an internal `sync.Map` keyed on the pointer address: the first `Put` for a given pointer records it; the second `Put` returns it to the pool. This supports the replay capture path, where both the server and the capture writer hold a reference.

`Flush` force-returns all tracked objects and is called before enabling passthru again (`SetPassthru(true)` calls `Flush` automatically).

The type parameter `K` is constrained to `[]byte | Packet`.

#### `Buffer`

```go
func NewBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets,
               listenerID string, telemetryStore *TelemetryStore) *Buffer
func (pb *Buffer) Append(packet *Packet)
func (pb *Buffer) Close()
```

`Append` adds a packet and immediately flushes if `len(packets) >= bufferSize`. A background goroutine flushes on each `flushTimer` tick. `Close` sends on `closeChannel`, which triggers a final flush, then waits up to one second for the goroutine to finish.

#### `Assembler`

```go
func NewAssembler(flushTimer time.Duration, packetsBuffer *Buffer,
                  sharedPacketPoolManager *PoolManager[Packet], packetSourceType SourceType) *Assembler
func (p *Assembler) AddMessage(message []byte)
func (p *Assembler) Close()
```

Used by the UDP listener to pack multiple datagrams into one `Packet`. `AddMessage` copies the datagram into the current packet's `Buffer` (separated by `'\n'`); when the buffer would overflow, the current packet is flushed to the `Buffer` and a new one is obtained from the pool.

#### `TelemetryStore`

Holds all Prometheus-style metrics for the packet pipeline:

| Metric | Type | Description |
|---|---|---|
| `dogstatsd.packets_channel_size` | Gauge | Number of batches in the server input channel |
| `dogstatsd.packets_buffer_size` | Gauge | Current buffer fill per listener |
| `dogstatsd.packets_buffer_flush_timer` | Counter | Timer-triggered flushes |
| `dogstatsd.packets_buffer_flush_full` | Counter | Full-buffer-triggered flushes |
| `dogstatsd.packet_pool` | Gauge | Objects currently checked out of the pool |
| `dogstatsd.listener_channel_latency` | Histogram | Time to push a batch from listener to server |

`NewTelemetryStore(buckets []float64, telemetrycomp telemetry.Component)` is called once at server startup and shared across all `Pool`, `Buffer`, and `Assembler` instances.

## Usage in the codebase

### Listeners (UDP, UDS, named pipe)

Each listener creates a `Pool` and a `PoolManager[Packet]`, then an `Assembler` (UDP) or uses `Pool.Get/Put` directly (UDS stream). Packets are accumulated in a `Buffer` and sent as `Packets` batches to the shared server channel.

```
Listener reads datagram
  -> Pool.Get() -> Packet
  -> Assembler.AddMessage(datagram bytes)
  -> [on flush] Buffer.Append(packet)
  -> [on flush] outputChannel <- Packets{...}
```

### DogStatsD server (`comp/dogstatsd/server`)

The server holds a `PoolManager[Packet]`. After parsing a `Packets` batch it calls `poolManager.Put` for each packet to return it to the pool. When replay capture is active `SetPassthru(false)` is called so both the server and the capture writer can hold references simultaneously.

### Replay capture (`comp/dogstatsd/replay`)

The capture component receives raw `Packets` and writes them to a `.dog` file. It calls `poolManager.Put` after writing each packet, mirroring the server's call, which together satisfy the two-reference contract of `PoolManager`.

### Tagger (`comp/core/tagger/impl`)

Imports the package for `SourceType` constants when enriching metrics with container tags derived from UDS origin detection.

## Related components

| Component | Doc | Relationship |
|---|---|---|
| `comp/dogstatsd/listeners` | [listeners.md](listeners.md) | All listener implementations (`UDPListener`, `UDSDatagramListener`, `UDSStreamListener`, `NamedPipeListener`) create `Pool` / `PoolManager[Packet]` and `Buffer` / `Assembler` instances directly from this package. |
| `comp/dogstatsd/server` | [server.md](server.md) | Owns the `PoolManager[Packet]` and calls `Put` on each packet after processing. Calls `SetPassthru(false)` on the manager when a capture begins, then `SetPassthru(true)` when it ends. The server's `packetsIn` channel has type `chan Packets`. |
| `comp/dogstatsd/replay` | [replay.md](replay.md) | Registers pool managers via `RegisterSharedPoolManager` / `RegisterOOBPoolManager`. Calls `PoolManager.Put` after writing each captured packet to the `.dog` file, completing the two-reference contract. Also uses the `PoolManager[[]byte]` OOB pool for Unix socket credentials. |

## Pool lifecycle during replay capture

Understanding when packets are returned to the pool is critical for avoiding use-after-free bugs:

```
Normal operation (passthru=true):
  listener → Pool.Get() → packet
  server.Put(packet)          ← returned immediately

Capture active (passthru=false):
  listener → Pool.Get() → packet
  server.Put(packet)          ← first reference: tracked in sync.Map
  replay.Put(packet)          ← second reference: removed from map, returned to pool
```

`PoolManager.Flush()` is called automatically by `SetPassthru(true)` to drain any packets still tracked in the map when a capture is stopped.
