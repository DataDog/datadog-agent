# comp/logs/integrations

**Team:** agent-log-pipelines

## Purpose

`comp/logs/integrations` is a lightweight pub/sub bus that lets integrations (Python or Go checks) emit log lines and register their log configurations without depending directly on the logs pipeline internals.

There are two distinct data flows it handles:

1. **Configuration registration** — when an integration with a `logs_config` block is scheduled by autodiscovery, it calls `RegisterIntegration`, which sends an `IntegrationConfig` to any subscriber. The subscriber (`pkg/logs/launchers/integration.Launcher`) creates a log file on disk and registers a file-type log source so the file launcher tails it.

2. **Log forwarding** — at runtime, an integration calls `SendLog` with a raw log string. The component relays it through a channel to the `Launcher`, which appends it to the integration's log file.

The component currently supports a single subscriber for each channel. The implementation notes that this can be extended to fan-out if needed.

## Key Elements

### Interface (`comp/logs/integrations/def/component.go`)

```go
type Component interface {
    RegisterIntegration(id string, config integration.Config)
    SubscribeIntegration() chan IntegrationConfig
    Subscribe() chan IntegrationLog
    SendLog(log, integrationID string)
}
```

| Method | Direction | Purpose |
|---|---|---|
| `RegisterIntegration` | producer → component | Called by an integration (or its loader) when it is scheduled and has log config |
| `SubscribeIntegration` | component → consumer | Returns the channel on which `IntegrationConfig` values are delivered |
| `Subscribe` | component → consumer | Returns the channel on which `IntegrationLog` values are delivered |
| `SendLog` | producer → component | Called by an integration to emit a single log line |

Both `Send*` calls block until a consumer reads from the respective channel. The current design assumes a single reader per channel. If no consumer has called `Subscribe` / `SubscribeIntegration` yet, callers will block indefinitely, so the consumer must be started before producers.

### Types (`comp/logs/integrations/def/types.go`)

```go
type IntegrationLog struct {
    Log           string
    IntegrationID string
}

type IntegrationConfig struct {
    IntegrationID string
    Config        integration.Config
}
```

`IntegrationID` is the opaque identifier assigned by autodiscovery (e.g., `docker:abc123`). The `Launcher` maps this to a file on disk under `logs_config.run_path/integrations/<id>.log`.

### Implementation (`comp/logs/integrations/impl/integrations.go`)

The `Logsintegration` struct holds two unbuffered channels:
- `logChan chan IntegrationLog` — for log lines
- `integrationChan chan IntegrationConfig` — for integration registrations

`RegisterIntegration` is a no-op if `config.LogsConfig` is empty, avoiding unnecessary channel sends for integrations that have no log configuration.

### Mock (`comp/logs/integrations/mock/mock.go`)

A mock implementation is available for unit tests that need to inject or observe integration log events without running the full logs pipeline.

## Usage

### Wiring the component

The component is provided as a co-output (`LogsReciever`) of the `comp/logs/agent` constructor. It is exposed as `option.Option[integrations.Component]` in the fx graph so downstream consumers can handle the case where the logs agent is disabled. See [comp/logs/agent](agent.md) for the full list of co-provided values and the `option.Option` unwrapping pattern.

It can also be wired independently for testing:

```go
import integrationsfx "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"

// In a test or standalone binary:
comp := integrationsfx.NewLogsIntegration()
```

### Producer side — integration check

An integration (Python loader, Go core check, shared library loader) registers itself when it is started:

```go
integrationsComp.RegisterIntegration(checkID, config)
```

It then sends log lines during execution:

```go
integrationsComp.SendLog("something happened", checkID)
```

`RegisterIntegration` is a no-op when `config.LogsConfig` is empty, so integrations without a `logs_config` block never block on the channel.

### Consumer side — integration launcher

`pkg/logs/launchers/integration.NewLauncher` subscribes to both channels and processes them in its run loop. The launcher must be started before any producer calls `RegisterIntegration` or `SendLog`; because the channels are unbuffered, producers block until the consumer reads. The logs agent ensures this ordering via `startstop.NewStarter`.

```go
launcher := integration.NewLauncher(fs, sources, integrationsComp)
// internally calls:
//   integrationsComp.Subscribe()          → integrationsLogsChan
//   integrationsComp.SubscribeIntegration() → addedConfigs
```

On `SubscribeIntegration` events the launcher creates a file on disk under `logs_config.run_path/integrations/<id>.log` and adds a `FileType` `LogSource` to `sources.LogSources`. The file launcher then picks up that source and begins tailing it through the normal logs pipeline. On `Subscribe` events the integration launcher appends the log line to that file, enforcing disk quotas:

| Config key | Effect |
|---|---|
| `logs_config.integrations_logs_files_max_size` | Maximum size (MB) per integration log file before rotation |
| `logs_config.integrations_logs_total_usage` | Combined maximum (MB) across all integration log files |
| `logs_config.integrations_logs_disk_ratio` | Fraction of available disk the agent may use (takes precedence if smaller) |

See [pkg/logs/launchers](../../pkg/logs/launchers.md) for the full integration launcher behaviour including quota enforcement and file rotation.

### How log sources flow downstream

When the integration launcher registers a `FileType` `LogSource` via `sources.LogSources.AddSource`, the file launcher (also started by `comp/logs/agent`) picks it up through `SubscribeForType("file")`. From that point the log line travels the standard pipeline: file launcher → tailer → processor → sender. See [pkg/logs/sources](../../pkg/logs/sources.md) for the `LogSources` subscription model and [comp/logs/agent](agent.md) for the full pipeline startup sequence.

### Where it is used

Producers:
- `pkg/collector/python/loader.go` — Python check loader calls `RegisterIntegration` and `SendLog`
- `pkg/collector/corechecks/loader.go` — Go core check loader
- `pkg/collector/sharedlibrary/sharedlibraryimpl/loader.go` — shared library check loader
- `pkg/collector/aggregator/check_context.go` — aggregator context forwards logs from checks
- `comp/core/diagnose/local/local.go` — local diagnosis runner

Consumers:
- `pkg/logs/launchers/integration/launcher.go` — the only current subscriber; bridges the channel to the file-based log pipeline
- `comp/logs/agent/agentimpl/agent.go` — wires the component into the logs agent and provides it to the file launcher

## Related documentation

| Document | Relationship |
|---|---|
| [comp/logs/agent](agent.md) | Top-level logs pipeline component; co-provides `integrations.Component` as `LogsReciever` and passes it to the integration launcher during startup |
| [pkg/logs/launchers](../../pkg/logs/launchers.md) | Documents `integration/launcher.go` in detail — file creation, disk quota enforcement, and how it injects `FileType` sources into the pipeline |
| [pkg/logs/sources](../../pkg/logs/sources.md) | `LogSources` pub/sub registry; the integration launcher adds `FileType` sources here so the file launcher can tail them |
