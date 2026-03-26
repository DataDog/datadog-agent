> **TL;DR:** `pkg/logs/sender` is the batching and delivery layer that converts encoded log messages into `Payload` objects and distributes them to one or more remote destinations (HTTP or TCP), with at-least-once delivery guarantees for reliable destinations and best-effort non-blocking sends for unreliable ones.

# pkg/logs/sender

## Purpose

`pkg/logs/sender` is the batching, multiplexing, and delivery layer of the logs pipeline. It takes encoded `message.Payload` objects produced by the pipeline strategy and forwards them to one or more remote destinations (HTTP or TCP), with the following guarantees:

- At least one **reliable** destination must accept each payload before the pipeline moves on.
- **Unreliable** destinations (e.g. additional intake endpoints) receive a best-effort, non-blocking send; drops are counted in telemetry but do not stall the pipeline.
- If a reliable destination is retrying (e.g. due to a transient HTTP 5xx), the worker suspends new sends to that destination without blocking other destinations.
- Multi-region failover (MRF) destinations receive only payloads explicitly marked `IsMRF` and are gated by `multi_region_failover.enabled` / `multi_region_failover.failover_logs` config flags.

The package also contains the two `Strategy` implementations (`BatchStrategy`, `StreamStrategy`) that sit upstream of the sender and convert `message.Message` streams into `message.Payload` streams.

## Key elements

### Key interfaces

| Interface | Description |
|-----------|-------------|
| `Strategy` | `Start()` / `Stop()` lifecycle. Converts an incoming `chan *message.Message` into payloads on an output `chan *message.Payload`. Implemented by `batchStrategy` and `streamStrategy`. |
| `PipelineComponent` | Implemented by `Sender`. Exposes `In() chan *message.Payload`, `PipelineMonitor()`, `Start()`, `Stop()`. Used by `Pipeline` to attach the strategy output to the sender input. |
| `Sink` | `Channel() chan *message.Payload`. Receives payloads that have been acknowledged by a reliable destination (typically the auditor). `NoopSink` is provided for cases where no auditor is needed. |
| `ServerlessMeta` | Carries `sync.WaitGroup` and `SenderDoneChan` used to synchronise serverless flush — the pipeline blocks `Flush()` until all in-flight payloads have been delivered. |

### Key types

#### `Sender`

```go
type Sender struct {
    workers []*worker
    queues  []chan *message.Payload
    // ...
}
```

A multiplexer over one or more queues, each served by one or more `worker` goroutines.

```go
func NewSender(config, sink, destinationFactory, bufferSize, serverlessMeta, queueCount, workersPerQueue, pipelineMonitor) *Sender
```

`In()` returns the next queue channel (round-robin). Each call to the factory creates a fresh `*client.Destinations` (set of reliable + unreliable destinations) for that worker.

Default topology (non-legacy, non-serverless HTTP):

| Parameter | Value |
|-----------|-------|
| `queueCount` (`DefaultQueuesCount`) | 1 |
| `workersPerQueue` (`DefaultWorkersPerQueue`) | 1 |
| `minSenderConcurrency` | `numberOfPipelines` |
| `maxSenderConcurrency` | `numberOfPipelines × 10` |

Legacy mode (one queue per pipeline) and serverless mode (one worker per pipeline, concurrency 1) are handled separately by `provider.go`.

#### `worker`

Reads from a shared queue channel and sends each payload to all configured destinations:

1. **Reliable destinations** — blocking `Send()`. The loop retries every 100 ms if all reliable destinations are blocked (back-pressure). Once one succeeds the payload is considered sent. Remaining reliable destinations that failed in step 1 get a non-blocking `NonBlockingSend()` attempt to buffer against transient failures.
2. **Unreliable destinations** — non-blocking `NonBlockingSend()`. Drops are counted in `logs_sender.payloads_dropped` and `logs_sender.messages_dropped` telemetry.

Serverless mode adds a `sync.WaitGroup` hand-off via `senderDoneChan` so that `pipeline.Flush()` blocks until all destinations have finished.

#### `DestinationSender`

Wraps a single `client.Destination`. Maintains a retry flag (updated by the destination over `retryReader`) to decide whether to block or skip a send attempt. Key methods:

| Method | Behaviour |
|--------|-----------|
| `Send(payload)` | Blocks on the input channel unless the destination is currently retrying; returns `false` if it was retrying when called. |
| `NonBlockingSend(payload)` | Returns `false` immediately if the buffer is full. |

MRF destinations are enabled/disabled dynamically based on config reads inside `canSend()`.

### Key functions

#### `Strategy` implementations

#### `batchStrategy` (HTTP)

Groups messages into batches bounded by `maxBatchSize` (message count) and `maxContentSize` (bytes). Maintains separate `batch` objects keyed on `"main"` and `"mrf"` so that MRF payloads are always batched independently.

A flush timer (`batchWait`, from `endpoints.BatchWait`) fires periodically to prevent messages from waiting indefinitely. An explicit `flushChan` is also provided for on-demand flush (serverless).

Each `batch` uses a `MessageBuffer` (count + size tracking), a `Serializer` (JSON array formatter), and a `StreamCompressor` to produce the final encoded `Payload`.

```go
sender.NewBatchStrategy(inputChan, outputChan, flushChan, serverlessMeta,
    batchWait, maxBatchSize, maxContentSize, pipelineName, compression,
    pipelineMonitor, instanceID)
```

