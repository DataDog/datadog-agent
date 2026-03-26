# comp/core/agenttelemetry — Agent Self-Telemetry Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/agenttelemetry`
**Team:** agent-runtimes
**Importers:** ~13 packages

## Purpose

`comp/core/agenttelemetry` (atel) collects internal Prometheus metrics from the agent's own `telemetry.Component` and periodically ships them to the Datadog backend as agent telemetry payloads. This gives Datadog visibility into how the agent is performing: check execution times, log throughput, DogStatsD packet rates, and more.

It also supports sending one-off structured event payloads (e.g. crash reports) and tracing the agent startup sequence using the lightweight `installertelemetry` tracer.

The component is entirely self-contained: it uses its own HTTP sender and internal scheduler. It does not go through the agent's normal metric pipeline.

## Package layout

| Package | Role |
|---|---|
| `comp/core/agenttelemetry/def` | `Component` interface |
| `comp/core/agenttelemetry/impl` | `atel` struct, metric collection, aggregation, sender, runner, config |
| `comp/core/agenttelemetry/fx` | `Module()` — wires `NewComponent` into fx |

## Component interface

```go
type Component interface {
    // Send a registered event payload to the Datadog backend.
    // eventType must be declared in a profile's events[] list in config.
    SendEvent(eventType string, eventPayload []byte) error

    // Start a startup span for tracing agent initialization.
    // Returns a no-op span when telemetry is disabled.
    StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context)
}
```

## Configuration

Agent telemetry is enabled by default when `agent_telemetry.enabled` is `true` in `datadog.yaml` (controlled by `pkg/config/utils.IsAgentTelemetryEnabled`). When enabled without explicit profiles in the configuration, the component uses a built-in default profile set.

### Profiles

A profile groups a set of metrics (or events) with a collection schedule. Profiles are defined under `agent_telemetry.profiles` in `datadog.yaml`, or the default built-in profiles are used.

Key profile fields:

| Field | Description |
|---|---|
| `name` | Profile identifier |
| `metric.metrics[].name` | Metric to collect, in `<subsystem>.<name>` format (e.g. `checks.execution_time`) |
| `metric.metrics[].aggregate_tags` | Tags to keep; all other tag dimensions are summed together |
| `metric.metrics[].aggregate_total` | Add an extra `total` timeseries with the count of merged timeseries |
| `metric.exclude.zero_metric` | Drop timeseries with zero value |
| `schedule.start_after` | Seconds to wait after start before first collection (default 30) |
| `schedule.period` | Seconds between collections (default 900 / 15 min) |
| `events[]` | Named event types the profile can receive via `SendEvent` |

### Built-in default profiles

The default configuration ships many profiles including: `checks`, `logs-and-metrics`, `database`, `synthetics`, `connectivity`, `ondemand` (crash events), `service-discovery`, `runtime-started`, `trace-agent`, `otlp`, `gpu`, `cluster-agent`, `injector`, and others.

### Metric naming convention

Metrics must be declared using the standard `telemetry.NewGauge(subsystem, name, ...)` API **without** `Options.NoDoubleUnderscoreSep`. The Prometheus name will be `<subsystem>__<name>`, which atel maps to the profile's `<subsystem>.<name>` format.

## fx wiring

```go
agenttelemetryfx.Module(),
```

The module uses `fxutil.ProvideOptional` so it is silently absent when not needed. It also registers a `/metadata/agent-telemetry` GET endpoint on the agent API that returns the current telemetry payload as JSON for inspection.

Dependencies injected by fx: `config.Component`, `log.Component`, `telemetry.Component`.

`comp/core/agenttelemetry` does **not** use `comp/forwarder/defaultforwarder`. It has its own lightweight HTTP sender that posts directly to the Datadog backend using the API key from `config.Component`. This means agent-telemetry payloads bypass the normal retry queue and disk-spill logic of the default forwarder. See [`comp/forwarder/defaultforwarder`](../forwarder/defaultforwarder.md) for the forwarder used by the regular metric pipeline.

## Startup tracing

On start, atel creates an `agent.startup` span using the `installertelemetry` lightweight tracer. The span automatically finishes after 1 minute or when the component stops. Callers can start child spans:

```go
span, ctx := atel.StartStartupSpan("my_init_operation")
defer span.Finish(nil)
// ... initialization work using ctx ...
```

The sampling rate is controlled by `agent_telemetry.startup_trace_sampling` (float, 0–1).

## Usage across the codebase

- **`cmd/agent`** — main agent; includes the full default profile set
- **`cmd/cluster-agent`** and **`cmd/cluster-agent-cloudfoundry`** — cluster agent
- **`cmd/trace-agent`** — APM trace agent
- **`cmd/otel-agent`** — OpenTelemetry collector agent
- **`comp/checks/agentcrashdetect`** (Windows) — calls `SendEvent("agentbsod", ...)` to report Windows agent crashes
- **`comp/collector/collectorimpl`** — uses `StartStartupSpan` to trace check loading
- **`comp/otelcol/collector`** — uses `StartStartupSpan` to trace OTLP collector startup

## Relationship to other telemetry components

| Component | Relationship |
|---|---|
| [`comp/core/telemetry`](telemetry.md) | Source of all Prometheus metrics. `agenttelemetry` calls `telemetry.Component.Gather(true)` to collect the current metric snapshot before each send. |
| [`pkg/telemetry`](../../../pkg/telemetry.md) | Package-level shim used by `pkg/` code to register metrics. Metrics declared here (without `Options.NoDoubleUnderscoreSep`) appear in agenttelemetry profiles using the `subsystem.name` dot-notation. |
| [`comp/forwarder/defaultforwarder`](../forwarder/defaultforwarder.md) | The normal agent metric pipeline forwarder. `agenttelemetry` does **not** use it — it has its own independent HTTP sender with no retry queue. |

Metrics must be declared **without** `Options.NoDoubleUnderscoreSep` (the default) so that the Prometheus name `subsystem__name` maps correctly to the profile's `subsystem.name` key. Metrics declared with `NoDoubleUnderscoreSep: true` will not match any profile entry and will be silently dropped.
