> **TL;DR:** Manages the lifecycle of a full OpenTelemetry Collector embedded in the Datadog Agent, supporting both the Datadog Distribution of OpenTelemetry (DDOT) with a user-supplied config and the core Agent's fixed OTLP ingestion pipeline.

# comp/otelcol/collector — OpenTelemetry Collector component

## Purpose

`comp/otelcol/collector` is the fx component that manages the lifecycle of a
full OpenTelemetry Collector process embedded inside the Datadog Agent. It is
used by the **Datadog Distribution of OpenTelemetry (DDOT)**, also called the
OTel Agent (`cmd/otel-agent`), and optionally by the core Agent when
`otelcollector.enabled: true` is set.

Unlike `comp/otelcol/otlp` (the in-process OTLP ingestion pipeline with a
fixed pipeline topology), this component accepts a user-supplied OTel Collector
configuration file and can run any combination of standard Collector components
together with Datadog-specific extensions.

The component requires the `otlp` build tag.

## Key elements

### Key interfaces

#### Interface

```go
// comp/otelcol/collector/def
type Component interface {
    Status() datatype.CollectorStatus
}
```

`Status()` returns the current OTel Collector state (e.g. `Running`, `Stopping`)
along with any error string.

### Key types

#### Implementations

There are two implementations, selected by the fx module used:

| Module | Implementation | Use case |
|--------|----------------|----------|
| `comp/otelcol/collector/fx` | `impl/collector.go` — `NewComponent` / `NewComponentNoAgent` | OTel Agent (`cmd/otel-agent`) |
| `comp/otelcol/collector/fx-pipeline` | `impl-pipeline/pipeline.go` — `NewComponent` | Core Agent OTLP pipeline (`cmd/agent`) |

#### `impl` (OTel Agent)

`NewComponent` (`Requires` struct) wires together:

- `CollectorContrib` (`comp/otelcol/collector-contrib`) — the full OTel contrib
  factory set.
- `Converter` (`confmap.Converter`) — enhanced config converter (see
  `comp/otelcol/converter`).
- `URIs []string` — paths to the user OTel config file(s).
- Core Agent components: `Serializer`, `TraceAgent`, `LogsAgent`, `Tagger`,
  `Hostname`, `Ipc`, `Telemetry`, `AgentTelemetry`.

`addFactories` injects Datadog-specific factories on top of the contrib set:

| Factory type | Name | Purpose |
|-------------|------|---------|
| Exporter | `datadog` | Routes metrics/traces/logs to the Datadog backend via the Agent. |
| Processor | `infraattributes` | Enriches signals with infrastructure tags from the Tagger. |
| Connector | `datadog` | APM stats connector (`apmstats`) for trace-to-metrics conversion. |
| Extension | `ddflare` | Exposes OTel config via the Agent's flare mechanism. |
| Extension | `ddprofiling` | Connects the OTel Collector to the Agent's profiling pipeline. |

`NewComponentNoAgent` is used when the user config does not include a Datadog
exporter. It registers only the `datadog` connector, `infraattributes`
processor, and `ddflare` extension, skipping all Agent dependencies.

#### `impl-pipeline` (core Agent)

A lighter implementation that wraps `comp/otelcol/otlp.Pipeline`. It provides:

- `FlareProvider` — serialises OTLP pipeline diagnostics into Agent flares.
- `StatusProvider` — surfaces the collector state in `agent status`.

### Key functions

#### fx modules

```go
// For OTel Agent
collectorfx.Module(params)         // uses impl.NewComponent
collectorfx.ModuleNoAgent()        // uses impl.NewComponentNoAgent

// For core Agent
collectorfx_pipeline.Module()      // uses impl-pipeline.NewComponent
```

Both modules call `fxutil.ProvideOptional[collector.Component]()` so the
component is available as an `option.Option[collector.Component]` for consumers
that must not hard-depend on it.