#### `streamStrategy` (TCP)

Creates one `Payload` per `Message`. No batching. Compresses each message individually (typically with `NoneKind`). Used when `!endpoints.UseHTTP && !serverless`.

```go
sender.NewStreamStrategy(inputChan, outputChan, compression)
```

#### `Serializer`

`NewArraySerializer()` — serialises a batch of messages as a JSON array `[{…},{…}]` by writing directly to a `StreamCompressor`-backed `io.Writer`.

#### `MessageBuffer`

Tracks metadata (not full content) of messages accumulating in the current batch. Enforces `batchSizeLimit` (count) and `contentSizeLimit` (total uncompressed bytes). Messages that exceed the content size limit alone are dropped with a warning.

#### Sub-packages

| Package | Description |
|---------|-------------|
| `sender/http` | `NewHTTPSender` — constructs a `Sender` with HTTP destinations. Accepts min/max concurrency to configure adaptive concurrency inside `pkg/logs/client/http.NewDestination`. Supports an `evpCategory` header for Event Platform routes. |
| `sender/tcp` | `NewTCPSender` — constructs a `Sender` with TCP destinations. TCP is synchronous; all concurrency is expressed as discrete workers. |

### Configuration and build flags

#### Telemetry

| Metric | Tags | Description |
|--------|------|-------------|
| `logs_sender.payloads_dropped` | `reliable`, `destination` | Payloads dropped (buffer full or all reliable destinations blocked at shutdown). |
| `logs_sender.messages_dropped` | `reliable`, `destination` | Individual messages in dropped payloads. |
| `logs_sender.send_wait` | — | Cumulative ms spent waiting for destination sends to complete. |
| `logs_sender_batch_strategy.dropped_too_large` | `pipeline` | Messages dropped because a single message exceeds `maxContentSize`. |

#### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `DefaultWorkersPerQueue` | `1` | Workers per queue in the default (non-legacy) topology. |
| `DefaultQueuesCount` | `1` | Queues in the default topology. |

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; the sender is the penultimate stage before the network destination |
| [pipeline.md](pipeline.md) | `Pipeline` creates the `Sender` (via `sender/http` or `sender/tcp`) and wires the strategy output to `Sender.In()` |
| [message.md](message.md) | `*message.Payload` (output of `Strategy`) and `*message.MessageMetadata` are the types the sender operates on |
| [client.md](client.md) | The `Destination` implementations (`http.Destination`, `tcp.Destination`) that each `worker` calls to deliver payloads |
| [comp/forwarder/eventplatform.md](../../comp/forwarder/eventplatform.md) | Uses `sender/http.NewHTTPSender` directly for each event-platform pipeline (DBM, CSPM, NetFlow, …) |

## Usage

`Sender` is never instantiated directly by callers. The `pipeline.Provider` creates it via `sender/http.NewHTTPSender` or `sender/tcp.NewTCPSender` depending on `endpoints.UseHTTP`:

```go
// pkg/logs/pipeline/provider.go
if endpoints.UseHTTP {
    senderImpl = httpSender(numberOfPipelines, cfg, sink, endpoints, ...)
} else {
    senderImpl = tcpSender(numberOfPipelines, cfg, sink, endpoints, ...)
}
```

The `epforwarder` (`comp/forwarder/eventplatform/eventplatformimpl`) creates HTTP senders directly for each event-platform pipeline (DBM, network paths, …):

```go
httpsender.NewHTTPSender(cfg, sink, bufferSize, serverlessMeta, endpoints,
    destinationsCtx, componentName, contentType, evpCategory,
    queueCount, workersPerQueue, minConcurrency, maxConcurrency)
```

The OTel logs pipeline and security/compliance reporters also use `NewProvider` which internally builds the appropriate sender.

To test pipeline components in isolation, use `sender_mock.go` (`SenderMock`) which implements `PipelineComponent` and captures payloads in memory.

### Reliable vs unreliable destinations

A `Destinations` object (from `pkg/logs/client`) carries two destination slices:

- **Reliable** — the primary intake endpoint. The `worker` performs a blocking `Send()` and retries every 100 ms when the destination is in retry mode. At least one reliable destination must acknowledge the payload before the pipeline continues.
- **Unreliable** — additional endpoints (e.g. a secondary region or a Vector proxy). The `worker` uses a non-blocking `NonBlockingSend()`. Full buffers cause payload drops counted in `logs_sender.payloads_dropped` (with `reliable:false` tag) but do not stall the pipeline.

### MRF routing

MRF (Multi-Region Failover) destinations are a special category of reliable destination. `DestinationSender.canSend()` reads `multi_region_failover.enabled` and `multi_region_failover.failover_logs` at send time to decide whether to route a payload to an MRF destination. Payloads are only routed to MRF destinations when `Payload.IsMRF()` returns `true` (set by the processor; see [processor.md](processor.md)).

### Adaptive concurrency (HTTP)

`sender/http.NewHTTPSender` creates an `http.Destination` with `minConcurrency` and `maxConcurrency` parameters. Internally, the `workerPool` inside `http.Destination` uses an EWMA of observed send latency to scale the number of concurrent HTTP workers between those bounds. Target latency is 150 ms. On a retryable error the pool backs off to `minWorkers` immediately. For more detail see [client.md](client.md).
