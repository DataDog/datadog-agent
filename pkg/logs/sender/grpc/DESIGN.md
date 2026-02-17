# Stateful Transport Layer for Logs Agent

## Overview

This document describes the new gRPC-based stateful transport layer introduced in `pkg/logs/sender/grpc/`. This transport provides a bidirectional streaming alternative to the existing HTTP and TCP transports, with support for stateful encoding, delta compression, and stream lifecycle management.

## Architecture Summary

### Data Flow (per Pipeline)

Each pipeline has its own dedicated processing chain connected 1:1 to a streamWorker:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                                 Pipeline i                                     │
│                                                                                │
│  InputChan ──► Processor ──► MessageTranslator ──► BatchStrategy ──────────────┼──┐
│                (encodes)     (pattern extraction,   (batches datums,           │  │
│                              stateful conversion)   delta encoding,            │  │
│                                                     compression)               │  │
└────────────────────────────────────────────────────────────────────────────────┘  │
                                                                                    │
                                                           senderImpl.In()          │
                                                           (returns Queue i)        │
                                                                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                      Sender                                         │
│                                                                                     │
│  ┌─────────┐       ┌─────────┐       ┌─────────┐                 ┌─────────┐        │
│  │ Queue 0 │       │ Queue 1 │       │ Queue 2 │      ...        │ Queue N │        │
│  └────┬────┘       └────┬────┘       └────┬────┘                 └────┬────┘        │
│       │                 │                 │                           │             │
│       ▼                 ▼                 ▼                           ▼             │
│  ┌─────────┐       ┌─────────┐       ┌─────────┐                 ┌─────────┐        │
│  │Worker 0 │       │Worker 1 │       │Worker 2 │      ...        │Worker N │        │
│  │         │       │         │       │         │                 │         │        │
│  │ Superv. │       │ Superv. │       │ Superv. │                 │ Superv. │        │
│  │  ┌─┴─┐  │       │  ┌─┴─┐  │       │  ┌─┴─┐  │                 │  ┌─┴─┐  │        │
│  │  ▼   ▼  │       │  ▼   ▼  │       │  ▼   ▼  │                 │  ▼   ▼  │        │
│  │ Snd Rcv │       │ Snd Rcv │       │ Snd Rcv │                 │ Snd Rcv │        │
│  └────┬────┘       └────┬────┘       └────┬────┘                 └────┬────┘        │
│       │                 │                 │                           │             │
│       └─────────────────┴─────────────────┴───────────────────────────┘             │
│                                      │                                              │
│                          Shared gRPC ClientConn                                     │
└──────────────────────────────────────┼──────────────────────────────────────────────┘
                                       │
                                       ▼
                            ┌──────────────────────┐
                            │    Intake Server     │
                            │ (StatefulLogsService)│
                            └──────────────────────┘
```


### streamWorker Internal Architecture (Master-2-Slave Model)

```
                            ┌───────────────────────────────────────┐
                            │            streamWorker               │
  inputChan ───────────────►│                                       │
  (from BatchStrategy)      │  ┌─────────────────────────────────┐  │
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
                            │   (StatefulLogsService.LogsStream)    │
                            └───────────────────────────────────────┘
