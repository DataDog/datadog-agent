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

The following settings can be overridden in the Datadog Agent configuration for both [Datadog Helm chart](helm.md) and [Datadog Operator](operator.md) deployments:

| Name                                   | Values            | Default          | Description                                                                                                                         |
|:---------------------------------------|:------------------|:-----------------|:------------------------------------------------------------------------------------------------------------------------------------|
| `hostprofiler.debug.verbosity`         | string            | _(disabled)_     | Configures a [debug exporter](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/debugexporter/README.md) |
| `hostprofiler.additional_http_headers` | map[string]string | _(empty)_        | Adds additional headers to payloads                                                                                                 |
| `hostprofiler.ddprofiling.enabled`     | bool              | `false`          | Enables Go Runtime Profiler                                                                                                         |
| `hostprofiler.ddprofiling.period`      | int               | `0`              | Sampling rate                                                                                                                       |
| `hostprofiler.health_metrics.enabled`  | bool              | `true`           | Enables collector internal metrics collection                                                                                       |
| `hostprofiler.health_metrics.target`   | string            | `127.0.0.1:8889` | Exposed Prometheus address                                                                                                          |
| `hostprofiler.hpflare.port`            | int               | `7778`           | Exposed port to retrieve Host Profiler flares                                                                                       |
