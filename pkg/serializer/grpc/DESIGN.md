# Stateful gRPC Transport for Metrics Serializer

## Overview

This package implements a gRPC-based stateful transport for the metrics serializer in `pkg/serializer/`. It provides a bidirectional streaming alternative to the existing HTTP transport, with support for stateful dictionary encoding, delta compression, and stream lifecycle management.

**Companion documents:**
- Stream dictionary and ID internment: `pkg/serializer/internal/metrics/stream_dictionary.go`
- Payload building and outer compression: `pkg/serializer/internal/metrics/iterable_series_v3_stateful.go`
- Proto definitions: `pkg/proto/datadog/stateful/stateful_encoding.proto`

## Architecture Summary

### Data Flow

The metrics flush pipeline has a single stateful path per serializer instance. There is one process-wide `Sender` singleton (initialized on first use) shared across flushes:

```
┌─────────────────────────────────────────────────────────────────┐
│                         Aggregator                              │
│                     (15-second flush)                           │
└──────────────────────────┬──────────────────────────────────────┘
                           │  []*metrics.Serie (N series)
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│             payloadsBuilderV3Stateful                           │
│         (pkg/serializer/internal/metrics/)                      │
│                                                                 │
│  1. Pre-intern tagsets → assign/cache tagset IDs               │
│  2. Sort series by tagset ID (sequential delta encoding)        │
│  3. Per series: InternName, InternTags, InternResources, ...    │
│     → appends define datums on cache miss                       │
│  4. Assemble MetricDatumSequence{defines..., MetricSeriesBatch} │
│  5. Marshal proto + outer-compress (zstd)                       │
│  6. sender.Submit(Payload)                                      │
└──────────────────────────┬──────────────────────────────────────┘
                           │  Payload (compressed bytes + metadata)
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Sender                                   │
│                   (pkg/serializer/grpc/)                        │
│                                                                 │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐                    │
│  │ Queue 0  │   │ Queue 1  │   │ Queue N  │  (round-robin)     │
│  └────┬─────┘   └────┬─────┘   └────┬─────┘                    │
│       ▼              ▼              ▼                           │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐                    │
│  │ Worker 0 │   │ Worker 1 │   │ Worker N │                    │
│  └────┬─────┘   └────┬─────┘   └────┬─────┘                    │
│       └──────────────┴──────────────┘                          │
│                  Shared gRPC ClientConn                         │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
               ┌───────────────────────┐
               │     Intake Server     │
               │ StatefulMetricsService│
               └───────────────────────┘
```

### streamWorker Internal Architecture (Master-2-Slave Model)

```
                            ┌───────────────────────────────────────┐
                            │            streamWorker               │
  inputChan ───────────────►│                                       │
  (from Submit)             │  ┌─────────────────────────────────┐  │
                            │  │      Supervisor Goroutine       │  │
                            │  │                                 │  │
                            │  │  • Manages inflight tracker     │  │
                            │  │  • State machine control        │  │
                            │  │  • Stream lifecycle/rotation    │  │
                            │  │  • Backpressure management      │  │
                            │  │  • Retransmission on rotation   │  │
                            │  └───────────────┬─────────────────┘  │
                            │            ┌─────┴─────┐              │
                            │            ▼           ▼              │
                            │     ┌───────────┐ ┌───────────┐       │
                            │     │  Sender   │ │ Receiver  │       │
                            │     │ Goroutine │ │ Goroutine │       │
                            │     │           │ │           │       │
                            │     │  stream.  │ │  stream.  │       │
                            │     │  Send()   │ │  Recv()   │       │
                            │     └─────┬─────┘ └─────┬─────┘       │
                            │           │             │             │
                            └───────────┼─────────────┼─────────────┘
                                        │             │
                                        ▼             ▼
                            ┌───────────────────────────────────────┐
                            │     Bidirectional gRPC Stream         │
                            │  (StatefulMetricsService.MetricsStream│
                            └───────────────────────────────────────┘
```

## Key Components

### 1. Sender (`sender.go`)

Entry point for the transport layer. Accepts `Payload` objects from the serializer and distributes them across workers.

**Key Design Decisions:**
- **Shared Connection**: All workers share a single `grpc.ClientConn` to reduce resource overhead
- **Round-Robin Distribution**: Uses `atomic.Uint32` for thread-safe distribution
- **Lazy Connection**: Uses `grpc.NewClient()` — does not block at construction

**Per-RPC Headers** (via `headerCredentials`):
- `dd-api-key`: API authentication
- `dd-protocol`: Protocol identifier
- `dd-evp-origin` / `dd-evp-origin-version`: Origin tracking
- `dd-content-encoding`: Compression type (e.g. `"zstd"`) or `"identity"`

