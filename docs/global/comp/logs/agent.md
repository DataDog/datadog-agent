> **TL;DR:** Top-level fx component for the logs collection subsystem; discovers log sources, processes each message through configurable rules, and forwards results to the Datadog intake or a Vector proxy.

# comp/logs/agent

**Team:** agent-log-pipelines

## Purpose

`comp/logs/agent` is the top-level fx component for the logs collection subsystem. It owns the full data pipeline: it discovers log sources (files, containers, journald, Windows Event Log, integrations, syslog listeners), processes each log message through a configurable set of rules, and forwards the results to the Datadog intake or a Vector proxy. The component also exposes status information, flare data, and a streaming API endpoint (`/stream-logs`) for live log inspection.

When `logs_enabled: false` is set in the configuration, `newLogsAgent` returns an `option.None` and the pipeline is skipped entirely without failing the agent startup.

## Key Elements

### Key interfaces

#### Interfaces (`comp/logs/agent/component.go`)

```go
type Component interface {
    AddScheduler(scheduler schedulers.Scheduler)
    GetSources() *sources.LogSources
    GetMessageReceiver() *diagnostic.BufferedMessageReceiver
    GetPipelineProvider() pipeline.Provider
}

type ServerlessLogsAgent interface {
    Component
    Start() error
    Stop()
    Flush(ctx context.Context)
}
```

`ServerlessLogsAgent` extends `Component` with an explicit lifecycle for use in the serverless agent, which does not wire dependencies through fx.

### Key types

#### fx module (`agentimpl/agent.go`)

```go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(newLogsAgent))
}
```

The constructor `newLogsAgent` resolves all dependencies listed in `dependencies` and produces the `provides` struct, which registers:

| Provided value | Type | Purpose |
|---|---|---|
| `Comp` | `option.Option[agent.Component]` | The logs agent itself (None when disabled) |
| `StatusProvider` | `statusComponent.InformationProvider` | Agent status section |
| `FlareProvider` | `flaretypes.Provider` | Flare data collection |
| `LogsReciever` | `option.Option[integrations.Component]` | Channel for integration-submitted logs |
| `APIStreamLogs` | `api.AgentEndpointProvider` | `POST /stream-logs` HTTP endpoint |

### Key functions

#### Dependencies

The constructor requires:
- `configComponent.Component` — for `logs_enabled`, endpoint config, processing rules, etc.
- `auditor.Component` (`comp/logs/auditor`) — tracks file offsets across restarts
- `logscompression.Component` (`comp/serializer/logscompression`) — compressor factory for sending
- `hostname.Component`, `tagger.Component`, `option.Option[workloadmeta.Component]`
- `inventoryagent.Component` — records the active transport (HTTP/TCP) in inventory metadata
- `[]schedulers.Scheduler` via the fx value group `"log-agent-scheduler"` — pluggable source schedulers (e.g., the autodiscovery scheduler)

### Configuration and build flags

#### Scheduler plug-in pattern

Any component that needs to add log sources at runtime should provide a scheduler via the `"log-agent-scheduler"` value group:

```go
// In a component's fx provider:
agentimpl.NewSchedulerProvider(myScheduler)
```

Schedulers are started once with `sync.Once` so they are always registered exactly once regardless of restarts.

### Pipeline structure

On `start`, the implementation:

1. Builds `config.Endpoints` (HTTP preferred; falls back to TCP with automatic background retry via `smartHTTPRestart`)
2. Creates a `pipeline.Provider` with the configured number of parallel pipelines (`logs_config.pipelines`)
3. Instantiates launchers for each source type: file, listener, journald, Windows Event Log, container, and integrations
4. Starts the auditor, pipeline provider, launchers, and schedulers in dependency order using `startstop.NewStarter`

On `stop`, components are drained in reverse order. A configurable grace period (`logs_config.stop_grace_period`) is respected; if it expires, destinations are force-flushed and a goroutine dump is captured.

The `restart` path (transport switch TCP → HTTP) only recreates the `destinationsCtx`, `pipelineProvider`, and `launchers`; the auditor, schedulers, sources, and tailing tracker are preserved.

### Config sub-package (`comp/logs/agent/config`)

Contains helpers for building `Endpoints`, parsing `ProcessingRule`s, validating fingerprint config, and defining configuration key constants (`config_keys.go`). Shared by the agent implementation and by `pkg/logs/pipeline`.

### Mock (`agentimpl/mock.go`)

The `Mock` interface adds `SetSources(*sources.LogSources)` on top of `Component` for unit tests that need to inject log sources directly without running the full pipeline.

## Usage

### Enabling the component

```go
// In a cmd's fx app:
logsagentfx.Module()         // from comp/logs/agent/agentimpl
logsauditorfx.Module()       // from comp/logs/auditor/fx
logscompressionfx.Module()   // from comp/serializer/logscompression/fx
```

### Adding a log scheduler at runtime

A component that discovers new log sources (e.g., autodiscovery) should:

1. Implement `schedulers.Scheduler`
2. Provide it to the fx app via `agentimpl.NewSchedulerProvider(myScheduler)`

The logs agent will call `AddScheduler` on it during startup.

### Accessing the component from another component

Because the component is wrapped in `option.Option`, callers must unwrap it:

```go
type MyDeps struct {
    fx.In
    LogsAgent option.Option[agent.Component]
}

func (d MyDeps) doWork() {
    if logsAgent, ok := d.LogsAgent.Get(); ok {
        provider := logsAgent.GetPipelineProvider()
        // ...
    }
}
```

### Integration logs (programmatic submission)

Integrations that want to submit log lines programmatically can use the `integrations.Component` receiver that is co-provided alongside the agent. Fetch it from the fx graph as `option.Option[integrations.Component]`.

### Streaming live logs

The `/stream-logs` endpoint is registered automatically. It wraps `GetMessageReceiver()`, which returns a `diagnostic.BufferedMessageReceiver` that receives a copy of every processed log message. This is also the backing store for `datadog-agent stream-logs`.

## Related documentation

| Document | Relationship |
|---|---|
| [pkg/logs/logs.md](../../pkg/logs/logs.md) | Comprehensive overview of the entire logs subsystem — schedulers, launchers, tailers, pipeline, processor, sender, and auditor |
| [pkg/logs/pipeline.md](../../pkg/logs/pipeline.md) | `Pipeline` and `Provider` — the processing/forwarding layer created and owned by this component; describes encoder selection, batch strategy, and failover routing |
| [pkg/logs/sources.md](../../pkg/logs/sources.md) | `LogSource` and `LogSources` — the registry that schedulers write to and launchers read from; `GetSources()` on `Component` returns the live `LogSources` instance |
| [pkg/logs/launchers.md](../../pkg/logs/launchers.md) | All concrete launchers (file, container, journald, Windows Event, channel, integration) assembled in `agentimpl/agent_core_init.go` |
| [pkg/logs/tailers.md](../../pkg/logs/tailers.md) | Tailer implementations and `TailerTracker` created by launchers and wired to pipeline channels |
| [comp/logs/auditor.md](auditor.md) | At-least-once delivery registry; injected as `auditor.Component` and passed as `auditor.Registry` to every launcher |
| [comp/serializer/logscompression.md](../serializer/logscompression.md) | Compressor factory injected here and forwarded to `pipeline.NewProvider`; each pipeline instance gets its own compressor |
| [comp/core/autodiscovery.md](../core/autodiscovery.md) | The autodiscovery scheduler plugs into this component via the `"log-agent-scheduler"` value group; it drives container/integration log source discovery |
