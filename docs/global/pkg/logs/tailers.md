# pkg/logs/tailers

## Purpose

`pkg/logs/tailers` contains the tailer implementations and the shared tracking infrastructure used across all source types. A tailer is responsible for reading raw log data from a single source (a file, a container stream, a journal, a socket connection, or an in-process channel) and forwarding decoded `*message.Message` values to a pipeline channel. Launchers create and manage tailers; each tailer is created with a dedicated output channel obtained from `pipeline.Provider`.

## Key elements

### `Tailer` interface (`tailer.go`)

```go
type Tailer interface {
    GetID() string
    GetType() string
    GetInfo() *status.InfoRegistry
}
```

Minimal interface shared by all concrete tailer types. `GetID` returns a string that uniquely identifies the tailer (e.g. `"docker:<containerID>"` or a file path). `GetInfo` returns a registry of status key/value pairs surfaced in `agent status`.

### `TailerTracker` (`tailer_tracker.go`)

```go
type TailerTracker struct { /* sync.RWMutex + containers */ }

func NewTailerTracker() *TailerTracker
func (t *TailerTracker) Add(container AnyTailerContainer)
func (t *TailerTracker) All() []Tailer
```

Global registry of all active tailers in the agent. Each launcher registers its own `TailerContainer` at `Start` time. The tracker is passed to every launcher by the `Launchers` collection and is the source of truth for the `agent status` tailers view.

### `TailerContainer[T Tailer]` (`tailer_tracker.go`)

```go
type TailerContainer[T Tailer] struct { /* sync.RWMutex + map[string]T */ }

func NewTailerContainer[T Tailer]() *TailerContainer[T]
func (t *TailerContainer[T]) Add(tailer T)
func (t *TailerContainer[T]) Remove(tailer T)
func (t *TailerContainer[T]) Get(id string) (T, bool)
func (t *TailerContainer[T]) Contains(id string) bool
func (t *TailerContainer[T]) All() []T
func (t *TailerContainer[T]) Count() int
```

A type-safe, concurrency-safe map of tailers keyed by `GetID()`. Each launcher that manages a homogeneous set of tailers creates one `TailerContainer` for its concrete type and registers it with the `TailerTracker`. This lets the tracker aggregate all tailers without losing concrete type information inside the launcher.

### Sub-packages

#### `file/` — file tailer (all platforms)

**`Tailer`** polls a single file for new data, decodes the content, and forwards messages. Internal pipeline:

```
readForever → decoder (goroutine) → forwardMessages → outputChan
```

- `readForever` polls the file with `tailerSleepDuration` (default 1 s) between empty reads.
- The `decoder` (`pkg/logs/internal/decoder`) handles line framing, multiline aggregation, and charset conversion.
- `forwardMessages` translates decoded messages and writes them to `outputChan`.
- `Start(offset, whence)` seeks to the saved position from the auditor registry before reading begins.
- `StopAfterFileRotation()` lets the tailer drain its current file before stopping, while the launcher immediately starts a fresh tailer on the new file.
- Platform differences: UNIX uses `closeTimeout` to keep the old inode alive after rotation; Windows uses `windowsOpenFileTimeout` to hold the file open while the pipeline drains.

**`File`** (`file.go`) is a value object wrapping a `sources.LogSource` and a path. `GetScanKey()` returns a unique key used by the file launcher's `TailerContainer` (includes container ID when applicable, to allow two tailers on the same path for overlapping containers).

**`Fingerprinter`** interface (`fingerprint.go`) computes a CRC64 checksum of the first N bytes or N lines of a file. Used to detect rotation when inode comparison is insufficient (e.g. on some network filesystems or Windows).
- `ShouldFileFingerprint(file)` — whether fingerprinting is enabled for this file.
- `ComputeFingerprint(file)` — compute and return the fingerprint.
- Strategies: `FingerprintStrategyByteChecksum` (first N bytes), `FingerprintStrategyLineChecksum` (first N lines, up to `MaxBytes` each).

#### `container/` — container tailer (build tags: `kubelet || docker`)

**`Tailer`** streams logs from a running container. The same struct supports two log readers via `messageForwarder`:
- **Docker socket** (`NewDockerTailer`): calls `docker.ContainerLogs` with `Follow:true`.
- **Kubelet API** (`NewAPITailer`): calls `kubelet.StreamLogs` with `Follow:true`.

Internal pipeline:

```
readForever → decoder (dockerstream framer) → messageForwarder → outputChan
```

