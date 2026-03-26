# comp/otelcol/otlp — OTLP ingestion pipeline

## Purpose

`comp/otelcol/otlp` embeds a minimal OpenTelemetry Collector pipeline directly
inside the Datadog Agent. It receives telemetry data over the OTLP protocol
(gRPC and HTTP) from instrumented applications and routes it through
Datadog-specific exporters without requiring a standalone Collector process.

This component is the **in-process OTLP ingestion path** for the core Agent and
the serverless Agent. It is separate from the full OTel Agent (`comp/otelcol/collector`),
which is the externally-configured Collector used in the Datadog Distribution of
OpenTelemetry (DDOT) deployment.

The package requires the `otlp` build tag.

## Key elements

### Types

| Type | Package | Description |
|------|---------|-------------|
| `PipelineConfig` | `comp/otelcol/otlp` | Runtime configuration for a pipeline instance. |
| `Pipeline` | `comp/otelcol/otlp` | Wraps an `otelcol.Collector` and exposes `Run`/`Stop`. |
| `CollectorStatus` | `comp/otelcol/otlp/datatype` | Reports the collector state string and any error message. |

#### `PipelineConfig` fields

```go
type PipelineConfig struct {
    OTLPReceiverConfig           map[string]interface{}
    TracePort                    uint     // port used by the trace-agent OTLP receiver
    MetricsEnabled               bool
    TracesEnabled                bool
    LogsEnabled                  bool
    TracesInfraAttributesEnabled bool
    Logs                         map[string]interface{}
    Debug                        map[string]interface{}
    Metrics                      map[string]interface{}
    MetricsBatch                 map[string]interface{}
}
```

### Key functions

| Function | Description |
|----------|-------------|
| `FromAgentConfig(cfg)` | Builds a `PipelineConfig` from the Agent's `config.Reader`. Reads `otlp_config.*` keys and validates that at least one signal is enabled. |
| `NewPipeline(cfg, ...)` | Creates a `Pipeline` from an explicit `PipelineConfig`. Wires up receivers, processors, and exporters and returns an unstarted collector. |
| `NewPipelineFromAgentConfig(cfg, ...)` | Convenience wrapper: calls `FromAgentConfig` then `NewPipeline`. |
| `(*Pipeline).Run(ctx)` | Runs the collector until the context is cancelled. Panics are caught and stored as the pipeline error. |
| `(*Pipeline).Stop()` | Shuts down the collector. |
| `(*Pipeline).GetCollectorStatus()` | Returns a `CollectorStatus` with the current state string and any stored error. |

### Embedded OTel components

The pipeline is assembled in `getComponents()`:

- **Receiver**: `otlpreceiver` — accepts gRPC and HTTP OTLP traffic.
- **Processors**: `batchprocessor`, `infraattributesprocessor` (enriches spans/metrics/logs with infrastructure tags from the Tagger).
- **Exporters**:
  - `serializerexporter` — converts OTLP metrics to Datadog format and hands them to the Agent's `MetricSerializer`.
  - `logsagentexporter` — forwards OTLP logs to the logs pipeline channel.
  - `otlpexporter` — forwards traces to the trace-agent internal OTLP port.
  - `debugexporter` — optional debug output, controlled by `otlp_config.debug.verbosity`.

### Sub-packages

| Sub-package | Description |
|-------------|-------------|
| `components/exporter/serializerexporter` | OTLP metrics → Datadog `MetricSerializer`. |
| `components/exporter/logsagentexporter` | OTLP logs → logs pipeline channel. |
| `components/exporter/datadogexporter` | Full Datadog exporter used by DDOT (traces, metrics, logs via the Datadog API). |
| `components/processor/infraattributesprocessor` | Enriches all signal types with infrastructure attributes from the Tagger. |
| `components/metricsclient` | StatsD client wrapper used by the Datadog exporter to report internal metrics. |
| `configcheck` | Helpers to check whether OTLP is enabled and to read config sections. |
| `datatype` | Shared `CollectorStatus` struct. |
| `internal/configutils` | YAML-to-`confmap.Conf` utilities for building the hardcoded pipeline config map. |

### Configuration keys (datadog.yaml)

| Key | Description |
|-----|-------------|
| `otlp_config.receiver.*` | OTLP receiver protocols (grpc/http endpoints). |
| `otlp_config.metrics.enabled` | Enable/disable metrics ingestion. |
| `otlp_config.traces.enabled` | Enable/disable trace ingestion. |
| `otlp_config.logs.enabled` | Enable/disable log ingestion. |
| `otlp_config.traces.span_name_as_resource_name` | Span naming convention. |
| `otlp_config.debug.verbosity` | Debug exporter verbosity (`detailed`, `normal`, `basic`, `none`). |

## Usage

### Where it is used

The `otlp` package is consumed in two places:

1. **`comp/otelcol/collector/impl-pipeline`** (`fx-pipeline` module) — the
   standard Agent path. `NewComponent` calls `otlp.NewPipelineFromAgentConfig`
   on startup and runs the pipeline as a goroutine. It also provides a
   `FlareProvider` and a `StatusProvider` for the Agent's flare and status
   endpoints.

2. **`pkg/serverless/otlp`** — the serverless Agent path. Creates a pipeline
   directly without fx, using the same `NewPipelineFromAgentConfig` API.

### Typical startup sequence (core Agent)

```
collectorimpl.NewComponent (impl-pipeline)
  └─ otlp.NewPipelineFromAgentConfig
       ├─ FromAgentConfig          // parse datadog.yaml otlp_config.*
       ├─ NewPipeline              // build otelcol.Collector with hardcoded confmap
       │    ├─ getComponents()     // assemble factories
       │    └─ buildMap(cfg)       // build confmap from PipelineConfig
       └─ Pipeline.Run(ctx)        // start collector goroutine
```

### Build tags

The implementation files use `//go:build otlp`. The `configcheck` sub-package
provides a stub for builds without the tag so that the rest of the Agent can
still call `configcheck.IsEnabled` and `configcheck.IsDisplayed` safely.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/otelcol/collector`](collector.md) | The full OTel Collector component used in DDOT. The `impl-pipeline` variant wraps `comp/otelcol/otlp` and adds `FlareProvider`/`StatusProvider`. The `impl` (OTel Agent) variant uses a separate user-supplied config file instead of the hardcoded pipeline topology defined here. |
| [`comp/otelcol/logsagentpipeline`](logsagentpipeline.md) | Provides the downstream logs pipeline channel (`NextPipelineChan()`) that the `logsagentexporter` writes into. Must be enabled (`logs_enabled: true`) for OTLP log ingestion to work. A validation check in `comp/otelcol/otlp` returns an error if `otlp_config.logs.enabled` is `true` but no channel is available. |
| [`comp/logs/agent`](../logs/agent.md) | The full logs Agent, distinct from `logsagentpipeline`. The full agent manages collection sources; `logsagentpipeline` is a send-only pipeline used by OTLP. The `logsagentexporter` and the full logs agent share the same `pipeline.Provider` abstraction. |
| [`pkg/opentelemetry-mapping-go`](../../pkg/opentelemetry-mapping-go.md) | Provides the OTLP-to-Datadog translation layer consumed by `serializerexporter`. The `attributes.Translator` maps OTel resource attributes to Datadog hostnames and tags; `metrics.Provider` (via `NewDefaultTranslator`) translates OTLP metrics into Datadog time series and sketches. |
