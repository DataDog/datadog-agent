# comp/dogstatsd/listeners — Packet Listeners

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/listeners`
**Team:** agent-metric-pipelines

## Purpose

`comp/dogstatsd/listeners` is a collection of network listeners that receive raw StatsD packets and forward them (as batched `packets.Packets` values) to the DogStatsD server's processing channel. The package is not an fx component itself — it is a library used directly by `comp/dogstatsd/server`, which instantiates the appropriate listeners based on configuration.

Each listener implementation handles one transport protocol and is responsible for:
- Opening and managing the underlying socket/pipe.
- Reading incoming data in a dedicated goroutine.
- Assembling partial reads into complete packets (using `packets.Assembler` / `packets.Buffer`).
- Optionally detecting the originating container (origin detection, UDS only, Linux only).
- Optionally forwarding raw packets to `comp/dogstatsd/replay` for traffic capture.
- Emitting per-listener telemetry.

## Listener implementations

### `UDPListener`

Listens on a UDP socket (default port 8125). Multiple DogStatsD messages may be packed into a single UDP datagram. Origin detection is **not** supported on UDP.

Key config:
- `dogstatsd_port` — port number, `"__random__"` for OS-assigned, `0` to disable
- `dogstatsd_non_local_traffic` — bind to `0.0.0.0` instead of the local address
- `dogstatsd_so_rcvbuf` — `SO_RCVBUF` socket buffer size override
- `dogstatsd_buffer_size` — per-read buffer size in bytes

### `UDSDatagramListener` (Linux/macOS)

Listens on a Unix Domain Socket in datagram mode (`unixgram`). Each read produces one complete packet. Origin detection is available on Linux: when `dogstatsd_origin_detection` is enabled the kernel appends `SCM_CREDENTIALS` to each datagram, allowing the listener to resolve the sender's PID to a container ID via `workloadmeta`.

Key config:
- `dogstatsd_socket` — path to the UDS socket file
- `dogstatsd_origin_detection` — enable credential-based origin detection
- `dogstatsd_mem_based_rate_limiter.enabled` — rate-limit ingestion based on memory pressure

### `UDSStreamListener` (Linux/macOS, experimental)

Listens on a Unix Domain Socket in stream mode (`unix`). Each message is length-prefixed with a 4-byte little-endian uint32. Supports the same origin detection as the datagram variant.

Key config:
- `dogstatsd_stream_socket` — path to the stream UDS socket file
- `dogstatsd_stream_log_too_big` — log oversized payloads before dropping them

### `NamedPipeListener` (Windows only)

Listens on a Windows named pipe. Origin detection is **not** supported. Multiple concurrent client connections are multiplexed through a shared `packets.PacketManager`.

Key config:
- `dogstatsd_pipe_name` — pipe name (without the `\\.\pipe\` prefix)
- `dogstatsd_windows_pipe_security_descriptor` — SDDL security descriptor for the pipe

## StatsdListener interface

All implementations satisfy the same two-method interface:

```go
type StatsdListener interface {
    // Listen starts the read loop in a new goroutine.
    Listen()

    // Stop shuts down the listener and waits for the goroutine to exit.
    Stop()
}
```

The server calls `Listen()` on startup and `Stop()` on shutdown for each configured listener.

## Packet flow

```
Network (UDP/UDS/pipe)
    │  raw bytes
    ▼
listener.listen() goroutine
    │  packets.Assembler.AddMessage(buf[:n])
    ▼
packets.Buffer (batches packets for throughput)
    │  chan packets.Packets
    ▼
server.packetsIn channel
    │
    ▼