```

## Key Components

### 1. Sender (`sender.go`)

The `Sender` implements `PipelineComponent` interface and serves as the entry point.

**Key Design Decisions:**
- **Shared Connection**: All workers share a single `grpc.ClientConn` to reduce resource overhead
- **Round-Robin Distribution**: Uses `atomic.Uint32` for thread-safe input distribution
- **Lazy Connection**: Uses `grpc.NewClient()` which doesn't block

**Per-RPC Headers** (via `headerCredentials`):
- `dd-api-key`: API authentication
- `dd-protocol`: Protocol identifier (if specified)
- `dd-evp-origin` / `dd-evp-origin-version`: Origin tracking
- `dd-content-encoding`: Compression type or "identity"

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
| -------------- | -------------- | ----------------------------------------- |
| `disconnected` | `connecting`   | Backoff timer expires                     |
| `connecting`   | `active`       | Stream creation succeeds                  |
| `connecting`   | `disconnected` | Stream creation fails                     |
| `active`       | `connecting`   | Lifetime expires without unacked payloads |
| `active`       | `disconnected` | Failure (send/recv error, protocol error) |
| `active`       | `draining`     | Lifetime expires with unacked payloads    |
| `draining`     | `connecting`   | All acks received, OR drain timeout       |
| `draining`     | `disconnected` | Failure (send/recv error, protocol error) |

**State Descriptions:**
| State          | Description                                                                    |
| -------------- | ------------------------------------------------------------------------------ |
| `disconnected` | Waiting for backoff timer after failure (not initial - starts in `connecting`) |
| `connecting`   | Waiting for async stream creation to complete                                  |
| `active`       | Normal operation - sending batches, receiving acks                             |
| `draining`     | Stream lifetime expired, waiting for pending acks before rotation              |

**Supervisor Loop:**
The main select loop handles:
- Input from upstream (gated by `inflight.hasSpace()` for backpressure)
- Sending batches (enabled when `active` state + unsent items)
- Batch acknowledgments from receiver
- Stream failures from sender/receiver
- Stream creation results
- Stream lifetime timer (soft rotation)
- Drain timer (for unacked payloads)
- Backoff timer (retry after failure)

**Critical Flow - Batch Acknowledgment:**
1. Validates ack is for current stream (ignores stale signals)
2. Handles batch 0 (snapshot) separately - no payload to pop
3. Verifies BatchID matches expected sequence
4. On BatchID==1: triggers `DecError()` to reset backoff (stream proven operational)
5. Pops acknowledged payload, update the snapshot (details see later)
6. Sends payload to auditor

**Stream Lifetime Rotation and Draining:**

Streams are rotated periodically (default: 15 minutes). When the lifetime timer expires, the worker must transition to a new stream. The challenge is handling payloads that have been sent but not yet acknowledged. If we rotated immediately while acks were still in-flight, those payloads would be retransmitted unnecessarily, causing **duplicate logs on the server side**. The drain state exists to wait for pending acks before rotating, minimizing unnecessary retransmissions. If there are no unacked payloads when the lifetime timer fires, the worker skips the drain state entirely and transitions directly to `connecting`. Draining works as follow:

1. **Lifetime expires with unacked payloads** → enter `draining` state, start drain timer (5s)
2. **During draining:**
   - **No new batches are sent** - the send channel is disabled (`sendChan = nil` when state ≠ active)
   - **New input is still accepted** - payloads continue buffering in the inflight tracker
   - **Acks continue arriving** - the receiver goroutine is still running, forwarding acks to supervisor
   - **Each ack pops a payload** - reducing the unacked count
3. **Exit draining when:**
   - All acks received (`!inflight.hasUnacked()`) → rotate with zero retransmissions, OR
   - Drain timer expires → rotate anyway, retransmitting any remaining unacked payloads




### 3. Inflight Tracker (`inflight.go`)

A bounded FIFO queue with two regions:

```
[--sent awaiting ack--][--buffered not sent--]
^                      ^                      ^
head                 sentTail                 tail
```

**Key Features:**
- **Capacity**: 10,000 payloads (`maxInflight` constant)
- **Ring buffer**: Uses "waste one slot" pattern (capacity+1 slots)
- **BatchID tracking**: Sequential IDs starting at 1 (0 reserved for snapshot)
- **Snapshot state**: Maintains accumulated pattern/dictionary definitions

**Snapshot Management:**
When payloads are acknowledged (popped), their `StatefulExtra.StateChanges` are applied to the snapshot. On stream rotation, this snapshot is serialized and sent as batch 0 to bootstrap the new stream's state. For details on snapshot management, see the [design doc](https://docs.google.com/document/d/1E6BsjzOS36e7TZBN2p1KJoPNymUM-0I8bGQIAhLjoho/edit?pli=1&tab=t.vy32dxhupwgk#bookmark=id.lyk8wjlnsr2).

### 4. Batch Strategy (`batch_strategy.go`)

Collects `StatefulMessage` datums and creates compressed `Payload` objects. The batching algorithm is exactly the same as the existing HTTP one.

**Delta Encoding:**
| Field      | Encoding Strategy                                                                           |
| ---------- | ------------------------------------------------------------------------------------------- |
| Timestamp  | First message: absolute. Same as previous: omit (0). Clock skew: absolute. Otherwise: delta |
| Pattern ID | Omit if same as previous                                                                    |
| Tags       | Omit if same dictionary index as previous                                                   |

**Flush Process:**
1. Collect message metadata and datums
2. Reset delta encoding state for next batch
3. Extract state changes for snapshot management
4. Create `DatumSequence` protobuf
5. Marshal and compress
6. Create `Payload` with `StatefulExtra` containing state changes

### 5. Protocol Buffer Definitions (`stateful_encoding.proto`)

**Service Definition:**
```protobuf
service StatefulLogsService {
  rpc LogsStream(stream StatefulBatch) returns (stream BatchStatus);
}
```

**Message Hierarchy:**
```
StatefulBatch (batch_id, compressed data)
  └── DatumSequence (ordered array)
        └── Datum (oneof)
              ├── PatternDefine (pattern_id, template, param_count, pos_list)
              ├── PatternDelete
              ├── DictEntryDefine (id, value)
              ├── DictEntryDelete
              └── Log (timestamp, structured/raw content, tags)
                    └── StructuredLog (pattern_id, dynamic_values, json_context)
