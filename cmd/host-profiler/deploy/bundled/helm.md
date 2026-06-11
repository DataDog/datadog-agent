# Deploying Host Profiler with the Datadog Helm chart

## Overview

Use this guide when the Datadog Agent is already installed with Helm. If your Agent is managed by the Datadog Operator, use the [Datadog Operator guide](operator.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

Before running commands in this guide, change to the deployment docs directory from the repository root:

```shell
cd cmd/host-profiler/deploy
```

## Prerequisites

Deploy the Datadog Agent using Helm (chart version 3.220.0 or later). See the [installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

1. Add to your `values.yaml` (create the file if you don't have one):

```yaml
datadog:
  hostProfiler:
    enabled: true
    image: "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"
```

2. Upgrade:

```shell
helm upgrade --install datadog datadog/datadog \
  -f values.yaml \
  --reuse-values \
  -n datadog
```

The chart automatically configures all required capabilities and seccomp.

Wait for the rollout to complete:

```shell
kubectl rollout status daemonset/datadog -n datadog
```

## Configuration

The profiler infers [most of its configuration](https://github.com/DataDog/datadog-agent/tree/main/cmd/host-profiler#configuration-inference) from the core Agent. The following settings can be overridden in the Agent config file:

| Name | Values | Default | Description |
| :---- | :---- | :---- | :---- |
| `hostprofiler.debug.verbosity` | string | _(disabled)_ | Configures a [debug exporter](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/debugexporter/README.md) |
| `hostprofiler.additional_http_headers` | map[string]string | _(empty)_ | Adds additional headers to payloads |
| `hostprofiler.ddprofiling.enabled` | bool | `false` | Enables Go Runtime Profiler |
| `hostprofiler.ddprofiling.period` | int | `0` | Sampling rate |
| `hostprofiler.health_metrics.enabled` | bool | `true` | Enables collector internal metrics collection |
| `hostprofiler.health_metrics.target` | string | `127.0.0.1:8889` | Exposed Prometheus address |
| `hostprofiler.hpflare.port` | int | `7778` | Exposed port to retrieve Host Profiler flares |

## AppArmor (optional)

To use a custom AppArmor profile instead of `unconfined`, load [`apparmor-profile`](../apparmor-profile) on each node, then set:

```yaml
datadog:
  hostProfiler:
    apparmor: localhost/dd-host-profiler
```

## Verification

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
