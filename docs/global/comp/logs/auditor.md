> **TL;DR:** Implements at-least-once delivery for the logs pipeline by maintaining a persistent on-disk registry of per-source read positions, so tailers can resume from the last committed offset after an agent restart.

# comp/logs/auditor

**Team:** agent-log-pipelines

## Purpose

`comp/logs/auditor` implements at-least-once delivery for the logs pipeline by maintaining a persistent registry of per-source read positions. After a batch of log messages is acknowledged by the intake, the sender sends those messages' payloads to the auditor's channel. The auditor records the latest committed offset for each source identifier and periodically flushes it to `registry.json` on disk. On restart, tailers consult the registry to resume from the last known position instead of re-reading from the beginning (or the end) of a file.

## Key Elements

### Key interfaces

#### Interfaces (`comp/logs/auditor/def/`)

```go
// Component is the full auditor lifecycle + channel.
type Component interface {
    Registry
    Start()
    Stop()
    Flush()
    Channel() chan *message.Payload
}

// Registry is the read-only query surface used by launchers and tailers.
type Registry interface {
    GetOffset(identifier string) string
    GetTailingMode(identifier string) string
    GetFingerprint(identifier string) *types.Fingerprint
    KeepAlive(identifier string)
    SetTailed(identifier string, isTailed bool)
    SetOffset(identifier string, offset string)
}
```

Launchers receive only the `Registry` interface (not the full `Component`) to prevent them from controlling the auditor lifecycle.

### Key types

#### Important types

| Type | Location | Description |
|---|---|---|
| `RegistryEntry` | `impl/auditor.go` | Per-source record: `Offset`, `TailingMode`, `LastUpdated`, `IngestionTimestamp`, `Fingerprint` |
| `JSONRegistry` | `impl/auditor.go` | On-disk serialization with a `Version` field (currently v2) |
| `RegistryWriter` | `def/component.go` | Strategy interface for writing the registry file — two implementations: atomic (via temp file rename) and non-atomic |

### Key functions

#### Lifecycle

```
Start() → recover registry from disk → goroutine: run()
Stop()  → close input channel → cleanup expired entries → flush registry to disk
Flush() → drain pending payloads → write registry to disk (blocks until done)
```

The `run()` goroutine has three responsibilities:
- **Update**: consume `*message.Payload` values from `Channel()`. Each payload carries `MessageMeta` items with `Origin.Identifier` and `Origin.Offset`, which are written to the in-memory registry.
- **Flush**: a 1-second ticker (`defaultFlushPeriod`) writes the registry to `<logs_config.run_path>/registry.json`.
- **Cleanup**: a 5-minute ticker (`defaultCleanupPeriod`) removes entries whose `LastUpdated` is older than the configured TTL (`logs_config.auditor_ttl`, in hours). Entries whose identifier is still in `tailedSources` are kept regardless of TTL.

### Offset update ordering

The auditor silently drops an incoming offset that has an `IngestionTimestamp` older than the currently stored one. This prevents dual-shipping scenarios (the same payload delivered to two endpoints) from rolling back the offset.

### Registry versioning

The on-disk format is versioned. The `unmarshalRegistry` method handles v0, v1, and v2 formats for backward compatibility on upgrade.

### Configuration and build flags

#### Configuration keys

| Key | Default | Effect |
|---|---|---|
| `logs_config.run_path` | platform-specific | Directory where `registry.json` is written |
| `logs_config.auditor_ttl` | 23 h | How long to keep an entry for a source that has stopped being tailed |
| `logs_config.message_channel_size` | — | Buffer size of the auditor's input channel |
| `logs_config.atomic_registry_write` | true | Write via temp file + rename for crash safety |

### fx modules

| Package | Description |
|---|---|
| `comp/logs/auditor/fx` | Production module — wires `auditorimpl.NewProvides` |
| `comp/logs/auditor/impl-none` | `NullAuditor` — drains the channel and discards all data; used in serverless where persistence is not needed |
| `comp/logs/auditor/mock` | `mock.Auditor` — collects received payloads in `ReceivedMessages` for test assertions; also exposes `mock.Registry` for testing offset/tailing-mode queries in isolation |

## Usage

### Wiring in production

```go
// Add to the fx app:
logsauditorfx.Module()  // from comp/logs/auditor/fx
```

The `comp/logs/agent` implementation injects `auditor.Component` and passes `auditor.Registry` to each launcher.

### Consuming the registry in a launcher

```go
func (l *MyLauncher) Start(
    sourceProvider launchers.SourceProvider,
    pipelineProvider pipeline.Provider,
    registry auditor.Registry,
    tracker *tailers.TailerTracker,
) {
    offset := registry.GetOffset(source.Config.Path)
    // use offset to resume tailing
}
```

### Sending acknowledged offsets to the auditor

The auditor is fed through the channel returned by `Channel()`. The pipeline sender writes payloads to this channel after the intake returns a success response. The channel is created by `Start()` and closed by `Stop()`; callers must not write to it after `Stop()` is called.

### Persisting a bookmark without a pipeline message

`SetOffset(identifier, offset)` allows a tailer to record its position directly, bypassing the message pipeline. This is useful for sources that perform their own acknowledgment (e.g., Windows Event Log) or when the tailer restarts internally without a full pipeline flush.

### Testing

Use `mock.AuditorMockModule()` to inject a no-op auditor with introspection:

```go
fx.Options(
    mock.AuditorMockModule(),
    // ...
)
```

Or instantiate directly with `mock.NewMockAuditor()` for table-driven unit tests.

## Related documentation

| Document | Relationship |
|---|---|
| [comp/logs/agent.md](agent.md) | The logs agent component injects `auditor.Component`; it passes `auditor.Registry` to each launcher and feeds the auditor channel as the pipeline's `Sink` |
| [pkg/logs/sender.md](../../pkg/logs/sender.md) | The `Sender` delivers payloads to intake destinations and then writes acknowledged `*message.Payload` values to `auditor.Channel()` via the `Sink` interface; at-least-once delivery is the contract between the sender and the auditor |
| [pkg/logs/tailers.md](../../pkg/logs/tailers.md) | Each tailer calls `registry.GetOffset` (and `GetTailingMode`, `GetFingerprint`) at startup to resume from the saved position; file tailers also consult the registry for rotation detection |
