> **TL;DR:** `pkg/logs/sources` is the thread-safe registry that connects schedulers (which discover log sources) to launchers (which tail them), providing pub/sub channels so launchers are notified when sources are added or removed.

# pkg/logs/sources

## Purpose

`pkg/logs/sources` is the source registry for the logs pipeline. It defines `LogSource` — the runtime representation of a single log collection target — and `LogSources` — the thread-safe registry that connects **schedulers** (which discover and register sources) to **launchers** (which tail/listen to those sources).

When a scheduler detects that an integration, file, container, or Windows Event Log should be collected, it creates a `LogSource` and adds it to `LogSources`. Launchers subscribe to change notifications from `LogSources` and start or stop the appropriate tailer in response.

## Key elements

### Key types

| Type | Description |
|------|-------------|
| `LogSource` | Represents one log collection target. Holds the `LogsConfig` (type, path, service name, tags, processing rules…), a `LogStatus`, latency and byte-read counters, an info registry for the status page, and an optional reference to a `ParentSource`. All mutations are protected by an internal mutex. |
| `LogSources` | Thread-safe registry and fan-out bus. Holds the set of all active `LogSource` instances and a set of subscriber channels, keyed by source type. |
| `ConfigSources` | Simpler, non-persistent variant used when sources come from static configuration files rather than from the dynamic autodiscovery bus. Does not support removal notifications. |
| `ReplaceableSource` | A `sync.RWMutex`-protected wrapper around a `*LogSource` that allows a tailer to atomically swap its underlying source (used in container log collection when the source changes while the tailer stays alive). |
| `SourceType` | String enum for the *origin* of log lines. Values: `DockerSourceType`, `KubernetesSourceType`, `IntegrationSourceType`. Distinct from `LogsConfig.Type`, which describes the tailer kind (file, TCP, journald, …). |

### Key functions

#### `LogSource` fields of interest

| Field | Description |
|-------|-------------|
| `Config *config.LogsConfig` | Static collection configuration (type, path, service, source, tags, processing rules). |
| `Status *status.LogStatus` | Tracks success/error state shown on the status page. |
| `Messages *config.Messages` | User-facing messages shown on the status page. |
| `LatencyStats *statstracker.Tracker` | Rolling 24 h histogram of message latency (decode → sender hand-off). |
| `BytesRead *status.CountInfo` | Cumulative byte counter forwarded to the parent source for `container_collect_all`. |
| `ProcessingInfo *status.ProcessingInfo` | Tracks counts of processed/dropped messages. |
| `ParentSource *LogSource` | Set when this source overrides a parent (e.g. per-container override of a `container_collect_all` source). Byte-read events bubble up to the parent. |

#### `LogSources` methods

| Method | Description |
|--------|-------------|
| `AddSource(source)` | Appends a source to the registry and fans out a notification to all matching subscribers. Sources with an invalid `Config` are stored but not broadcast. |
| `RemoveSource(source)` | Removes the source and notifies removal subscribers. |
| `SubscribeAll()` | Returns `(added, removed)` channels that receive every future add/remove event. Existing sources are replayed on `added` from a goroutine. |
| `SubscribeForType(type)` | Like `SubscribeAll` but filtered to a single source type string (`"file"`, `"docker"`, `"journald"`, …). |
| `GetAddedForType(type)` | Returns only the `added` channel (no removal notifications). |
| `GetSources()` | Returns a snapshot copy of all current sources. |

> Channels are **unbuffered**. Subscribers must consume promptly or they will block the goroutine calling `AddSource`/`RemoveSource`.

#### `ReplaceableSource` methods

`Replace(source)`, `Config()`, `Status()`, `AddInput()`, `RemoveInput()`, `RecordBytes()`, `GetSourceType()`, `RegisterInfo()`, `GetInfo()`, `UnderlyingSource()` — all delegate to the wrapped `*LogSource` under a read lock, with `Replace` taking a write lock.

### Configuration and build flags

`pkg/logs/sources` has no user-facing config keys of its own. The source type strings that `SubscribeForType` filters on correspond to `LogsConfig.Type` values set by the agent configuration (`"file"`, `"docker"`, `"journald"`, `"tcp"`, `"udp"`, `"windows_event"`, `"string_channel"`, etc.). Per-source configuration is defined in `comp/logs/agent/config.LogsConfig`.

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; `LogSources` is the shared registry that connects schedulers to launchers |
| [message.md](message.md) | `Origin.LogSource` embeds a `*LogSource`; every `Message` carries a reference to its source |
| [schedulers.md](schedulers.md) | Schedulers call `AddSource`/`RemoveSource` on the `SourceManager` wrapper of `LogSources` |
| [launchers.md](launchers.md) | Launchers call `SubscribeForType` or `SubscribeAll` on `LogSources` to receive source notifications |

## Usage

`LogSources` is created once at agent startup in `comp/logs/agent/agentimpl`:

```go
// comp/logs/agent/agentimpl/agent.go
a.sources = sources.NewLogSources()
```

**Schedulers** add and remove sources:

```go
// pkg/logs/schedulers/ad/scheduler.go
mgr.AddSource(sources.NewLogSource(name, cfg))
```

**Launchers** subscribe to be notified of new sources to tail. For example, the file launcher subscribes to `"file"`-typed sources:

```go
// pkg/logs/launchers/file/launcher.go
added, removed := logSources.SubscribeForType("file")
```

The container launcher, Windows Event launcher, journald launcher, and integration launcher all follow the same pattern with their respective source types.

`ReplaceableSource` is used inside tailers (e.g. `pkg/logs/tailers/file`) when the underlying `LogSource` may need to be swapped while the tailer is running — common in container log collection scenarios.

`ConfigSources` is used by the `analyzelogs` subcommand (`cmd/agent/subcommands/analyzelogs`) and in tests where static configuration is preferred over autodiscovery.

### Subscription channel blocking

`LogSources` subscriber channels are **unbuffered** (see `SubscribeForType` / `SubscribeAll`). A launcher that is slow to consume the channel will block the goroutine that called `AddSource` or `RemoveSource`. Keep launcher run loops lean and avoid blocking operations inside the `select` on `added`/`removed` channels.

### Parent-source byte accounting

When `container_collect_all` is active, a child `LogSource` (per-container override) sets `ParentSource` to the catch-all source. Each call to `RecordBytes` on the child also increments the parent's `BytesRead` counter. This lets the status page show aggregate throughput for the `container_collect_all` source even when individual container sources handle the actual bytes.

### `ReplaceableSource` in container tailers

File tailers in the container launcher hold a `ReplaceableSource` rather than a direct `*LogSource`. When the container's metadata changes (e.g. Kubernetes pod labels are updated) the launcher can call `Replace(newSource)` without stopping and restarting the tailer. The tailer reads config and status through the `ReplaceableSource` wrappers so it automatically picks up the new source on the next read cycle. See [launchers.md](launchers.md) for the container launcher details.
