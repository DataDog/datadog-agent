> **TL;DR:** Records live DogStatsD traffic to a binary `.dog` file (with tagger-state snapshot) and replays it with original timing, enabling load testing and offline reproduction of production metric workloads.

# comp/dogstatsd/replay — Traffic Capture and Replay Component

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def`
**Team:** agent-metric-pipelines
**Importers:** ~12 packages

## Purpose

`comp/dogstatsd/replay` provides the ability to capture live DogStatsD traffic to a file and replay it later. Captures reproduce the original timing of packets, making them useful for load testing, reproducing production bugs offline, and writing deterministic integration tests against a recorded workload.

During a capture the component intercepts packets as they flow through UDS listeners, serialises them with their Unix socket ancillary data (PIDs, OOB credentials), and writes them to a binary file. The file also embeds a snapshot of the tagger state (entity tags per PID/container-ID), so replays can reproduce the tag context.

The component is always present in the main agent; in environments where capture is not needed the `fx-noop` variant is used.

## Package layout

| Package | Role |
|---|---|
| `comp/dogstatsd/replay/def` | `Component` interface and shared types (`CaptureBuffer`, `UnixDogstatsdMsg`) |
| `comp/dogstatsd/replay/impl` | Full implementation: `TrafficCapture`, `TrafficCaptureWriter`, `TrafficCaptureReader` |
| `comp/dogstatsd/replay/fx` | `Module()` — wires `impl.NewTrafficCapture` into fx |
| `comp/dogstatsd/replay/fx-mock` | fx module providing the mock for test apps |
| `comp/dogstatsd/replay/fx-noop` | fx module providing the no-op for production contexts that do not need capture |
| `comp/dogstatsd/replay/impl-noop` | No-op implementation (`noopTrafficCapture`) |
| `comp/dogstatsd/replay/mock` | Mock implementation for unit tests |

## Key elements

### Key interfaces

```go
type Component interface {
    // IsOngoing returns true while a capture is in progress.
    IsOngoing() bool

    // StartCapture begins recording to the file at path p for duration d.
    // If p is empty the agent's run_path/dsd_capture directory is used.
    // Returns the absolute path of the capture file, or an error.
    StartCapture(p string, d time.Duration, compressed bool) (string, error)

    // StopCapture halts an in-progress capture.
    StopCapture()

    // RegisterSharedPoolManager registers the packet pool used by listeners
    // so that the writer can return buffers to it when it is done with them.
    RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error

    // RegisterOOBPoolManager registers the OOB (ancillary data) pool used
    // by UDS listeners for origin detection credentials.
    RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error

    // Enqueue submits a captured packet for async writing.
    // Returns false if no capture is active or the writer queue is full.
    Enqueue(msg *CaptureBuffer) bool

    // GetStartUpError returns any error that prevented the component from
    // initialising (e.g. writer allocation failure).
    GetStartUpError() error
}
```

### Key types

```go
// CaptureBuffer carries a single captured packet and its metadata.
type CaptureBuffer struct {
    Pb          UnixDogstatsdMsg // Protobuf-serialisable payload + ancillary
    Oob         *[]byte          // Raw OOB data (Unix socket credentials)
    Pid         int32
    ContainerID string
    Buff        *packets.Packet  // Raw packet buffer (returned to pool after write)
}

// UnixDogstatsdMsg mirrors pb.UnixDogstatsdMsg without requiring a pbgo import.
type UnixDogstatsdMsg struct {
    Timestamp     int64
    PayloadSize   int32
    Payload       []byte
    Pid           int32
    AncillarySize int32
    Ancillary     []byte
}
```

`GUID = 999888777` is a magic constant used in replay mode to inject fake Unix socket credentials so replayed packets get the same origin tags as the original.

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `dogstatsd_capture_depth` | `0` (unlimited) | Channel buffer depth for async packet writing |
| `dogstatsd_capture_path` | `""` | Directory for capture files; falls back to `run_path/dsd_capture` |

## File format (`.dog`)

Capture files written by `TrafficCaptureWriter` use a custom binary format:

1. **Header** — a fixed magic byte sequence identifying the file as a Datadog DogStatsD capture.
2. **Records** — repeated: `[uint32 length][protobuf-encoded UnixDogstatsdMsg]`.
3. **State separator** — four zero bytes.
4. **Tagger state** — protobuf-encoded `pb.TaggerState` (PID→containerID map + per-entity tags).
5. **State size** — `uint32` length of the tagger state payload.

Files can optionally be compressed with zstd (`compressed = true` in `StartCapture`).

## How capture integrates with listeners

When a capture starts, `TrafficCaptureWriter.Capture` sets the shared packet-pool managers to non-passthrough mode. This causes listeners to hold packets in the pool (preventing them from being reused) until the writer has serialised them. Listeners that detect an ongoing capture wrap each inbound packet in a `CaptureBuffer` and call `replay.Component.Enqueue` before forwarding the packet to the main processing pipeline.

```
UDS Listener
  │  reads packet + OOB creds
  ├──► CaptureBuffer{Pb, Oob, ContainerID, ...}
  │        └──► replay.Enqueue(capBuff)   ← async, non-blocking
  │
  └──► packetsBuffer.Append(packet)       ← normal processing continues
