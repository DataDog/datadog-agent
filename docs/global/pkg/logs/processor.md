# pkg/logs/processor

## Purpose

`pkg/logs/processor` transforms decoded `*message.Message` values before they are handed to the sending layer. It sits between tailers and the `sender.Strategy` in each pipeline. Its responsibilities are:

1. **Filtering** — drop messages that match (or do not match) user-defined patterns.
2. **Redaction** — replace sensitive substrings in message content.
3. **Encoding** — serialize the message into the wire format expected by the Datadog intake (JSON, protobuf, or RFC 5424 syslog).
4. **MRF tagging** — mark messages for Multi-Region Failover routing when enabled.
5. **Diagnostics** — forward rendered messages to in-process diagnostic receivers (used by `agent stream-logs`).

The processor runs in a single goroutine and is wired into each `Pipeline` instance. Multiple processor instances run in parallel across pipelines, but each processes its own `inputChan` sequentially.

## Key elements

### `Processor`

```go
type Processor struct {
    inputChan       chan *message.Message
    outputChan      chan *message.Message  // strategy input
    processingRules []*config.ProcessingRule
    encoder         Encoder
    // ...
}

func New(config pkgconfigmodel.Reader, inputChan, outputChan chan *message.Message,
    processingRules []*config.ProcessingRule, encoder Encoder,
    diagnosticMessageReceiver diagnostic.MessageReceiver,
    hostname hostnameinterface.Component,
    pipelineMonitor metrics.PipelineMonitor, instanceID string) *Processor

func (p *Processor) Start()
func (p *Processor) Stop()
func (p *Processor) Flush(ctx context.Context)
```

`Start` spawns a goroutine running `run()`. `Stop` closes `inputChan` and blocks until the goroutine exits (after draining the channel). `Flush` is a synchronous drain used in the serverless agent.

For each message `processMessage` is called, which:
1. Applies all processing rules via `applyRedactingRules`. If the message is dropped, it is not forwarded.
2. Calls `msg.Render()` to produce the final string content.
3. Reports the message to the diagnostic receiver.
4. Optionally calls `filterMRFMessages` when MRF failover is active.
5. Encodes the message in-place via `encoder.Encode(msg, hostname)`.
6. Writes to `outputChan`.

**Config-driven failover**: the processor registers an `OnUpdate` callback with the config system. When `multi_region_failover.failover_logs` or `multi_region_failover.logs_service_allowlist` changes, the new configuration is sent through an internal `configChan` and applied on the next iteration of the run loop. This avoids locking during message processing.

### `Encoder` interface

```go
type Encoder interface {
    Encode(msg *message.Message, hostname string) error
}
```

Three implementations are provided:

| Variable | Type | Wire format | Used when |
|----------|------|-------------|-----------|
| `JSONEncoder` | `jsonEncoder` | JSON object | `endpoints.UseHTTP == true` |
| `ProtoEncoder` | `protoEncoder` | Protobuf (`pb.Log`) | `endpoints.UseProto == true` |
| `RawEncoder` | `rawEncoder` | RFC 5424 syslog | TCP (default) |
| `JSONServerlessInitEncoder` | `jsonServerlessInitEncoder` | JSON (serverless variant) | `serverless == true` |

`jsonEncoder` produces:
```json
{"message":"…","status":"info","timestamp":1234567890,"hostname":"h","service":"s","ddsource":"src","ddtags":"t1:v1,t2:v2"}
```

`rawEncoder` wraps the content as RFC 5424 unless it is already RFC 5424-formatted (detection: `<NNN>N ` prefix pattern).

`protoEncoder` serializes to `agent-payload/v5/pb.Log`.

`ValidUtf8Bytes` is a `[]byte` wrapper whose `MarshalText` replaces invalid UTF-8 runes with `utf8.RuneError`; it is used by the JSON encoder so invalid bytes do not cause marshalling to fail.

### Processing rules (`comp/logs/agent/config.ProcessingRule`)