### 2. Stream Worker (`stream_worker.go`)

Implements a **Master-2-Slave threading model**: one supervisor + one sender + one receiver per stream.

**State Machine:**

```
                    ┌──────────────┐
        ┌──────────►│  connecting  │◄─────────────────┐
        │  backoff  └──┬─────┬─────┘                  │
        │         fail │     │ success                │
        │      ┌───────┘     │                        │
        │      ▼             ▼              ltm exp   |
┌───────┴───────┐        ┌──────────┐      w/o unacked│
│ disconnected  │◄───────┤  active  ├─────────────────┤
└───────────────┘  fail  └────┬─────┘                 │
        ▲                     │ ltm exp w/unacked     │
        │                     ▼                       │
        │              ┌──────────┐    drain done     │
        └──────────────┤ draining ├───────────────────┘
               fail    └──────────┘    or timeout
```

**State Transitions:**

| From           | To             | Trigger                                   |
|----------------|----------------|-------------------------------------------|
| `disconnected` | `connecting`   | Backoff timer expires                     |
| `connecting`   | `active`       | Stream creation succeeds                  |
| `connecting`   | `disconnected` | Stream creation fails                     |
| `active`       | `connecting`   | Lifetime expires without unacked payloads |
| `active`       | `disconnected` | Failure (send/recv error, protocol error) |
| `active`       | `draining`     | Lifetime expires with unacked payloads    |
| `draining`     | `connecting`   | All acks received, OR drain timeout       |
| `draining`     | `disconnected` | Failure (send/recv error, protocol error) |

**Stream Lifetime Rotation and Draining:**

Streams rotate periodically (default: 900s). On expiry with unacked payloads, the worker enters `draining` to wait for pending acks before rotating. This avoids unnecessary retransmissions:

1. **Lifetime expires with unacked payloads** → enter `draining`, start drain timer (5s)
2. **During draining:** no new batches sent; new input buffered; acks continue arriving
3. **Exit draining:** all acks received (zero retransmissions) OR drain timer expires (retransmit remaining)

On stream rotation, `inflight.resetOnRotation()` converts unacked payloads back to unsent. The first message on the new stream is a state **snapshot** (batch ID 0) that bootstraps the new backend's dictionary state.

**Snapshot Compression:**

The snapshot is compressed using the same compressor that was used for normal payloads (stored in `streamWorker.compression`). This ensures the new stream's backend receives a correctly-encoded bootstrap even if the compressor is stateful (e.g. zstd with a shared context).

### 3. Inflight Tracker (`inflight.go`)

A bounded FIFO queue with two logical regions:

```
[--sent awaiting ack--][--buffered not sent--]
^                      ^                      ^
head                 sentTail                 tail
```

**Key Features:**
- Ring buffer with "waste one slot" pattern
- BatchID tracking: sequential IDs starting at 1 (0 reserved for snapshot)
- Snapshot state: accumulates `StateChanges` from acknowledged payloads
- On rotation: snapshot is serialized and sent as batch 0 to bootstrap the new stream

### 4. Payload (`payload.go`)

`Payload` carries compressed bytes from the serializer to the transport layer. Fields:

| Field               | Description                                          |
|---------------------|------------------------------------------------------|
| `Encoded`           | Compressed serialized `MetricDatumSequence`          |
| `Encoding`          | Compression type string (e.g. `"zstd"`)              |
| `UnencodedSize`     | Size before compression (for telemetry)              |
| `PreCompressionBytes` | Proto-marshaled size before outer compression      |
| `PointCount`        | Number of metric data points                         |
| `StateChanges`      | New defines introduced in this payload (for snapshot)|

## Proto Message Hierarchy

```
MetricStatefulBatch (batch_id, data: compressed bytes)
  └── MetricDatumSequence (ordered array of MetricDatum)
        └── MetricDatum (oneof)
              ├── MetricNameDefine         (name_id, name)
              ├── MetricTagStringDefine    (tag_string_id, tag_string)
              ├── MetricTagsetDefine       (tagset_id, tag_string_ids[])
              ├── MetricResourceStringDefine (resource_string_id, value)
              ├── MetricResourceDefine     (resource_id, type_id, name_id)
              ├── MetricOriginDefine       (origin_id, OriginInfo)
              ├── MetricSourceTypeNameDefine (source_type_name_id, name)
              └── MetricSeriesBatch
                    └── MetricDatum[]
                          └── MetricDatum (refs to defined IDs + points)
```

**ID spaces** (all per-stream, monotonically from 1, defined in `StreamDictionary`):

| Kind             | Field in MetricDatum    |
|------------------|-------------------------|
| metric name      | `name_id`               |
| tag string       | `tag_string_id`         |
| tagset           | `tagset_id`             |
| resource string  | `resource_string_id`    |
| resource         | `resource_id`           |
| origin           | `origin_id`             |
| source type name | `source_type_name_id`   |

