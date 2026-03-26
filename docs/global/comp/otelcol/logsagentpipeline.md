> **TL;DR:** A slimmed-down send-only logs pipeline for the OTel Agent and core Agent OTLP ingestion path, providing the downstream `pipeline.Provider` channel that OTLP log exporters write into.

# comp/otelcol/logsagentpipeline — OTel logs pipeline component

## Purpose

`comp/otelcol/logsagentpipeline` is a slimmed-down logs Agent pipeline that
runs inside the OTel Agent and the core Agent's OTLP ingestion path. Its sole
job is to provide the downstream infrastructure that OTLP log exporters write
into: a `pipeline.Provider` that handles batching, processing, and forwarding
of log messages to the Datadog logs intake.

It is distinct from the full logs Agent (`comp/logs/agent`) which also manages
log collection sources (files, containers, etc.). This component only handles
the sending side of the pipeline and is initialised only when
`logs_enabled: true` is set in the Agent configuration.

## Key elements

### Key interfaces

#### Interface

Defined in `comp/otelcol/logsagentpipeline/component.go`:

```go
type Component interface {
    GetPipelineProvider() pipeline.Provider
}

// LogsAgent is the non-fx variant used for manual lifecycle management.
type LogsAgent interface {
    Component
    Start(context.Context) error
    Stop(context.Context) error
}
```

`GetPipelineProvider()` returns the `pipeline.Provider`, which exposes
`NextPipelineChan()` — the channel that OTLP log exporters write
`*message.Message` values into.

### Key types

#### Implementation: `logsagentpipelineimpl.Agent`

Located in `comp/otelcol/logsagentpipeline/logsagentpipelineimpl/agent.go`.

| Step | Details |
|------|---------|
| **Construction** | `NewLogsAgentComponent(deps)` returns `option.Option[logsagentpipeline.Component]`. Returns `None` if `logs_enabled` is false. |
| **`Start`** | Builds HTTP endpoints from `datadog.yaml`, applies global processing rules, creates a `pipeline.Provider` via `pipeline.NewProvider`. |
| **`Stop`** | Drains the pipeline with a configurable grace period (`logs_config.stop_grace_period`). Forces a hard stop after an additional 5-second timeout. |

#### `Dependencies` (fx)

```go
type Dependencies struct {
    fx.In
    Lc           fx.Lifecycle
    Log          log.Component
    Config       configComponent.Component
    Hostname     hostnameinterface.Component
    Compression  compression.Component
    IntakeOrigin config.IntakeOrigin
}
```

`IntakeOrigin` identifies the source of logs to the intake (e.g. `agent`, `otel`).

### Key functions

#### fx module

```go
// logsagentpipelineimpl.Module()
fx.Provide(NewLogsAgentComponent)
```

The component is provided as `option.Option[logsagentpipeline.Component]` so
that consumers like `comp/otelcol/collector/impl` can declare an optional
dependency without failing when logs are disabled.

### Configuration and build flags

#### Pipeline internals

`SetupPipeline` (called from `Start`) creates:

- A `client.DestinationsContext` for managing HTTP destination lifecycle.
- A `pipeline.Provider` with `N` pipelines (`logs_config.pipelines`), each
  consisting of a processor, sender, and retry queue.
- A no-op `sender.NoopSink` and `diagnostic.NoopMessageReceiver` (no
  local log-collection diagnostics in this mode).

The provider is started in dependency order via `startstop.NewStarter` and
stopped in reverse via `startstop.NewSerialStopper`.

## Usage

### Where it is used

| Consumer | How |
|----------|-----|
| `comp/otelcol/collector/impl` (OTel Agent) | Declared as `option.Option[logsagentpipeline.Component]` in `Requires`. If present, the `logsagentexporter` factory is initialized with the pipeline channel via `v.GetPipelineProvider()`. |
| `comp/otelcol/collector/impl-pipeline` (core Agent OTLP) | Declared as `option.Option[logsagentpipeline.Component]` in `Requires`. If present, `provider.NextPipelineChan()` is passed to `otlp.NewPipelineFromAgentConfig` as the `logsAgentChannel`. |
| `comp/otelcol/otlp/components/exporter/datadogexporter` | Receives the `Component` directly and calls `GetPipelineProvider()` to obtain the log channel. |

### Wire-up in `cmd/otel-agent`

```go
logsagentpipelineimpl.Module()   // provides option.Option[logsagentpipeline.Component]
logscompressionfx.Module()       // provides compression.Component (dependency)
```

### Enabling logs in OTLP mode (core Agent)

```yaml
# datadog.yaml
logs_enabled: true
otlp_config:
  logs:
    enabled: true
```

Without `logs_enabled: true`, `NewLogsAgentComponent` returns `None` and log
ingestion is silently disabled. A validation check in `comp/otelcol/otlp`
(`checkAndUpdateCfg`) will return an error if `otlp_config.logs.enabled` is
true but no logs channel is available.

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/logs/agent` | [../logs/agent.md](../logs/agent.md) | The full logs Agent, which also exposes `GetPipelineProvider() pipeline.Provider`. This component is a slimmed-down counterpart: it provides only the sending side of the pipeline without log source discovery, autodiscovery schedulers, or the auditor. Both components construct a `pipeline.Provider` via `pipeline.NewProvider` and share the same downstream pipeline infrastructure. |
| `comp/otelcol/otlp` | [otlp.md](otlp.md) | In the core Agent OTLP path, `comp/otelcol/collector/impl-pipeline` reads `option.Option[logsagentpipeline.Component]` and passes `provider.NextPipelineChan()` to `otlp.NewPipelineFromAgentConfig`. The `logsagentexporter` in the OTLP pipeline writes `*message.Message` values into that channel. A validation check in `comp/otelcol/otlp` returns an error if `otlp_config.logs.enabled` is `true` but no pipeline channel is available. |
| `comp/otelcol/collector` | [collector.md](collector.md) | In the OTel Agent (`impl`), `comp/otelcol/collector` declares `option.Option[logsagentpipeline.Component]` in its `Requires` struct. When present, the `logsagentexporter` factory is initialised with the pipeline channel returned by `GetPipelineProvider()`. If the option is absent (logs disabled), the exporter is omitted from the factory set. |
| `pkg/logs/pipeline` | [../../pkg/logs/pipeline.md](../../pkg/logs/pipeline.md) | `pipeline.Provider` (created via `pipeline.NewProvider` in `SetupPipeline`) is the core abstraction this component exposes through `GetPipelineProvider()`. The provider owns the `Pipeline` instances, processor stage, sender, and retry queue. `NextPipelineChan()` on the provider is the channel that OTLP exporters write decoded log records into. |
