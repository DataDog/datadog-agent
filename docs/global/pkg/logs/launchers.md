> **TL;DR:** `pkg/logs/launchers` bridges log sources to the tailer/listener layer by defining the `Launcher` interface and a `Launchers` collection, with concrete sub-package implementations covering files, containers, journald, Windows Event Log, network sockets, in-process channels, and integration checks.

# pkg/logs/launchers

## Purpose

`pkg/logs/launchers` bridges log sources (`sources.LogSource`) and the tailer/listener layer. Each launcher watches for sources of a specific type to be added or removed (via `SourceProvider`), and responds by starting or stopping the appropriate tailer or listener. The package also provides the `Launchers` collection type that the logs agent uses to manage the full set of launchers as one unit.

## Key elements

### Key interfaces

#### `Launcher` interface

```go
type Launcher interface {
    Start(sourceProvider SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker)
    Stop()
}
```

Every concrete launcher implements this interface. `Start` receives all shared resources and begins watching for sources; `Stop` blocks until the launcher has fully shut down. The `registry` (auditor) is used to persist read offsets/cursors so the agent can resume after a restart.

#### `SourceProvider` interface

```go
type SourceProvider interface {
    SubscribeForType(sourceType string) (added, removed chan *sources.LogSource)
    SubscribeAll() (added, removed chan *sources.LogSource)
    GetAddedForType(sourceType string) chan *sources.LogSource
}
```

The concrete implementation is `*sources.LogSources`. Launchers call one of the subscribe methods at `Start` time and then select on the returned channels in their run loop.

### Key types

#### `Launchers` collection

```go
type Launchers struct { /* private */ }

func NewLaunchers(sources *sources.LogSources, pipelineProvider pipeline.Provider,
    registry auditor.Registry, tracker *tailers.TailerTracker) *Launchers

func (ls *Launchers) AddLauncher(launcher Launcher)
func (ls *Launchers) Start()
func (ls *Launchers) Stop()
```

Holds the full set of active launchers. If `AddLauncher` is called after `Start`, the launcher is started immediately. `Stop` stops all launchers concurrently and waits for all of them to finish.

### Key functions

#### Sub-packages

#### `file/` — file launcher (all platforms)

Subscribes for `config.FileType` sources. On each scan tick (period: `logs_config.file_scan_period`) it calls `FileProvider.FilesToTail` to resolve glob patterns, then creates, updates, or stops `tailers/file.Tailer` instances. Key behaviors:
- Enforces a maximum number of open tailers (`logs_config.open_files_limit`).
- Detects log rotation via inode comparison (UNIX) or fingerprint checksum.
- Supports two wildcard prioritisation strategies: `by_name` (default, reverse alphabetical) and `by_modification_time` (controlled by `logs_config.file_wildcard_selection_mode`).
- Uses a `Fingerprinter` to detect rotation for files that are on rotating filesystems or in container environments.
- Config key `logs_config.validate_pod_container_id` enables cross-checking that a pod log file belongs to the expected container.

Constants: `DefaultSleepDuration` (1 s), `WildcardModeByName`, `WildcardModeByModificationTime`.

Sub-package `file/provider/` contains `FileProvider`, which resolves glob patterns and enforces the open-file limit.

#### `container/` — container launcher (build tags: `kubelet || docker`)

Subscribes to all sources via `SubscribeAll` and handles sources whose `Config.Type` is one of `docker`, `containerd`, `podman`, or `cri-o`. Delegates tailer creation to `tailerfactory.Factory`, which decides whether to use a Docker-socket tailer or a Kubelet-API tailer based on the source configuration. Requires the `kubelet` or `docker` build tag.

#### `journald/` — journald launcher (build tag: `systemd`)

Subscribes for `config.JournaldType` sources. Creates one `tailers/journald.Tailer` per source, opening either the default system journal or a journal at a specific path (`source.Config.Path`). Deduplicates by identifier so the same journal is not tailed twice. Uses a `JournalFactory` interface for testing. Requires the `systemd` build tag. `SDJournalFactory` is the production implementation, wrapping `go-systemd/sdjournal`.

#### `listener/` — network listener launcher (all platforms)

Subscribes for `config.TCPType` and `config.UDPType` sources. For each source it creates a `TCPListener` or `UDPListener` that accepts connections and spawns `tailers/socket.Tailer` instances. Frame size is configurable (`logs_config.frame_size`).

#### `windowsevent/` — Windows Event Log launcher (build tag: `windows`)

Subscribes for `config.WindowsEventType` sources. Creates one `tailers/windowsevent.Tailer` per (channel path, XPath query) pair. On startup it enumerates available Windows event log channels for diagnostics. Uses a `publishermetadatacache` component to resolve provider display names. Requires the `windows` build tag.

#### `channel/` — channel launcher (all platforms)