```

## streamWorker Concurrency Model

### Why Master-2-Slave?

The streamWorker uses a **Master-2-Slave threading model** (1 supervisor + 2 I/O goroutines) because both directions of gRPC streaming I/O can block:

| Operation                   | Blocking Behavior                                                  |
| --------------------------- | ------------------------------------------------------------------ |
| `stream.Send()`             | Blocks on flow control, network backpressure, or connection issues |
| `stream.Recv()`             | Blocks indefinitely waiting for server responses                   |
| `conn.WaitForStateChange()` | Blocks until connection state changes or timeout                   |

If the supervisor performed I/O directly, it could not respond to:
- New input from upstream (would cause upstream backpressure)
- Timer events (stream lifetime, drain timeout, backoff)
- Shutdown signals (would delay graceful termination)

The same reasoning applies to **stream creation** - `WaitForStateChange()` can block up to `connectionTimeout` (10s), so it runs in a separate one-shot goroutine (`asyncCreateNewStream`).

### Goroutine Responsibilities

| Goroutine          | Lifetime                                  | Responsibility                                                               |
| ------------------ | ----------------------------------------- | ---------------------------------------------------------------------------- |
| **Supervisor**     | Entire worker lifetime                    | Owns all state & inflight, makes all decisions, coordinates I/O goroutines   |
| **Sender**         | Per-stream (created on stream activation) | Stateless; reads batches from channel, calls `Send()`, reports failures      |
| **Receiver**       | Per-stream (created on stream activation) | Stateless; calls `Recv()` in loop, forwards acks, detects stream termination |
| **Stream Creator** | One-shot per creation attempt             | Waits for connection ready, creates stream, signals result                   |

**Key insight**: Sender and Receiver goroutines are **stateless** - they don't make decisions, only perform I/O and report results. All state transitions happen in the Supervisor.

### Resource Ownership

```
Supervisor owns:
├── streamState (disconnected/connecting/active/draining)
├── currentStream (*streamInfo - the active gRPC stream)
├── inflight (*inflightTracker - all payload state)
├── timers (streamTimer, drainTimer, backoffTimer)
├── backoff state (nbErrors, backoffPolicy)
└── channels (creates and closes batchToSendCh)

Shared (thread-safe):
├── grpc.ClientConn (shared across all workers, thread-safe by design)
└── StatefulLogsServiceClient (derived from conn, thread-safe)

