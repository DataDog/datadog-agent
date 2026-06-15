# Deploying Host Profiler with the OpenTelemetry Helm chart

## Overview

Use this guide when your cluster does not run the Datadog Agent and you manage deployments with Helm. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently as an OpenTelemetry Collector DaemonSet and sends profiles directly to Datadog.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Before deploying, make sure you have:

- [OpenTelemetry Collector Helm chart](https://opentelemetry.io/docs/platforms/kubernetes/helm/collector/) version **0.152.1** or later.
- A namespace for the Host Profiler. You can reuse an existing namespace or create a dedicated one.
- A Datadog API key available to the Collector as `DD_API_KEY`.

The example values read `DD_API_KEY` from a Kubernetes Secret named `datadog-secret`, using the key `api-key`, in the same namespace as the Helm release. If you use another secret-management mechanism, adapt the configuration files accordingly.

> **Note:** Do not put the raw API key directly in Helm values or Collector configuration; those may be stored in the cluster.

## Adapt the configuration

Before deploying, update the provided configuration files for your environment:

1. In [`helm/pod-spec.yaml`](helm/pod-spec.yaml):
   - Set `DD_SITE` if your Datadog site is not `datadoghq.com`. See [Datadog sites](https://docs.datadoghq.com/getting_started/site/).
   - Adapt the `DD_API_KEY` secret reference if you do not use the example `datadog-secret` Kubernetes Secret.
   - Review the remaining pod settings. For all supported values, see the [OpenTelemetry Collector Helm chart values](https://github.com/open-telemetry/opentelemetry-helm-charts/blob/main/charts/opentelemetry-collector/values.yaml).

2. In [`helm/otel-config.yaml`](helm/otel-config.yaml):
   - Review the OpenTelemetry Collector pipelines and Datadog export configuration.
   - Adapt it like any other [OpenTelemetry Collector configuration](https://opentelemetry.io/docs/collector/configuration/).

3. Choose a network policy values file:
   - Use [`helm/network-policy.yaml`](helm/network-policy.yaml) by default.
   - If your cluster uses Cilium and you want FQDN-scoped egress enforcement, use [`helm/cilium-network-policy.yaml`](helm/cilium-network-policy.yaml) instead.

## Deploy

Deploy or update the OpenTelemetry Collector Helm release with the provided values files. Adapt this command to your Helm workflow and chosen namespace:

```shell
helm upgrade --install <RELEASE_NAME> open-telemetry/opentelemetry-collector \
  --namespace <NAMESPACE> \
  --values cmd/host-profiler/deploy/standalone/helm/pod-spec.yaml \
  --values cmd/host-profiler/deploy/standalone/helm/otel-config.yaml \
  --values cmd/host-profiler/deploy/standalone/helm/network-policy.yaml
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

## Verification

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