Subscribes for `config.StringChannelType` sources. For each source, creates a `tailers/channel.Tailer` that reads `*config.ChannelMessage` values from `source.Config.Channel`. Used by internal agent components (serverless, OTel exporter) that produce logs programmatically rather than from an external source.

**Note:** removing a source does not stop the corresponding channel tailer; the tailer drains and exits when its input channel is closed.

#### `integration/` — integration log launcher (all platforms)

Handles logs emitted by Python or Go integration checks. Receives `IntegrationConfig` and `IntegrationLog` events from the `comp/logs/integrations` component. For each integration it creates a per-integration log file under `logs_config.run_path/integrations/`, writes incoming logs to it, and injects a `FileType` source into `LogSources` so the file launcher picks it up.

Enforces per-file and combined disk quotas:
- `logs_config.integrations_logs_files_max_size` — maximum size per integration log file (MB).
- `logs_config.integrations_logs_total_usage` — absolute combined maximum (MB).
- `logs_config.integrations_logs_disk_ratio` — fraction of available disk the agent may use (takes precedence if smaller).

When a file reaches its maximum size it is rotated (delete + recreate). When combined quota is exceeded, the least-recently-modified file is truncated first.

### Configuration and build flags

| Config key / Build tag | Launcher | Description |
|---|---|---|
| `logs_config.open_files_limit` | `file/` | Maximum number of simultaneously open file tailers. |
| `logs_config.file_scan_period` | `file/` | Interval between glob rescans. |
| `logs_config.file_wildcard_selection_mode` | `file/` | `by_name` (default) or `by_modification_time` prioritisation. |
| `logs_config.validate_pod_container_id` | `file/` | Cross-check that a pod log file belongs to the expected container. |
| `logs_config.frame_size` | `listener/` | Maximum TCP/UDP frame size. |
| `logs_config.run_path` | `integration/` | Base directory for integration log files. |
| `logs_config.integrations_logs_files_max_size` | `integration/` | Maximum size per integration log file (MB). |
| `logs_config.integrations_logs_total_usage` | `integration/` | Combined disk quota for all integration log files (MB). |
| `logs_config.integrations_logs_disk_ratio` | `integration/` | Fraction of available disk the agent may use (overrides the fixed quota if smaller). |
| Build tag `kubelet \|\| docker` | `container/` | Required for the container launcher. |
| Build tag `systemd` | `journald/` | Required for the journald launcher. |
| Build tag `windows` | `windowsevent/` | Required for the Windows Event Log launcher. |

## Usage

The launchers are assembled in `comp/logs/agent/agentimpl/agent_core_init.go` (`addLauncherInstances`) and wired up with `NewLaunchers`:

```go
lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, a.auditor, a.tracker)
lnchrs.AddLauncher(filelauncher.NewLauncher(...))
lnchrs.AddLauncher(listener.NewLauncher(...))
lnchrs.AddLauncher(journald.NewLauncher(...))
lnchrs.AddLauncher(windowsevent.NewLauncher())
lnchrs.AddLauncher(container.NewLauncher(...))
lnchrs.AddLauncher(integrationLauncher.NewLauncher(...))
// later:
lnchrs.Start()
// on shutdown:
lnchrs.Stop()
```

The `Launchers` collection is also reconstructed during a transport restart (e.g. when connectivity switches from TCP to HTTP) while keeping the auditor and source registry intact.

To add a new source type: implement the `Launcher` interface, subscribe to the appropriate source type in `Start`, and register the launcher via `AddLauncher`.

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; shows how launchers fit into the full data-flow (schedulers → launchers → tailers → pipeline) |
| [sources.md](sources.md) | `LogSource` and `LogSources` — the `SourceProvider` concrete implementation; launchers call `SubscribeForType` / `SubscribeAll` on `*sources.LogSources` |
| [tailers.md](tailers.md) | Tailer implementations created and managed by each launcher; `TailerTracker` and `TailerContainer` are the lifecycle registry passed to every launcher |
| [pipeline.md](pipeline.md) | `pipeline.Provider` is the second argument to every launcher's `Start`; launchers call `NextPipelineChan()` / `NextPipelineChanWithMonitor()` to obtain write channels for tailers |
| [message.md](message.md) | `*message.Message` values are what tailers ultimately write to pipeline channels; the `Origin` field links each message back to the originating `LogSource` |
| [schedulers.md](schedulers.md) | Schedulers populate `LogSources` with `LogSource` entries that launchers react to; the two layers are decoupled through the pub/sub channels on `LogSources` |
| [../../pkg/util/docker.md](../../pkg/util/docker.md) | The container launcher's `tailerfactory` uses `pkg/util/docker.DockerUtil.ContainerLogs` when creating socket-backed container tailers (Docker daemon mode) |
| [../../pkg/util/containerd.md](../../pkg/util/containerd.md) | The container launcher's `tailerfactory` queries the containerd API for container metadata when deciding whether to use the Docker socket or the Kubelet API tailer |