Worker goroutines (parse + enrich + batch → aggregator)
```

`packets.Assembler` and `packets.Buffer` are internal helpers from `comp/dogstatsd/packets` that accumulate reads into fixed-size batches and flush them either when the batch is full or after a configurable timeout (`dogstatsd_packet_buffer_flush_timeout`).

Packets are allocated from a shared pool (`packets.PoolManager`) to reduce GC pressure. When the server is done with a packet it calls `sharedPacketPoolManager.Put(packet)` to return it. During a traffic capture the pool is switched to non-passthrough mode, preventing reuse until the writer has serialised the packet.

## Origin detection (Linux only)

When `dogstatsd_origin_detection: true` is set and the listener is a UDS datagram/stream socket:

1. The listener calls `SO_PASSCRED` on the socket (`enableUDSPassCred`).
2. Each `ReadMsgUnix` call retrieves OOB data (`SCM_CREDENTIALS`) alongside the payload.
3. `processUDSOrigin` parses the credentials to extract the sender PID, then looks up the PID in `pidmap.Component` or `workloadmeta` to find the container ID.
4. The container ID is stored in `packet.Origin` and used by the server for tag enrichment.

Origin detection is a Linux kernel feature and is not available on macOS or Windows.

## Telemetry

Listener telemetry is collected in `TelemetryStore` and emitted under the `dogstatsd` Prometheus namespace:

| Metric | Labels | Description |
|---|---|---|
| `dogstatsd_udp_packets` | `state` (ok/error) | UDP packets received |
| `dogstatsd_udp_packets_bytes` | — | UDP bytes received |
| `dogstatsd_uds_packets` | `listener_id`, `transport`, `state` | UDS packets received |
| `dogstatsd_uds_packets_bytes` | `listener_id`, `transport` | UDS bytes received |
| `dogstatsd_uds_connections` (gauge) | `listener_id`, `transport` | Active UDS connections |
| `dogstatsd_uds_origin_detection_error` | `listener_id`, `transport` | Origin detection failures |
| `dogstatsd_listener_read_latency` (histogram) | `listener_id`, `transport`, `listener_type` | Time between consecutive reads |

`listener_id` is `"uds-<network>[-<fd>]"`. Including the file descriptor in the ID (`dogstatsd_telemetry_enabled_listener_id: true`) increases cardinality but allows correlating metrics with OS-level socket stats.

## Usage

### From the server

The server constructs listeners during `start()`:

```go
// UDP
udpListener, err := listeners.NewUDPListener(
    packetsChannel, sharedPacketPoolManager,
    s.config, s.tCapture,
    s.listernersTelemetry, s.packetsTelemetry)

// UDS datagram
unixListener, err := listeners.NewUDSDatagramListener(
    packetsChannel, sharedPacketPoolManager, sharedUDSOobPoolManager,
    s.config, s.tCapture, s.wmeta, s.pidMap,
    s.listernersTelemetry, s.packetsTelemetry, s.telemetry)

// Windows named pipe
namedPipeListener, err := listeners.NewNamedPipeListener(
    pipeName, packetsChannel, sharedPacketPoolManager,
    s.config, s.tCapture,
    s.listernersTelemetry, s.packetsTelemetry, s.telemetry)
```

All listeners are started after construction:

```go
for _, l := range s.listeners {
    l.Listen()
}
```

And stopped on shutdown:

```go
for _, l := range s.listeners {
    l.Stop()
}
```

### Writing a test that exercises a listener

Listeners are not fx components; they are constructed directly. In unit tests, replace the real `packetsChannel` with a buffered channel and supply a `packets.NewPool` and `packets.NewTelemetryStore`. The mock replay component (`comp/dogstatsd/replay/mock`) can be used in place of the full capture implementation so tests do not produce `.dog` files:

```go
packetsChannel := make(chan packets.Packets, 100)
pool := packets.NewPool(4096, telemetryStore)
poolManager := packets.NewPoolManager[packets.Packet](pool)
noopCapture := replaymock.NewNoopTrafficCapture()

listener, err := listeners.NewUDPListener(
    packetsChannel, poolManager,
    cfg, noopCapture,
    listenersTelemetry, packetsTelemetry)
listener.Listen()
// ... send UDP packets to the bound address ...
batch := <-packetsChannel
listener.Stop()
```

## Connection tracking

`connections_tracker.go` provides a `ConnectionTracker` helper (used by the stream listener) that tracks the set of active connections and provides a `CloseAll` method for clean shutdown. Each accepted connection runs in its own goroutine; `ConnectionTracker` maintains a `sync.WaitGroup` so `Stop` blocks until all connections have exited.

## Rate limiting

`listeners/ratelimit/` contains `MemBasedRateLimiter`, which pauses reads on a UDS connection when system memory pressure exceeds a configurable threshold. Enabled per-listener with `dogstatsd_mem_based_rate_limiter.enabled`.

## Related components

| Component | Doc | Relationship |
|---|---|---|
| `comp/dogstatsd/server` | [server.md](server.md) | The server instantiates all listeners during `start()`. It owns the `packetsIn` channel that listeners write to, and calls `Listen()` / `Stop()` on each listener. |
| `comp/dogstatsd/packets` | [packets.md](packets.md) | Provides `Packet`, `Pool`, `PoolManager`, `Buffer`, and `Assembler` — the low-level types that listeners use to read, assemble, and batch DogStatsD datagrams before forwarding them to the server. |
| `comp/dogstatsd/replay` | [replay.md](replay.md) | UDS listeners call `replay.Component.Enqueue(CaptureBuffer{...})` for every inbound packet when a capture is active. The `tCapture` field injected into each listener constructor is the replay component. |
| `comp/core/workloadmeta` | [../core/workloadmeta.md](../core/workloadmeta.md) | Injected into `UDSDatagramListener` and `UDSStreamListener` for Linux origin detection. After extracting the sender PID from `SCM_CREDENTIALS`, the listener calls `workloadmeta.GetContainerForProcess(pid)` to resolve the container ID stored in `packet.Origin`. |
