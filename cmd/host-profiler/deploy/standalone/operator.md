# Deploying Host Profiler with the OpenTelemetry Operator

## Overview

Use this guide when the Datadog Agent is not installed and you want the OpenTelemetry Operator to deploy the Host Profiler as its own OpenTelemetry Collector DaemonSet. If the Datadog Agent is already installed, use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

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

3. If the OpenTelemetry Operator is not installed, follow the [official installation guide](https://opentelemetry.io/docs/kubernetes/operator/).

## Configuration

### Image and Datadog site

In [`operator/collector.yaml`](operator/collector.yaml), update `spec.image` if you want a different image version, and set `DD_SITE` under `spec.env` to your Datadog site if you are not using `datadoghq.com`.

### OpenTelemetry Collector configuration

The Collector configuration lives in [`operator/collector.yaml`](operator/collector.yaml) under `spec.config` and can be configured like a regular Collector. See the [OpenTelemetry Collector configuration documentation](https://opentelemetry.io/docs/collector/configuration/).

## Deploy

```shell
kubectl apply -f standalone/operator/collector.yaml
```

On non-Cilium clusters:

```shell
kubectl apply -f standalone/network-policy.yaml
```

[`operator/collector.yaml`](operator/collector.yaml) contains three resources:

1. A `ClusterRole` granting the collector's service account access to the kubelet APIs needed for hostname resolution.
2. A `ClusterRoleBinding` attaching that role to the `dd-host-profiler-collector` service account the Operator creates.
3. The `OpenTelemetryCollector` Custom Resource defining the DaemonSet.

The Operator reconciles the Custom Resource and creates the DaemonSet.

### Seccomp

The Collector is automatically configured to run under a seccomp profile. An init container copies the profile from the Collector image to every node at pod startup. No manual steps required.

### AppArmor (optional)

Load [`apparmor-profile`](../apparmor-profile) on each node using your cluster's AppArmor provisioning mechanism, then update `securityContext` in [`operator/collector.yaml`](operator/collector.yaml):

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
