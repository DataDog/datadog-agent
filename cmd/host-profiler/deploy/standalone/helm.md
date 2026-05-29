# Deploying Standalone: Using Helm Charts

Complete the [prerequisites](README.md#prerequisites) before continuing.

## Setup

Add the OpenTelemetry Helm chart repository:

```shell
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update
```

## Configure

In [`helm/pod-spec.yaml`](helm/pod-spec.yaml), update `image.tag` if you want a different image version, and set `DD_SITE` under `extraEnvs` to your Datadog site if you are not using `datadoghq.com`.

## Deploy

```shell
helm upgrade --install dd-host-profiler open-telemetry/opentelemetry-collector \
  --namespace dd-host-profiler \
  --values standalone/helm/pod-spec.yaml \
  --values standalone/helm/otel-config.yaml
```

On non-Cilium clusters:

```shell
kubectl apply -f standalone/network-policy.yaml
```

### Seccomp

The collector is automatically configured to run under a seccomp profile. An init container copies the profile from the collector image to every node at pod startup. No manual steps required.

### AppArmor (optional)

Load [`apparmor-profile`](../apparmor-profile) on each node using your cluster's AppArmor provisioning mechanism, then update `securityContext` in [`helm/pod-spec.yaml`](helm/pod-spec.yaml):

```yaml
securityContext:
  # ... existing fields ...
  appArmorProfile:
    type: Localhost
    localhostProfile: dd-host-profiler
```

### Cilium (optional)

On clusters with Cilium, replace the standard network policy with the Cilium one to get FQDN-scoped egress enforcement:

```shell
kubectl delete -f standalone/network-policy.yaml
kubectl apply -f standalone/cilium-network-policy.yaml
```
