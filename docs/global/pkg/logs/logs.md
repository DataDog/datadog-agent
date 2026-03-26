# pkg/logs

## Purpose

`pkg/logs` implements the logs-agent: the subsystem responsible for collecting log lines from
files, containers, journald, Windows Event Log, TCP/UDP sockets, and other sources, then
processing and forwarding them to the Datadog intake.

The package is organized into two logical halves:

- **What to log** — schedulers and launchers discover and register log sources; launchers create
  tailers that feed raw bytes into decoders.
- **How to log** — pipelines (processor → strategy → sender → destination) transform, encode, and
  deliver messages to the intake, then report successful delivery back to tailers via the auditor.

```
Autodiscovery
     │ integration.Config
     ▼
┌─ Schedulers ─────────────────────┐
│  Scheduler  │  ad.Scheduler  │ … │  →  LogSources / Services stores
└────────────────────────────────-─┘
                    │
                    ▼
┌─ Launchers ───────────────────────────────────────┐
│  file.Launcher  docker.Launcher  k8s.Launcher  … │
└───────────────────────────────────────────────────┘
                    │
                    ▼
             Tailers + Decoders  (one per source)
                    │  *message.Message
                    ▼
┌─ Pipeline (processor → strategy → sender → destination) ─┐
│                                                           │
│  Processor  ──►  Strategy  ──►  Sender  ──►  Destination ─► Datadog intake
│                                                           │
└───────────────────────────────────────────────────────────┘
                    │
                    ▼
                 Auditor  (notifies tailers of delivered offsets)
```

### Sub-packages

| Sub-package | Responsibility |
|---|---|
| `sources` | `LogSource` and `LogSources` — the live registry of active sources |
| `service` | `Service` — container handle used for `container_collect_all` |
| `schedulers` | `Scheduler` interface + `Schedulers` manager; built-in schedulers live under `schedulers/ad`, `schedulers/channel` |
| `launchers` | `Launcher` interface + `Launchers` manager; concrete launchers live under `launchers/file`, `launchers/container`, `launchers/listener`, etc. |
| `pipeline` | `Pipeline`, `Provider` — process/encode/send pipeline instances |
| `processor` | `Processor`, `Encoder` — per-message filtering, redaction, hostname injection, encoding |
| `sender` | `Sender`, `Strategy` (`BatchStrategy`, `StreamStrategy`) — payload batching and delivery queues |
| `client` | HTTP and TCP destination implementations |
| `message` | Core data types (`Message`, `Payload`, `Origin`) — see [message.md](message.md) |
| `internal/decoder` | `Decoder` — byte-stream → `message.Message` framing and multiline assembly |
| `internal/framer` | Low-level frame delimiters (newline, docker, etc.) |
| `internal/parsers` | `Parser` interface plus docker, journald, noop parsers |
| `internal/tailers` | File, journald, socket, Windows Event tailer implementations |
| `diagnostic` | `MessageReceiver` — in-memory message inspector for `agent stream-logs` |
| `metrics` | Internal Dogstatsd and telemetry counters for the logs pipeline |
| `status` | Status page data structures for the logs agent |
| `tailers` | `TailerTracker` — lifecycle registry shared across all launchers |
| `types` | Shared primitive types (`Fingerprint`, etc.) |
| `util` | Misc utilities (framing helpers, tailer randomization, …) |

## Key elements

### `sources.LogSource`

The unit of log collection configuration. Created by schedulers and consumed by launchers.

```go
type LogSource struct {
    Name         string
    Config       *config.LogsConfig  // type, path/port, service, source, tags, processing rules, …
    Status       *status.LogStatus
    ParentSource *LogSource          // set when a source is derived from another (e.g. container_collect_all)
    BytesRead    *status.CountInfo
    ProcessingInfo *status.ProcessingInfo
    LatencyStats *statstracker.Tracker
    // ...
}

func NewLogSource(name string, cfg *config.LogsConfig) *LogSource
```

`Config.Type` controls which launcher picks up the source (e.g. `"file"`, `"docker"`,
`"journald"`, `"tcp"`, `"udp"`, `"windows_event"`).

### `sources.LogSources`

Thread-safe pub/sub registry. Schedulers call `AddSource`/`RemoveSource`; launchers subscribe
with `SubscribeForType` or `SubscribeAll` and receive channels of `*LogSource`.

