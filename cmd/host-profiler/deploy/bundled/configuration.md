# Configuring Host Profiler with the Datadog Agent

The bundled Host Profiler runs as a sidecar in the Datadog Agent DaemonSet. It infers most configuration from the Datadog Agent configuration.

## Datadog intake configuration

In bundled deployments, the Host Profiler uses the Datadog Agent configuration to determine where to send profiles and debug symbols. In most cases, you do not need to configure the Datadog site, API key, or profiling intake endpoints separately for the Host Profiler.

The Host Profiler uses:

- `apm_config.profiling_dd_url`, when set, as the preferred profiling intake URL;
- `site`, when `apm_config.profiling_dd_url` is not set;
- `api_key` for the main Datadog site;
- `apm_config.profiling_additional_endpoints`, when set, to send profiles to additional Datadog sites with their respective API keys.

If your Agent is configured to send profiles to multiple Datadog endpoints, the Host Profiler uses matching destinations for profile export and debug symbol upload.

## Optional overrides

Most bundled deployments do not need these settings. Use them to expose Host Profiler health data, collect diagnostics, or follow instructions from Datadog Support.

The following settings can be overridden in the Datadog Agent configuration for both [Datadog Helm chart](helm.md) and [Datadog Operator](operator.md) deployments.

### Health metrics

| Name                                  | Values | Default          | Description                                                                                                                                      |
|:--------------------------------------|:-------|:-----------------|:-------------------------------------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.health_metrics.enabled` | bool   | `true`           | Sends internal Host Profiler health metrics to Datadog.                                                                                          |
| `hostprofiler.health_metrics.target`  | string | `127.0.0.1:8889` | Address used for the Host Profiler internal Prometheus metrics endpoint. Change this only if the default address conflicts with another service. |

### Diagnostics

| Name                           | Values                        | Default      | Description                                                                                                                                                                                                                                                          |
|:-------------------------------|:------------------------------|:-------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.hpflare.port`    | int                           | `7778`       | Local port used to collect Host Profiler flare diagnostics. Change this only if the default port conflicts with another service.                                                                                                                                     |
| `hostprofiler.debug.verbosity` | `basic`, `normal`, `detailed` | _(disabled)_ | Enables the OpenTelemetry debug exporter for troubleshooting. See the [debug exporter verbosity documentation](https://pkg.go.dev/go.opentelemetry.io/collector/exporter/debugexporter#readme-verbosity-levels). Use temporarily because it can increase log volume. |

### Advanced export settings

| Name                                   | Values            | Default   | Description                                                                                                |
|:---------------------------------------|:------------------|:----------|:-----------------------------------------------------------------------------------------------------------|
| `hostprofiler.additional_http_headers` | map[string]string | _(empty)_ | Adds custom headers to profile export requests, for example when required by an outbound proxy or gateway. |

### Host Profiler self-profiling

These options are for Datadog Support diagnostics only. Leave self-profiling disabled unless Datadog Support asks you to enable it.

| Name                               | Values | Default                   | Description                                                                                                        |
|:-----------------------------------|:-------|:--------------------------|:-------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.ddprofiling.enabled` | bool   | `false`                   | Enables Datadog profiling for the Host Profiler process itself. This does not control profiling of your workloads. |
| `hostprofiler.ddprofiling.period`  | int    | `60` seconds when enabled | Self-profiling collection interval. Used only when `hostprofiler.ddprofiling.enabled` is `true`.                   |
| `hostprofiler.ddprofiling.port`    | int    | `7501`                    | Local port used by the self-profiling HTTP server. Used only when `hostprofiler.ddprofiling.enabled` is `true`. Change this only if the default port conflicts with another service. |
