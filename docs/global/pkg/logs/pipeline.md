# pkg/logs/pipeline

## Purpose

`pkg/logs/pipeline` orchestrates the processing and forwarding of log messages from the moment they are decoded by a tailer/listener to the moment they are handed to the network sender. It owns two internal stages:

1. **Processor** (`pkg/logs/processor`) — applies processing rules (redaction, multiline, filtering), encodes messages (JSON, proto, or raw), and feeds them into a strategy.
2. **Strategy** (`sender.Strategy`) — groups messages into payloads according to the transport (batch for HTTP, stream for TCP) and pushes them to the `Sender`.

The package exposes a `Provider` interface so the rest of the agent does not need to know how many pipelines exist or which transport is in use.

## Key elements

### `Pipeline`

```go
type Pipeline struct {
    InputChan chan *message.Message
    // private: processor, strategy, pipelineMonitor
}
```

Represents one processing lane. Each `Pipeline` has its own `InputChan` (buffered, size `logs_config.message_channel_size`). Tailers write to this channel. The pipeline runs the processor and strategy concurrently; `Start()`/`Stop()` manage the lifecycle. `Flush(ctx)` is a synchronous flush used in serverless mode.

`NewPipeline` selects the encoder and strategy automatically:

| Condition | Encoder | Strategy |
|-----------|---------|----------|
| `serverless` | `JSONServerlessInitEncoder` | `BatchStrategy` |
| `endpoints.UseHTTP` | `JSONEncoder` | `BatchStrategy` |
| `endpoints.UseProto` | `ProtoEncoder` | `StreamStrategy` |
| default (TCP) | `RawEncoder` | `StreamStrategy` |

Compression for `BatchStrategy` is configured from `endpoints.Main.UseCompression`, `CompressionKind`, and `CompressionLevel`.

### `Provider` interface

```go
type Provider interface {
    Start()
    Stop()
    NextPipelineChan() chan *message.Message
    NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor)
    GetOutputChan() chan *message.Message
    Flush(ctx context.Context)
}
```

Launchers call `NextPipelineChan()` (or `NextPipelineChanWithMonitor()` when they track capacity) to obtain a channel they can write messages to. The provider round-robins across its internal pipelines.

### `provider` (concrete implementation)

Created by `NewProvider`. Key constructor parameters:

| Parameter | Description |
|-----------|-------------|
| `numberOfPipelines` | Number of parallel `Pipeline` instances. |
| `endpoints` | Selects HTTP vs TCP transport, batch settings, compression. |
| `processingRules` | Global processing rules applied in every pipeline. |
| `legacyMode` | When `true`, reverts to one queue per pipeline (older concurrency model). |
| `serverless` | Activates serverless-specific flush synchronisation. |

The provider creates a `Sender` (HTTP or TCP) and a set of `Pipeline` instances that all share the same `Sender`.

**Pipeline failover** (`logs_config.pipeline_failover.enabled`): when enabled, the provider adds a layer of router channels and forwarder goroutines. A message is first sent non-blocking to its primary pipeline; if that pipeline is full, the forwarder tries other pipelines before blocking. This allows one stalled pipeline to shed load to its siblings.

### `processorOnlyProvider`

A lightweight `Provider` variant that runs only the processor stage with no network sender. Used by the `analyzelogs` subcommand to process log lines locally and inspect them without sending to Datadog.

```go
pipeline.NewProcessorOnlyProvider(diagnosticReceiver, processingRules, hostname, cfg)
```

`GetOutputChan()` returns the channel where processed messages appear (instead of going to a sender).

### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `maxConcurrencyPerPipeline` | `10` | Upper bound on HTTP destination concurrency per pipeline (not user-configurable directly; use `BatchMaxConcurrentSend` on the endpoint). |

### Config keys

| Key | Default | Description |
|-----|---------|-------------|
| `logs_config.message_channel_size` | — | Buffer depth of each pipeline's `InputChan` and strategy input channel. |
| `logs_config.pipeline_failover.enabled` | `false` | Enables the cross-pipeline failover router. |
| `logs_config.pipeline_failover.router_channel_size` | — | Buffer depth of the failover router channels. |

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; `pipeline.Provider` is the entry point that tailers write decoded messages to |
| [message.md](message.md) | `*message.Message` and `*message.Payload` are the types flowing through the pipeline |
| [processor.md](processor.md) | `Processor` runs inside each `Pipeline` lane; handles filtering, rendering, and encoding |
| [sender.md](sender.md) | `Sender` and `Strategy` are created by the provider and wired to the processor output |
| [client.md](client.md) | HTTP/TCP `Destination` implementations that the sender dispatches payloads to |
| [comp/serializer/logscompression.md](../../comp/serializer/logscompression.md) | `logscompression.Component` is injected into `NewProvider` and used to create per-pipeline compressors for `BatchStrategy` |
| [comp/logs/agent.md](../../comp/logs/agent.md) | Calls `pipeline.NewProvider` at startup and exposes `GetPipelineProvider()` to launchers |

## Usage

`Provider` is created once per logs agent instance in `comp/logs/agent/agentimpl/agent.go` via `pipeline.NewProvider(...)` and stored as `a.pipelineProvider`. The instance is also accessible through `GetPipelineProvider()`.

Launchers obtain a write channel from the provider:

```go
// Example: file launcher
inputChan, monitor := pipelineProvider.NextPipelineChanWithMonitor()
// tailer writes decoded messages to inputChan
```

The `epforwarder` (`comp/forwarder/eventplatform`) uses `NewProvider` independently to create its own pipeline set for event-platform data (DBM, CSPM, network paths, …).

The OTel logs exporter (`comp/otelcol/logsagentpipeline`) also constructs a `Provider` directly to process OpenTelemetry log records through the same pipeline infrastructure.

The security and compliance reporters (`pkg/security/reporter`, `pkg/compliance/reporter`) instantiate a `Provider` to forward security-related log events.

### Encoder and strategy selection

`NewPipeline` selects the encoder and strategy automatically based on `endpoints`:

| Condition | Encoder (processor.go) | Strategy (sender.go) |
|-----------|------------------------|----------------------|
| `serverless == true` | `JSONServerlessInitEncoder` | `BatchStrategy` |
| `endpoints.UseHTTP == true` | `JSONEncoder` | `BatchStrategy` |
| `endpoints.UseProto == true` | `ProtoEncoder` | `StreamStrategy` |
| default (TCP) | `RawEncoder` | `StreamStrategy` |

See [processor.md](processor.md) for encoder wire formats and [sender.md](sender.md) for strategy behavior.

### Compression

`BatchStrategy` receives a `compression.Compressor` created by `logscompression.Component.NewCompressor(kind, level)`. Each pipeline instance gets its own compressor, so the main endpoint and any additional endpoints (e.g. a Vector proxy) may use different algorithms without shared state. See [comp/serializer/logscompression.md](../../comp/serializer/logscompression.md).

### Transport restart

When `comp/logs/agent` detects a connectivity transition (TCP → HTTP), it tears down and recreates the `destinationsCtx`, `pipelineProvider`, and launchers without touching the auditor, schedulers, or `LogSources`. The `Provider.Stop()` → `Provider.Start()` sequence drains in-flight messages before the new provider starts. Tailers detect the new provider through the launcher restart and obtain a fresh channel from the new provider.