```go
func (s *LogSources) AddSource(source *LogSource)
func (s *LogSources) RemoveSource(source *LogSource)
func (s *LogSources) SubscribeForType(sourceType string) (added, removed chan *LogSource)
func (s *LogSources) SubscribeAll() (added, removed chan *LogSource)
func (s *LogSources) GetAddedForType(sourceType string) chan *LogSource
```

### `schedulers.Scheduler` interface

```go
type Scheduler interface {
    Start(sourceMgr SourceManager)
    Stop()
}
```

`SourceManager` provides `AddSource`, `RemoveSource`, `AddService`, `RemoveService` to
schedulers. The most important built-in implementation is the Autodiscovery scheduler in
`pkg/logs/schedulers/ad`.

### `launchers.Launcher` interface

```go
type Launcher interface {
    Start(sourceProvider SourceProvider, pipelineProvider pipeline.Provider,
          registry auditor.Registry, tracker *tailers.TailerTracker)
    Stop()
}
```

`SourceProvider` exposes `SubscribeForType`, `SubscribeAll`, and `GetAddedForType`.

### `pipeline.Provider` interface

Entry point used by tailers to obtain a pipeline channel.

```go
type Provider interface {
    Start()
    Stop()
    NextPipelineChan() chan *message.Message
    NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor)
    Flush(ctx context.Context)
}
```

`NextPipelineChan` returns a round-robin channel across all running pipelines. Tailers write
`*message.Message` values directly to this channel.

### `pipeline.Pipeline`

A single pipeline instance: `Processor → Strategy → Sender → Destination`.

```go
type Pipeline struct {
    InputChan chan *message.Message
    // ...
}

func NewPipeline(...) *Pipeline
func (p *Pipeline) Start()
func (p *Pipeline) Stop()
func (p *Pipeline) Flush(ctx context.Context)
```

The encoder chosen at construction time depends on the transport:
- `processor.JSONEncoder` — HTTP transport (default)
- `processor.JSONServerlessInitEncoder` — serverless
- `processor.ProtoEncoder` — protobuf/HTTP
- `processor.RawEncoder` — TCP

### `processor.Processor`

Runs on each pipeline goroutine. For every `*message.Message` it:
1. Applies `config.ProcessingRule`s (exclude, include, mask/redact, exclude-truncated).
2. Calls `msg.Render()` to collapse structured content to bytes.
3. Optionally tags the message for Multi-Region Failover (MRF).
4. Calls `Encoder.Encode` (in-place, transitions message to `StateEncoded`).
5. Sends the encoded message to the strategy channel.

```go
type Encoder interface {
    Encode(msg *message.Message, hostname string) error
}
```

Processing rules are sourced from both the global `logs_config.processing_rules` and per-source
`LogsConfig.ProcessingRules`.

### `sender.Strategy`

Converts a stream of encoded `*message.Message` values into `*message.Payload` batches.

- `BatchStrategy` — collects messages up to `BatchMaxSize`/`BatchMaxContentSize` or `BatchWait`,
  then compresses and flushes; used for HTTP.
- `StreamStrategy` — one message per payload; used for TCP.

### `sender.Sender`

Distributes `*message.Payload` values across one or more `worker` goroutines, each of which
calls a `client.Destination` to deliver the payload to the intake.

### Pipeline failover (`logs_config.pipeline_failover.enabled`)

When enabled, the provider inserts a router channel between tailers and pipelines. A forwarder
goroutine tries non-blocking sends to all pipelines in order before falling back to a blocking
send on the primary. This prevents a single blocked pipeline from stalling all tailers.

### Multi-Region Failover (MRF)

Controlled by `multi_region_failover.failover_logs` and
`multi_region_failover.logs_service_allowlist`. When active, the processor sets
`msg.IsMRFAllow = true` on matching messages, and the sender routes those payloads to MRF
destinations.

## Related documentation

