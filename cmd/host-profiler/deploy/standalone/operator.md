# Deploying Standalone: Using the OTel Operator

Complete the [prerequisites](README.md#prerequisites) before continuing.

If the OpenTelemetry Operator is not installed, follow the [official installation guide](https://opentelemetry.io/docs/kubernetes/operator/).

## Configure

Set your Datadog site in [`operator/collector.yaml`](operator/collector.yaml) (default: `datadoghq.com`):

```yaml
env:
  - name: DD_SITE
    value: "datadoghq.com"
```

## Deploy

```shell
kubectl apply -f standalone/operator/collector.yaml
```

[`operator/collector.yaml`](operator/collector.yaml) contains three resources:

1. A `ClusterRole` granting the collector's service account access to the kubelet APIs needed for hostname resolution.
2. A `ClusterRoleBinding` attaching that role to the `dd-host-profiler-collector` service account the Operator creates.
3. The `OpenTelemetryCollector` CR defining the DaemonSet.

The Operator reconciles the CR and creates the DaemonSet.

### Cilium (optional)

On clusters with Cilium, apply the Cilium network policy to restrict egress to Datadog endpoints:

```shell
kubectl apply -f cilium-network-policy.yaml
```
