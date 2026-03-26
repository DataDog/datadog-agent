# pkg/security/telemetry

## Purpose

Provides the CWS (Cloud Workload Security) side of container-running telemetry. It periodically samples the list of containers currently running on the host and emits a gauge metric per container. This is used for CWS/CSPM metering — the metric drives billing-related dashboards, so changes to this package require extra care.

## Key elements

### Types

| Type | Description |
|------|-------------|
| `ContainersTelemetry` | Low-level helper. Holds a `SimpleTelemetrySender`, a `workloadmeta.Component` store, and a container filter. Exposes `ListRunningContainers()` and `ReportContainers(metricName string)`. |
| `ContainersRunningTelemetryConfig` | Config struct carrying two booleans: `RuntimeEnabled` (CWS runtime) and `FIMEnabled`. |
| `ContainersRunningTelemetry` | Linux-only higher-level wrapper. Reads the config, picks the right metric name (plain vs Fargate variant), and calls `ReportContainers` on a one-minute ticker via `Run(ctx context.Context)`. |
| `SimpleTelemetrySender` | Minimal interface (`Gauge`, `Commit`) abstracting either a statsd client or a `SenderManager` default sender. |

### Functions

| Function | Description |
|----------|-------------|
| `NewContainersTelemetry(sender, wmeta, filterBundle)` | Constructor; returns an error if the container filter has configuration errors. |
| `NewContainersRunningTelemetry(cfg, statsdClient, wmeta, filterStore)` | Linux-only constructor. Bridges the workloadfilter component to `ContainersTelemetry`. |
| `NewSimpleTelemetrySenderFromStatsd(sci)` | Wraps a `statsd.ClientInterface` as a `SimpleTelemetrySender`. |

### Build flags

`containers_running_telemetry_linux.go` carries no explicit build tag but is named `_linux.go`, so the `ContainersRunningTelemetry` type and its `Run` loop are Linux-only. `containers_running_telemetry_others.go` provides a stub for non-Linux platforms.

## Usage

`ContainersRunningTelemetry.Run` is started as a goroutine in `pkg/security/module/cws.go` and in `pkg/compliance/agent.go`. The container filter is obtained from the `workloadfilter` component (`filterStore.GetContainerRuntimeSecurityFilters()`), which excludes paused containers and agent-owned containers automatically.

`ReportContainers` emits one gauge point per running, non-excluded container with tags `container_id:<id>` and a cardinality tag (`constants.CardinalityTagPrefix + "orch"`). The metric name is chosen by the caller from constants in `pkg/security/metrics` (e.g. `MetricSecurityAgentRuntimeContainersRunning`).

Key invariant: Datadog agent containers (env var `DOCKER_DD_AGENT=yes|true`) are always excluded.