### Configuration and build flags

#### `Params`

```go
type Params struct {
    BYOC bool  // true when the OTel Agent was built via Bring-Your-Own-Collector
}
```

Passed to the `impl` constructor via `fx.Supply(params)` in the fx module.

### Configuration keys (datadog.yaml / environment)

| Key | Description |
|-----|-------------|
| `otelcollector.enabled` | Must be `true` for `NewComponent` to start the collector. |
| `otelcollector.converter.enabled` | Activates the `ddConverter` confmap converter. |
| `otelcollector.gateway.mode` | Switches deployment type to `gateway` in the Datadog extension. |
| `otelcollector.extension_timeout` | Timeout (seconds) for the `ddflare` extension HTTP calls. |

## Usage

### OTel Agent (`cmd/otel-agent`)

`cmd/otel-agent/subcommands/run/command.go` wires the full dependency graph:

```
collectorfx.Module(params)          ← collector component
converterfx.Module()                ← converter component
collectorcontribFx.Module()         ← upstream OTel contrib factories
logsagentpipelineimpl.Module()      ← OTel-specific logs pipeline
traceagentfx.Module()               ← trace-agent
...
```

On `OnStart`, the implementation:
1. Calls `col.DryRun(ctx)` to validate the user-supplied config.
2. Starts `col.Run` in a background goroutine.
3. Registers `setupShutdown` to propagate OTel Collector exits to the fx `Shutdowner`.

On `OnStop`, calls `col.Shutdown()`.

### Core Agent (`cmd/agent`)

`cmd/agent/subcommands/run/command.go` includes
`collectorfx_pipeline.Module()` (the `fx-pipeline` variant) which drives
`comp/otelcol/otlp` and does not use the user-supplied OTel config file.

### Checking collector state

```go
status := collectorComp.Status()
// status.Status      → e.g. "Running"
// status.ErrorMessage → non-empty if startup failed
```

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/otelcol/otlp` | [otlp.md](otlp.md) | The in-process OTLP ingestion pipeline for the core Agent. The `fx-pipeline` variant of this component wraps `comp/otelcol/otlp` directly: `impl-pipeline.NewComponent` calls `otlp.NewPipelineFromAgentConfig` and adds `FlareProvider`/`StatusProvider` on top. In contrast, the `impl` (OTel Agent) variant runs a full user-supplied OTel Collector config and does not use the hardcoded `otlp` pipeline topology. |
| `comp/otelcol/converter` | [converter.md](converter.md) | The `impl` constructor receives a `confmap.Converter` (provided by `converterfx.Module()`) and wraps it in a `converterFactory` passed to `otelcol.ConfigProviderSettings`. The converter automatically enriches the user-supplied OTel config with Datadog extensions (`infraattributes`, `ddflare`, `datadog`, etc.) before the Collector validates it. It is activated only when `otelcollector.converter.enabled: true`. |
| `comp/otelcol/logsagentpipeline` | [logsagentpipeline.md](logsagentpipeline.md) | Both the `impl` and `impl-pipeline` constructors declare `option.Option[logsagentpipeline.Component]` in their `Requires` struct. When present, the `logsagentexporter` factory is initialised with the pipeline channel from `GetPipelineProvider()`. When the option is absent (logs disabled or component not wired), the exporter is omitted from the factory set and OTLP log ingestion is disabled. |
| `pkg/opentelemetry-mapping-go` | [../../pkg/opentelemetry-mapping-go.md](../../pkg/opentelemetry-mapping-go.md) | Used indirectly through the `datadog` exporter factory injected by `addFactories`. The exporter relies on `attributes.Translator` for hostname/tag mapping and `metrics.Provider` (via `NewDefaultTranslator`) for OTLP-to-Datadog metric conversion. In the DDOT (`impl`) path, `WithRemapping()` and `WithOTelPrefix()` options may be applied to avoid name collisions with native Agent metrics. |
