# Configuring Host Profiler with the Datadog Agent

The bundled Host Profiler runs as a sidecar in the Datadog Agent DaemonSet. It infers most configuration from the Datadog Agent configuration.

## Datadog intake configuration

In bundled deployments, the Host Profiler uses the Datadog Agent configuration to determine where to send profiles and debug symbols. In most cases, you do not need to configure the Datadog site, API key, or profiling intake endpoints separately for the Host Profiler.

For the main profiling destination, the Host Profiler extracts the Datadog site from `apm_config.profiling_dd_url` when set. This setting is not a literal Host Profiler export URL. If it is not set, the Host Profiler uses the Agent's `site`, or `datadoghq.com` if `site` is also unset. The Agent's `api_key` provides credentials for the main site, and `apm_config.profiling_send_to_main_endpoint` controls whether the Host Profiler includes this destination.

Entries in `apm_config.profiling_additional_endpoints` provide additional Datadog sites and API keys. Their URL keys identify the sites; they are not literal routing URLs. The resolved destinations are used for profile export and debug symbol upload.

## Optional overrides

Most bundled deployments do not need these settings. Use them to expose Host Profiler health data, collect diagnostics, or follow instructions from Datadog Support.

The following settings can be overridden in the Datadog Agent configuration for both [Datadog Helm chart](helm.md) and [Datadog Operator](operator.md) deployments.

### Health metrics

| Name                                  | Values | Default          | Description                                                                                                                                      |
|:--------------------------------------|:-------|:-----------------|:-------------------------------------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.health_metrics.enabled` | bool   | `false`          | Sends internal Host Profiler health metrics to Datadog. These may count as [custom metrics](https://docs.datadoghq.com/metrics/custom_metrics/). Only sent when [`infrastructure_mode`](https://docs.datadoghq.com/agent/configuration/infrastructure-modes/) is `full` or `basic`. |
| `hostprofiler.health_metrics.target`  | string | `127.0.0.1:8889` | Address used for the Host Profiler internal Prometheus metrics endpoint. Change this only if the default address conflicts with another service. |

### Diagnostics

| Name                           | Values                        | Default                              | Description                                                                                                                                                                                                                                                          |
|:-------------------------------|:------------------------------|:-------------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.hpflare.port`    | int                           | `7778`                               | Local port used to collect Host Profiler flare diagnostics. Change this only if the default port conflicts with another service.                                                                                                                                     |
| `hostprofiler.debug.exporter.verbosity` | `basic`, `normal`, `detailed` | _(disabled)_                         | Enables the OpenTelemetry debug exporter for troubleshooting. See the [debug exporter verbosity documentation](https://pkg.go.dev/go.opentelemetry.io/collector/exporter/debugexporter#readme-verbosity-levels). Use temporarily because it can increase log volume. |
| `hostprofiler.debug.ebpf_logs` | bool | `false` | Enables verbose eBPF profiler logging. |
| `hostprofiler.log_file`         | string                        | `${log_path}/host-profiler.log`      | Path to the Host Profiler log file. `${log_path}` resolves to the Agent's configured log directory.                                                                                                                                                                  |

### Advanced export settings

| Name                                   | Values            | Default   | Description                                                                                                |
|:---------------------------------------|:------------------|:----------|:-----------------------------------------------------------------------------------------------------------|
| `hostprofiler.reporter_interval` | duration | `60s` | Interval between profile reports. Must be at least `60s`; shorter intervals are rejected. |
| `hostprofiler.additional_http_headers` | map[string]string | _(empty)_ | Adds custom headers to profile export requests, for example when required by an outbound proxy or gateway. |
| `hostprofiler.send_error_frames` | bool   | `false` | Includes unwind error frames in profiles for troubleshooting. |
| `hostprofiler.include_env_vars` | string (comma-separated) | _(empty)_ | Collects additional environment variables from profiled processes without replacing the default metadata collection. Only include variables whose values are safe to send with profiles; do not include secrets. |
| `hostprofiler.filter_min_process_age` | duration | `0s` | Skips profiling processes younger than this duration, to filter out short-lived processes. `0s` disables the filter. |

### Symbol uploader

| Name | Values | Default | Description |
|:-----|:-------|:--------|:------------|
| `hostprofiler.symbol_uploader.enabled` | bool | `true` | Uploads debug symbols to Datadog using the Agent's profiling intake destinations and API keys. Disabling it can leave native and Go frames unresolved. |
| `hostprofiler.symbol_uploader.report.interval`   | int    | `300`   | Interval, in seconds, at which the Host Profiler logs symbol upload reports. Maximum `86400` (24 hours). Values outside `1` to `86400` are ignored and fall back to the default. |
| `hostprofiler.symbol_uploader.report.max_paths_per_report` | int | `100` | Maximum number of binary paths kept per symbol upload report, per log level. Maximum `100000`. Values outside `1` to `100000` fall back to the default. |

### Host Profiler self-profiling

These options are for Datadog Support diagnostics only. Leave self-profiling disabled unless Datadog Support asks you to enable it.

| Name                               | Values | Default                   | Description                                                                                                        |
|:-----------------------------------|:-------|:--------------------------|:-------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.ddprofiling.enabled` | bool   | `false`                   | Enables Datadog profiling for the Host Profiler process itself. This does not control profiling of your workloads. |
| `hostprofiler.ddprofiling.period`  | int    | `60` seconds when enabled | Self-profiling collection interval. Used only when `hostprofiler.ddprofiling.enabled` is `true`.                   |
| `hostprofiler.ddprofiling.port`    | int    | `7501`                    | Local port used by the self-profiling HTTP server. Used only when `hostprofiler.ddprofiling.enabled` is `true`. Change this only if the default port conflicts with another service. |
