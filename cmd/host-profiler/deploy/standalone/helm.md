# Deploying Standalone: Using Helm Charts

Complete the [prerequisites](README.md#prerequisites) before continuing.

## Setup

Add the OpenTelemetry Helm chart repository:

```shell
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update
```

## Configure

Set your Datadog site in [`helm/pod-spec.yaml`](helm/pod-spec.yaml) (default: `datadoghq.com`):

```yaml
extraEnvs:
  - name: DD_SITE
    value: "datadoghq.com"
```

## Deploy

```shell
helm upgrade --install dd-host-profiler open-telemetry/opentelemetry-collector \
  --namespace dd-host-profiler \
  --values standalone/helm/pod-spec.yaml \
  --values standalone/helm/otel-config.yaml
```

### Cilium (optional)

On clusters with Cilium, apply the Cilium network policy to restrict egress to Datadog endpoints:

```shell
kubectl apply -f cilium-network-policy.yaml
```