| Document | Relationship |
|---|---|
| [message.md](message.md) | Core data types (`Message`, `Payload`, `Origin`) shared by every pipeline stage |
| [sources.md](sources.md) | `LogSource` and `LogSources` — the registry that schedulers write to and launchers read from |
| [schedulers.md](schedulers.md) | `Scheduler` interface and built-in schedulers (`ad/`, `channel/`) that populate `LogSources` |
| [launchers.md](launchers.md) | `Launcher` interface and all concrete launchers (file, container, journald, Windows Event, channel, integration) |
| [tailers.md](tailers.md) | Tailer implementations and `TailerTracker`; called by launchers, write to pipeline channels |
| [pipeline.md](pipeline.md) | `Pipeline` and `Provider` — orchestrates processor → strategy → sender for each pipeline lane |
| [processor.md](processor.md) | `Processor` and `Encoder` — applies processing rules and serializes messages |
| [sender.md](sender.md) | `Sender` and `Strategy` (`BatchStrategy`, `StreamStrategy`) — batches and delivers payloads |
| [client.md](client.md) | HTTP and TCP `Destination` implementations consumed by the sender |
| [comp/logs/agent.md](../../comp/logs/agent.md) | Top-level fx component that wires all of the above together at agent startup |

## Usage

### Adding a new launcher

1. Implement `launchers.Launcher`.
2. Register it via `Launchers.AddLauncher(myLauncher)` in the agent startup sequence (typically
   in `comp/logs/agent`).
3. In `Start`, call `sourceProvider.SubscribeForType("my_type")` or `SubscribeAll()` and spawn a
   goroutine that ranges over the added channel, creating tailers as sources arrive.
4. Write `*message.Message` values to `pipelineProvider.NextPipelineChan()`.

### Adding a new scheduler

1. Implement `schedulers.Scheduler`.
2. Register it via `Schedulers.AddScheduler(myScheduler)`.
3. In `Start`, call `sourceMgr.AddSource(src)` as new log configurations are discovered, and
   `sourceMgr.RemoveSource(src)` when they are removed.

### Sending a log message from a tailer

```go
// Obtain a pipeline channel (round-robin across running pipelines)
ch := pipelineProvider.NextPipelineChan()

// Construct a message (unstructured, file tailer example)
msg := message.NewMessageWithSource(
    []byte("my log line"),
    message.StatusInfo,
    logSource,
    time.Now().UnixNano(),
)

ch <- msg
```

### Inspecting messages live (`stream-logs`)

The `diagnostic.MessageReceiver` interface receives a copy of every rendered message just before
encoding. The CLI `agent stream-logs` command uses the implementation in `pkg/logs/diagnostic` to
surface live messages without altering the pipeline.

### End-to-end data flow summary

The following is a concrete walk-through from configuration discovery to network delivery:

1. `comp/logs/agent` starts and calls `schedulers.NewSchedulers`, registering the Autodiscovery scheduler (`schedulers/ad`) and any channel schedulers. See [schedulers.md](schedulers.md).
2. The AD scheduler calls `CreateSources` and `AddSource` on the `LogSources` registry as container labels and integration configs are discovered. See [sources.md](sources.md).
3. Launchers (file, container, journald, etc.) receive `*LogSource` values from their subscribed `LogSources` channels. Each launcher creates one or more tailers. See [launchers.md](launchers.md) and [tailers.md](tailers.md).
4. Each tailer constructs `*message.Message` values and writes them to a `pipeline.Provider` channel (round-robin across pipelines). See [message.md](message.md).
5. Each `Pipeline` runs a `Processor` that applies processing rules, renders structured content, and encodes the message (JSON/proto/raw). See [processor.md](processor.md).
6. The encoded message is passed to a `Strategy` (`BatchStrategy` or `StreamStrategy`) that assembles it into a `*message.Payload`. See [sender.md](sender.md).
7. The `Sender` delivers the `Payload` to one or more `client.Destination` (HTTP or TCP). See [client.md](client.md).
8. After delivery, the auditor (via `Sink`) records the offset so tailing can resume after a restart.

### Relevant config keys

| Key | Purpose |
|---|---|
| `logs_config.enabled` | Enable/disable the logs agent |
| `logs_config.message_channel_size` | Buffering depth of per-pipeline channels |
| `logs_config.payload_channel_size` | Buffering depth of the sender payload channel |
| `logs_config.batch_wait` | Max seconds before flushing a partial HTTP batch |
| `logs_config.batch_max_size` | Max messages per HTTP batch |
| `logs_config.batch_max_content_size` | Max uncompressed bytes per HTTP batch |
| `logs_config.batch_max_concurrent_send` | HTTP destination concurrency (legacy) |
| `logs_config.pipeline_failover.enabled` | Enable pipeline failover routing |
| `multi_region_failover.failover_logs` | Enable MRF for logs |
| `multi_region_failover.logs_service_allowlist` | Services to route to MRF |
