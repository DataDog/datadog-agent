# Deploying Host Profiler with the OpenTelemetry Operator

## Overview

Use this guide when your cluster does not run the Datadog Agent and you use the OpenTelemetry Operator to manage Collector deployments. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently and sends profiles directly to Datadog. For cluster-wide host profiling, this guide uses the recommended OpenTelemetry Collector DaemonSet deployment.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Before deploying, make sure you have:

- [OpenTelemetry Operator](https://opentelemetry.io/docs/kubernetes/operator/) installed.
- A namespace for the Host Profiler. You can reuse an existing namespace or create a dedicated one.
- A Datadog API key available to the Collector as `DD_API_KEY`.

The example manifests read `DD_API_KEY` from a Kubernetes Secret named `datadog-secret`, using the key `api-key`, in the same namespace as the `OpenTelemetryCollector` Custom Resource. If you use another secret-management mechanism, adapt [`operator/collector.yaml`](operator/collector.yaml) accordingly.

> **Note:** Do not put the raw API key directly in Collector configuration; it may be stored in the cluster.

## Adapt the manifests

Before deploying, update the provided manifests for your environment:

1. In [`operator/rbac.yaml`](operator/rbac.yaml):
   - If you use a namespace other than `host-profiler`, update the `ClusterRoleBinding` subject namespace.
   - If you change the `OpenTelemetryCollector` name, update the service account name in the `ClusterRoleBinding` subject.

2. In [`operator/collector.yaml`](operator/collector.yaml):
   - Set `metadata.namespace` to your chosen namespace.
   - Set `DD_SITE` if your Datadog site is not `datadoghq.com`. See [Datadog sites](https://docs.datadoghq.com/getting_started/site/).
   - Adapt the `DD_API_KEY` secret reference if you do not use the example `datadog-secret` Kubernetes Secret.
   - Review the OpenTelemetry Collector configuration under `spec.config`. Adapt it like any other [OpenTelemetry Collector configuration](https://opentelemetry.io/docs/collector/configuration/).

3. Choose a network policy manifest:
   - If your cluster enforces Kubernetes NetworkPolicy, use [`operator/network-policy.yaml`](operator/network-policy.yaml) by default.
   - If your cluster uses Cilium and you want FQDN-scoped egress enforcement, use [`operator/cilium-network-policy.yaml`](operator/cilium-network-policy.yaml) instead.
   - If you change the namespace or `OpenTelemetryCollector` name, update the policy metadata and pod selectors.

If your cluster does not enforce NetworkPolicy resources, these manifests do not restrict egress; use your cluster's supported network controls instead.

## Deploy

```shell
kubectl apply -f standalone/operator/rbac.yaml
kubectl apply -f standalone/operator/collector.yaml
```

On non-Cilium clusters:

```shell
kubectl apply -f standalone/operator/network-policy.yaml
```

The Operator deployment uses two manifests:

1. [`operator/rbac.yaml`](operator/rbac.yaml): RBAC needed for node, kubelet, and Kubernetes metadata used during profile enrichment.
2. [`operator/collector.yaml`](operator/collector.yaml): the `OpenTelemetryCollector` Custom Resource that runs the Host Profiler as a DaemonSet.

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
kubectl delete -f standalone/operator/network-policy.yaml
kubectl apply -f standalone/operator/cilium-network-policy.yaml
```

## Verification

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
