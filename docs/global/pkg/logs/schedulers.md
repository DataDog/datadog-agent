# pkg/logs/schedulers

## Purpose

`pkg/logs/schedulers` is the glue between the logs agent's source registry and the various systems that decide _which_ log sources should be active. A **Scheduler** watches some external event source (autodiscovery, an in-process channel, remote config, etc.) and calls `SourceManager.AddSource` / `RemoveSource` in response. Multiple schedulers can run simultaneously; the `Schedulers` aggregate manages their lifecycle.

---

## Key elements

### Core interfaces and types (`schedulers.go`, `types.go`)

| Symbol | Description |
|---|---|
| `Scheduler` interface | `Start(sourceMgr SourceManager)` — called when the logs agent starts; `Stop()` — blocks until shutdown is complete. |
| `SourceManager` interface | `AddSource(*sources.LogSource)`, `RemoveSource(*sources.LogSource)`, `GetSources() []*sources.LogSource`, `AddService(*service.Service)`, `RemoveService(*service.Service)`. The surface schedulers use to mutate the running source set. |
| `Schedulers` struct | Holds a collection of `Scheduler` implementations and a single `SourceManager`. `AddScheduler` can be called before or after `Start`; if already started the new scheduler is started immediately. `Stop` stops all schedulers concurrently with a `sync.WaitGroup`. |
| `NewSchedulers(sources, services)` | Constructor. Wraps `sources.LogSources` and `service.Services` in the internal `sourceManager` adapter that implements `SourceManager`. |

---

### `ad/` — Autodiscovery-based scheduler

Listens to the AutoConfig component and creates/removes log sources in response to integration configs (container labels, pod annotations, config files, remote config).

| Symbol | Description |
|---|---|
| `Scheduler` struct | Holds a `schedulers.SourceManager` and an `adlistener.ADListener`. |
| `New(ac autodiscovery.Component) schedulers.Scheduler` | Creates the scheduler and wires it to the given AutoConfig instance. |
| `Schedule([]integration.Config)` | Called by AutoConfig when new configs are scheduled. Filters out non-log configs and workload-filtered containers, then calls `CreateSources` and adds the results via `SourceManager.AddSource`. |
| `Unschedule([]integration.Config)` | Called when configs are removed. Finds matching sources by `Identifier` and removes them. |
| `CreateSources(config integration.Config) ([]*sources.LogSource, error)` | Parses the `LogsConfig` blob (YAML or JSON, depending on the provider) and returns ready-to-use `*sources.LogSource` values. Exported so it can be tested or used directly. |
| `filterConflictingSources` | When the provider is `process_log`, manually configured file sources (from the `file` provider) take precedence — the new sources for conflicting paths are silently dropped. |

**Supported providers:** `file`, `container`, `kubernetes`, `kube_container`, `process_log`, `remote_config`, `datastreams_live_messages`.

---

### `channel/` — Channel-based scheduler

Manages exactly one log source backed by an in-process `chan *config.ChannelMessage`. Used by internal components (e.g. the serverless agent, DogStatsD) that generate logs programmatically rather than tailing files or containers.

| Symbol | Description |
|---|---|
| `Scheduler` struct | Holds the channel, source name, source tag, the live `*sources.LogSource`, and the `SourceManager`. |
| `NewScheduler(sourceName, source string, logsChan chan *config.ChannelMessage) *Scheduler` | Creates the scheduler (does not start it). |
| `Start(sourceMgr)` | Registers a `StringChannelType` log source with the pipeline. |
| `Stop()` | No-op; the channel source is implicitly removed when the logs agent stops. |
| `SetLogsTags(tags []string)` | Updates the tags associated with future channel messages at runtime (thread-safe). |
| `GetLogsTags() []string` | Returns a defensive copy of the current tags. |

---

## Usage

The logs agent component (`comp/logs/agent`) instantiates `schedulers.NewSchedulers`, then registers each desired scheduler with `AddScheduler`:

```go
ss := schedulers.NewSchedulers(logSources, logServices)
ss.AddScheduler(ad.New(autoConfig))
ss.AddScheduler(channel.NewScheduler("dogstatsd-logs", "dogstatsd", logsChan))
ss.Start()
// ... agent runs ...
ss.Stop()
```

Adding a scheduler after `Start()` is safe and starts it immediately. Schedulers communicate exclusively through the `SourceManager` interface; they do not interact with each other.