Per-stream (owned by streamInfo):
├── stream (gRPC bidirectional stream)
├── ctx (per-stream context)
└── cancel (cancellation function for cleanup)
```

### Synchronization Mechanisms

**No mutexes are used.** All synchronization is achieved through channels:

| Channel           | Direction                    | Buffer | Purpose                                        |
| ----------------- | ---------------------------- | ------ | ---------------------------------------------- |
| `inputChan`       | BatchStrategy → Supervisor   | 100    | Incoming payloads from upstream                |
| `batchToSendCh`   | Supervisor → Sender          | 10     | Batches ready to send                          |
| `batchAckCh`      | Receiver → Supervisor        | 10     | Acknowledgments from server                    |
| `streamFailureCh` | Sender/Receiver → Supervisor | 0      | Failure signals (blocking intentional)         |
| `streamReadyCh`   | Creator → Supervisor         | 0      | Stream creation results (blocking intentional) |
| `stopChan`        | External → All               | 0      | Shutdown signal (close to broadcast)           |

**Why unbuffered for failure/ready channels?** These are blocking by design - when a sender/receiver fails or a stream is created, the goroutine should wait until the supervisor acknowledges. This prevents race conditions during stream rotation.

### Conditional Channel Selection

The supervisor uses Go's pattern of setting channels to `nil` to conditionally enable/disable operations:

```go
// Backpressure: only read input when inflight has capacity
var inputChan <-chan *message.Payload
if s.inflight.hasSpace() {
    inputChan = s.inputChan  // Enable
} else {
    inputChan = nil          // Disable - select will skip this case
}

// Only send when active with unsent payloads
var sendChan chan<- *statefulpb.StatefulBatch
if s.streamState == active && s.inflight.hasUnSent() {
    sendChan = s.batchToSendCh
    nextBatch = s.getNextBatch()  // Idempotent peek
} else {
    sendChan = nil
}
```

### Stale Signal Handling

During stream rotation, the old stream's sender/receiver goroutines may still be running and could send signals. To prevent these stale signals from corrupting state:

1. **All signals carry stream identity** (`*streamInfo` pointer)
2. **Supervisor validates before processing**:
   ```go
   if ack.stream != s.currentStream {
       return  // Ignore stale ack from old stream
   }
   ```
3. **Context cancellation** ensures old goroutines exit promptly

### Lifecycle: Stream Rotation

When rotating streams (soft or hard):

1. **Close `batchToSendCh`** - signals Sender to exit after draining
2. **Cancel stream context** - causes `Send()`/`Recv()` to return with context error
3. **Old goroutines detect cancellation and exit** (no failure signal sent for context errors)
4. **Create new `batchToSendCh`** for new stream
5. **`finishStreamRotation()`** spawns new Sender/Receiver goroutines
6. **`inflight.resetOnRotation()`** converts unacked payloads back to unsent for retransmission

After these steps, the new stream is considered ready. The first message onto the new stream is a state snapshot to initialize the new backend, then any unsent messages in the inflight queue can be transmitted.

## Integration with Logs Agent

**Configuration:**
- `logs_config.use_grpc`: Enable gRPC transport (default: `false`)
- `logs_config.stream_lifetime`: Stream rotation interval (default: 900s / 15 minutes)
- Environment: `DD_LOGS_CONFIG_USE_GRPC=true`


## Future Works

1. **Irrecoverable Errors**:
   Currently treated as stream errors causing retry loops. maybe block ingestion for auth/protocol errors?

2. **Graceful Shutdown**:
   Current implementation may lose some acks during shutdown.

3. **Stream Jitter**:
   All streams rotate at the same time without jitter, potentially causing thundering herd.

4. **Single Endpoint Support**:
   Currently only uses first reliable endpoint. TODO: support multiple endpoints with failover.

5. **No Stream Negotiation**:
   Currently no capability to downgrade to HTTP transport if gRPC fails.

6. **Proper Backpressure**:
   Evaluating if gRPC/HTTP2 flow control is adequate, if not implement flow control at application level


## Constants Worth Noting

These constants are subjected to tweaking after benchmarking (especially the `maxInflight`)

| Constant              | Value  |
| --------------------- | ------ |
| `inputChanBufferSize` | 100    |
| `ioChanBufferSize`    | 10     |
| `maxInflight`         | 10,000 |
| `connectionTimeout`   | 10s    |
| `drainTimeout`        | 5s     |