The `dockerstream` parser demultiplexes the Docker multiplexed stream format (`[SEV][TS][MSG]`). The Kubelet forwarder adds an extra timestamp deduplication step to compensate for the Kubelet API's sub-second granularity limitation (see code comment in the source for the full timeline).

Key fields: `ContainerID`, `lastSince` (RFC3339 timestamp of the last forwarded message, used as the `since` parameter on reader restart).

Reconnection: if the reader returns `io.EOF` or a read timeout (`context.Canceled`), `tryRestartReader` retries. Persistent errors send the container ID to `erroredContainerID` so the launcher can reschedule the tailer.

#### `journald/` — journald tailer (build tag: `systemd`)

**`Tailer`** reads entries from a `Journal` interface (wrapping `go-systemd/sdjournal`). After each entry it calls `journal.Wait(defaultWaitDuration)` to block until new entries arrive. Cursor position is persisted to the auditor registry.

`processRawMessage` mode: when `true`, the whole structured JSON journal entry is forwarded as the message content (legacy behavior). When `false`, only `MESSAGE` field is forwarded. A warning is emitted when processing rules are combined with `processRawMessage: true`.

Filtering: supports filtering by systemd unit (`Config.IncludeSystemUnits` / `Config.ExcludeSystemUnits`) and by arbitrary journal field matches.

#### `socket/` — socket tailer (all platforms)

**`Tailer`** reads from a `net.Conn`. The `read` field is a callback injected by the listener, making the tailer generic over connection type (TCP vs UDP have different read semantics). If `logs_config.use_sourcehost_tag` is true, a `source_host:<ip>` tag is added to each message.

#### `windowsevent/` — Windows Event Log tailer (build tag: `windows`)

**`Tailer`** subscribes to a Windows Event Log channel using a pull subscription (`evtsubscribe.PullSubscription`). Bookmark position is stored in the auditor registry as XML. On restart, the subscription is initialized from the last saved bookmark.

**`Config`** holds the channel path, an XPath query (defaults to `*`), and `ProcessRawMessage`.

A `publishermetadatacache.Component` is used to resolve provider display names for event message rendering.

`Identifier(channelPath, query)` generates the unique key for this tailer stored in the auditor.

#### `channel/` — channel tailer (all platforms)

**`Tailer`** consumes `*config.ChannelMessage` values from an in-process Go channel (`source.Config.Channel`) and emits `*message.Message` values. Attaches `ChannelTags` from the source config (mutex-protected for dynamic updates). `IsError` on the input message sets the log status to `error`.

Used for: serverless agent log ingestion, OTel collector log export, integration check log forwarding.

`WaitFlush()` closes the input channel and blocks until the run goroutine drains it — this is the shutdown path called by the channel launcher.

## Usage

Tailers are created exclusively by their corresponding launchers. The typical pattern is:

```go
// In a launcher:
container := tailers.NewTailerContainer[*file.Tailer]()
tracker.Add(container)  // register with global tracker

// On source added:
t := file.NewTailer(options)
t.Start(offset, whence)
container.Add(t)

// On source removed / rotation:
t.Stop() // or StopAfterFileRotation()
container.Remove(t)
```

Launchers do not share tailers between each other. Each tailer writes to exactly one pipeline channel returned by `pipelineProvider.NextPipelineChan()` or `NextPipelineChanWithMonitor()`. The channel remains the tailer's property for its lifetime and is not reused.

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; shows where tailers sit in the full data-flow (launchers → tailers + decoders → pipeline) |
| [launchers.md](launchers.md) | Launchers are the exclusive creators and owners of tailers; they also register each `TailerContainer` with the `TailerTracker` and call `Start`/`Stop` on individual tailers |
| [sources.md](sources.md) | `LogSource` and `ReplaceableSource` — each tailer holds a reference to the source it is collecting from; the file tailer holds a `ReplaceableSource` to support in-place source swaps during container metadata updates |
| [message.md](message.md) | `*message.Message` is the output type of every tailer; the `Origin` embedded in each message links it back to its `LogSource` and carries the offset consumed by the auditor |
| [internal.md](internal.md) | The `pkg/logs/internal/decoder` pipeline (Framer → LineParser → Preprocessor) is created and driven by the file and socket tailers; decoders transform raw bytes into `*message.Message` values |
| [pipeline.md](pipeline.md) | `pipeline.Provider.NextPipelineChan()` is called by each tailer (via its launcher) to obtain the write channel; the pipeline's `Processor` then consumes the messages from the other end |