Processing rules are defined in `comp/logs/agent/config` and are referenced and applied by the processor. The processor merges global rules (passed at construction) with per-source rules (`msg.Origin.LogSource.Config.ProcessingRules`) before evaluating them.

| Type constant | Effect |
|---------------|--------|
| `ExcludeAtMatch` | Drop message if regex matches content. |
| `IncludeAtMatch` | Drop message if regex does not match content. |
| `MaskSequences` | Replace matching substrings with `ReplacePlaceholder`. |
| `ExcludeTruncated` | Drop message if it was truncated by the decoder. |
| `MultiLine` | (Consumed by the decoder, not the processor.) |

For `MaskSequences`, the processor uses `isMatchingLiteralPrefix` to short-circuit evaluation: if the regex has a literal prefix and the content does not contain it, the replacement is skipped without running the full regex.

### MRF (Multi-Region Failover)

When `multi_region_failover.failover_logs` is `true`, `filterMRFMessages` sets `msg.IsMRFAllow = true` on messages that should be routed to MRF destinations. If `multi_region_failover.logs_service_allowlist` is set, only messages whose `Origin.Service()` appears in the allowlist are tagged. An empty allowlist means all messages are tagged.

### Telemetry

| Metric | Description |
|--------|-------------|
| `datadog.logs_agent.tailer.unstructured_processing` | Emitted by tailers (journald, Windows event) when processing rules are applied to raw structured content rather than the parsed message field. |
| `LogsDecoded` / `TlmLogsDecoded` | Incremented for each message entering the processor. |
| `LogsProcessed` / `TlmLogsProcessed` | Incremented for each message that passes filtering and is forwarded. |
| `TlmTruncatedCount` | Incremented for each truncated message, labelled by service and source. |

The `UnstructuredProcessingMetricName` constant (`"datadog.logs_agent.tailer.unstructured_processing"`) is also used by the journald and Windows event tailers to report when unstructured processing mode is active.

### `processorOnlyProvider` (`pipeline/processor_only_provider.go`)

Not in this package, but closely related: a `Provider` implementation that wraps only a `Processor` with no sender. Used by `cmd/agent/subcommands/analyzelogs` to process logs locally and inspect the output.

## Usage

A `Processor` is created inside `pipeline.NewPipeline`:

```go
encoder := processor.JSONEncoder  // chosen based on endpoint config
p := processor.New(cfg, inputChan, strategyInput, processingRules,
    encoder, diagnosticReceiver, hostname, pipelineMonitor, instanceID)
p.Start()
// ...
p.Stop()
```

Global processing rules come from the agent configuration (`logs_config.processing_rules`). Per-source rules are attached to `LogSource.Config.ProcessingRules` and are picked up automatically during message processing; no additional wiring is needed.

To add a new processing rule type: add a constant in `comp/logs/agent/config/processing_rules.go`, validate and compile it in `ValidateProcessingRules` / `CompileProcessingRules`, then add the corresponding case in `processor.applyRedactingRules`.

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; the processor is the first stage of each `Pipeline` lane, sitting between the tailer-written channel and the strategy/sender |
| [pipeline.md](pipeline.md) | `pipeline.NewPipeline` creates and wires the `Processor`; the encoder and strategy are selected there based on transport type |
| [message.md](message.md) | The processor drives the `MessageContent` state machine: `msg.Render()` → `StateRendered`, then `encoder.Encode()` → `StateEncoded`; `GetContent`/`SetContent` are the safe accessors used during redaction |
| [sources.md](sources.md) | Per-source processing rules are read from `LogSource.Config.ProcessingRules`; the processor merges them with global rules at message-processing time |
| [../../pkg/redact.md](../../pkg/redact.md) | `pkg/redact` is a separate scrubber for Kubernetes/process collector data (env vars, CLI args, CRD manifests); the logs processor's own redaction uses `MaskSequences` processing rules and operates on raw log-line content, not structured Kubernetes objects |
| [diagnostic.md](diagnostic.md) | `diagnostic.MessageReceiver` receives a copy of each rendered message inside the processor, before encoding; used by `agent stream-logs` to surface live log content |
