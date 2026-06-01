# Deploying Bundled

The host profiler runs as a sidecar in the Datadog Agent DaemonSet.

- **[Datadog Operator](operator.md)**: add annotations to your existing `DatadogAgent` CR.
- **[Helm Charts](helm.md)**: add `hostProfiler` values to your existing Datadog Helm release.

## Configuring the profiler

The profiler infers [most of its configuration](https://github.com/DataDog/datadog-agent/tree/main/cmd/host-profiler#configuration-inference) from the core agent. The following settings can be overridden in the agent's config file:

| Name | Values | Default | Description |
| :---- | :---- | :---- | :---- |
| `hostprofiler.debug.verbosity` | string | _(disabled)_ | Configures a [debug exporter](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/debugexporter/README.md) |
| `hostprofiler.additional_http_headers` | map[string]string | _(empty)_ | Adds additional headers to payloads |
| `hostprofiler.ddprofiling.enabled` | bool | `false` | Enables Go Runtime Profiler |
| `hostprofiler.ddprofiling.period` | int | `0` | Sampling rate |
| `hostprofiler.health_metrics.enabled` | bool | `true` | Enables collector internal metrics collection |
| `hostprofiler.health_metrics.target` | string | `127.0.0.1:8889` | Exposed Prometheus address |
| `hostprofiler.hpflare.port` | int | `7778` | Exposed port to retrieve Host Profiler flares |

## Verification

After deploying the host profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