```

## Replay (`TrafficCaptureReader`)

`TrafficCaptureReader` is an internal type (not exported through the `Component` interface) used by tooling and tests to replay a capture file. It reads messages sequentially, sleeping between messages to reproduce the original inter-packet timing, and sends each `pb.UnixDogstatsdMsg` on a channel for downstream consumption.

Replay is triggered externally (e.g. via a CLI tool or gRPC endpoint) rather than through the component itself. The component exposes the capture side; replay consumers interact with `TrafficCaptureReader` directly.

## fx wiring

```go
// Full implementation (main agent, includes capture API)
import replayfx "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx"
replayfx.Module()

// No-op (contexts where capture is never needed)
import replayfxnoop "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-noop"
replayfxnoop.Module()
```

`replayfx.Module()` also calls `fxutil.ProvideOptional` so the component can be injected as `option.Option[replay.Component]` in packages that work with or without capture support.

The implementation depends on:
- `comp/core/config` — reads `dogstatsd_capture_depth` and `run_path`
- `comp/core/tagger/def` — captures the current entity-tag state on stop

## Configuration keys

| Key | Default | Description |
|---|---|---|
| `dogstatsd_capture_depth` | `0` (unlimited) | Channel buffer depth for async packet writing |
| `dogstatsd_capture_path` | `""` | Directory for capture files; falls back to `run_path/dsd_capture` |

## Usage patterns

**Start and stop a capture from a gRPC handler:**

```go
type Requires struct {
    compdef.In
    Capture replay.Component
}

func (s *server) DogstatsdCaptureTrigger(req *pb.CaptureTriggerRequest) {
    path, err := s.capture.StartCapture(
        req.GetPath(),
        time.Duration(req.GetDuration())*time.Second,
        req.GetCompressed(),
    )
    // path is the absolute file location written
}

func (s *server) DogstatsdStopCapture() {
    s.capture.StopCapture()
}
```

**Check if a capture is active (e.g. before allowing a second capture):**

```go
if capture.IsOngoing() {
    return errors.New("a capture is already in progress")
}
```

**Using the no-op in serverless / lightweight contexts:**

```go
// serverless.go uses the noop directly to avoid the full impl dependency
import replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl-noop"
tc := replay.NewNoopTrafficCapture()
```

## Key importers

| Package | Usage |
|---|---|
| `comp/dogstatsd/server/server.go` | Registers pool managers; calls `Enqueue` per packet when capture is active; calls `StopCapture` on shutdown |
| `comp/api/grpcserver/impl-agent/grpc.go` | Exposes `StartCapture` / `StopCapture` / `IsOngoing` over the agent gRPC API |
| `comp/dogstatsd/listeners/uds_common.go` | Creates `CaptureBuffer` per packet and calls `Enqueue` |
| `comp/dogstatsd/listeners/uds_datagram.go` | Same as above for datagram transport |
| `comp/dogstatsd/listeners/uds_stream.go` | Same as above for stream transport |
| `comp/dogstatsd/listeners/named_pipe_windows.go` | Windows named-pipe listener (capture field present but currently ignored) |

## Related components

| Component | Doc | Relationship |
|---|---|---|
| `comp/dogstatsd/server` | [server.md](server.md) | Primary orchestrator: registers pool managers on startup, calls `StopCapture` during shutdown, and controls when `packetsIn` packets are enqueued for capture via the server worker loop. |
| `comp/dogstatsd/listeners` | [listeners.md](listeners.md) | UDS listeners construct a `CaptureBuffer` for every inbound packet and call `Enqueue` before forwarding the packet to the main processing channel. UDP and named-pipe listeners do not currently capture. |
| `comp/dogstatsd/packets` | [packets.md](packets.md) | `CaptureBuffer.Buff` is a `*packets.Packet`; after writing it the replay writer calls `PoolManager.Put` to complete the two-reference contract and return the buffer to the pool. |

## Usage patterns (extended)

### Replaying a `.dog` file in a test or tool

`TrafficCaptureReader` is not exported through the `Component` interface but is accessible directly from the `impl` package for tooling:

```go
import replayimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl"

reader, err := replayimpl.NewTrafficCaptureReader(captureFilePath, 10 /*depth*/)
if err != nil { ... }

go reader.Read(ctx)  // sends messages on reader.Traffic channel
for msg := range reader.Traffic {
    // msg is a *pb.UnixDogstatsdMsg with Payload and timing metadata
}
```

### Checking pool manager state before starting a capture

Always call `RegisterSharedPoolManager` / `RegisterOOBPoolManager` before `StartCapture`; if those registrations have not happened the writer will not be able to return packets to the pool:

```go
if err := capture.RegisterSharedPoolManager(packetPoolMgr); err != nil {
    return err
}
if err := capture.RegisterOOBPoolManager(oobPoolMgr); err != nil {
    return err
}
path, err := capture.StartCapture(dir, 30*time.Second, true)
```

### Injecting the no-op in a unit test

When a test exercises listener or server logic but does not assert on capture behaviour, use the noop to avoid `.dog` file side-effects:

```go
import replaynoop "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl-noop"

tc := replaynoop.NewNoopTrafficCapture()
// pass tc as the replay.Component argument to the listener / server under test
```
