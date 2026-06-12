# Deploying Host Profiler with the OpenTelemetry Helm chart

## Overview

Use this guide when the Datadog Agent is not installed and you want Helm to deploy the Host Profiler as its own OpenTelemetry Collector DaemonSet. If the Datadog Agent is already installed, use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently and sends profiles directly to Datadog.

Review the [supported environments](../README.md#supported-environments) before continuing.

Before running commands in this guide, change to the deployment docs directory from the repository root:

```shell
cd cmd/host-profiler/deploy
```

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

## Setup

Add the OpenTelemetry Helm chart repository:

```shell
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update
```

## Configuration

### Image and Datadog site

In [`helm/pod-spec.yaml`](helm/pod-spec.yaml), update `image.tag` if you want a different image version, and set `DD_SITE` under `extraEnvs` to your Datadog site if you are not using `datadoghq.com`.

### OpenTelemetry Collector configuration

The Collector configuration lives in [`helm/otel-config.yaml`](helm/otel-config.yaml) and can be configured like a regular Collector. See the [OpenTelemetry Collector configuration documentation](https://opentelemetry.io/docs/collector/configuration/).

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

The Collector is automatically configured to run under a seccomp profile. An init container copies the profile from the Collector image to every node at pod startup. No manual steps required.

### AppArmor (optional)

Load [`apparmor-profile`](../apparmor-profile) on each node using your cluster's AppArmor provisioning mechanism, then update `securityContext` in [`helm/pod-spec.yaml`](helm/pod-spec.yaml):

```yaml
securityContext:
  # ... existing fields ...
  appArmorProfile:
    type: Localhost
    localhostProfile: host-profiler
```

### Cilium (optional)

On clusters with Cilium, replace the standard network policy with the Cilium one to get FQDN-scoped egress enforcement:

```shell
kubectl delete -f standalone/network-policy.yaml
kubectl apply -f standalone/cilium-network-policy.yaml
```

## Verification

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