## Delta Encoding — Tagset Sort Optimization

Tagset and name refs are varint-encoded deltas between consecutive emissions. To keep these deltas small (~1 byte instead of ~2), the payload builder **pre-interns all tagsets** and then **sorts series by tagset ID ascending** before encoding. This ensures refs are emitted in sequential order, producing deltas of ±1 in steady state.

Without sorting, Go map iteration order is random, producing scattered deltas and ~2× larger ref fields.

Cost: three short-lived slices per flush (pointers + uint64 IDs + sort indices, ~120 KB at 5K series) plus an O(N log N) sort (~0.2 ms at 5K series, 15s flush interval).

## Concurrency Model

### Why Master-2-Slave?

Both gRPC I/O directions can block:

| Operation              | Blocking Behavior                                         |
|------------------------|-----------------------------------------------------------|
| `stream.Send()`        | Blocks on flow control, network backpressure, or errors   |
| `stream.Recv()`        | Blocks indefinitely waiting for server acks               |
| `WaitForStateChange()` | Blocks until connection state changes or timeout          |

The supervisor must remain responsive to input, timers, and shutdown. It delegates all blocking I/O to satellite goroutines.

### Goroutine Responsibilities

| Goroutine      | Lifetime                  | Responsibility                                                             |
|----------------|---------------------------|----------------------------------------------------------------------------|
| **Supervisor** | Entire worker lifetime    | Owns all state & inflight; makes all decisions; coordinates I/O goroutines |
| **Sender**     | Per-stream                | Stateless; reads batches from channel, calls `Send()`, reports failures    |
| **Receiver**   | Per-stream                | Stateless; calls `Recv()` in loop, forwards acks, detects termination      |
| **Creator**    | One-shot per attempt      | Waits for connection ready, creates stream, signals result                 |

### Synchronization

**No mutexes.** All synchronization is via channels:

| Channel           | Direction                  | Buffer | Purpose                              |
|-------------------|----------------------------|--------|--------------------------------------|
| `inputChan`       | Submit → Supervisor        | 100    | Incoming payloads                    |
| `batchToSendCh`   | Supervisor → Sender        | 10     | Batches ready to transmit            |
| `batchAckCh`      | Receiver → Supervisor      | 10     | Acks from server                     |
| `streamFailureCh` | Sender/Receiver → Supervisor | 0    | Failure signals (blocking)           |
| `streamReadyCh`   | Creator → Supervisor       | 0      | Stream creation result (blocking)    |
| `stopChan`        | External → All             | 0      | Shutdown (close to broadcast)        |

Failure and ready channels are unbuffered by design: the satellite goroutine blocks until the supervisor acknowledges, preventing races during stream rotation.

The supervisor uses `nil` channel assignment to conditionally enable/disable select cases (backpressure, send-when-active gating).

### Stale Signal Handling

All signals carry a `*streamInfo` pointer. The supervisor validates identity before processing, discarding signals from previous streams.

## Configuration

The **destination host and API key are NOT configured here** — they are derived
from the agent's existing `DomainResolver` (the standard `dd_url`/site/`api_key`/
`additional_endpoints` config). The gRPC sender dials the resolver's base domain
(`GetBaseDomain()`, parsed to `host:port` + TLS) and reads the key per-RPC from
`GetAPIKeys()`. Only gRPC *tuning* lives under `grpc.*`:

| Key                          | Default  | Description                          |
|------------------------------|----------|--------------------------------------|
| `enabled`                    | `false`  | Enable stateful gRPC path            |
| `grpc.compression_kind`      | `"zstd"` | Outer compression codec              |
| `grpc.compression_level`     | `1`      | Compression level                    |
| `grpc.stream_lifetime`       | `900s`   | Stream rotation interval             |
| `grpc.max_inflight_payloads` | `50`     | Max in-flight payloads per worker    |
| `grpc.drain_timeout`         | `5s`     | Max wait for acks before rotation    |
| `grpc.connection_timeout`    | `10s`    | gRPC connection establishment timeout|

## Future Work

1. **Graceful Shutdown**: Current implementation may lose acks during shutdown.
2. **Stream Jitter**: All workers rotate at the same time — thundering herd risk.
3. **Multiple Endpoints**: Currently uses only first endpoint; no failover.
4. **Irrecoverable Errors**: Auth/protocol errors currently retry indefinitely.
5. **Shared Transport**: Extend to logs and traces (gRPC connection is already shared-friendly — `ClientConn` is per-`Sender` instance, not global, so sharing across telemetry types requires a higher-level connection pool).
