# Deploying Standalone

Standalone mode runs the Host Profiler as a DaemonSet, one pod per node, without the Datadog Agent.

Choose based on what your cluster already uses:

- **[Helm](helm.md)**: uses the `open-telemetry/opentelemetry-collector` Helm chart with a Datadog-provided values override.
- **[OTel Operator](operator.md)**: uses an `OpenTelemetryCollector` Custom Resource (CR) managed by the OpenTelemetry Operator.

## Prerequisites

1. Apply [`prerequisites.yaml`](../prerequisites.yaml) to create the `dd-host-profiler` namespace:

```shell
kubectl apply -f prerequisites.yaml
```

2. Create a Kubernetes secret with your Datadog API key:

```shell
kubectl create secret generic datadog-secret \
  --from-literal=api-key="$DD_API_KEY" \
  --namespace dd-host-profiler
```

## Verification

After deploying the host profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
