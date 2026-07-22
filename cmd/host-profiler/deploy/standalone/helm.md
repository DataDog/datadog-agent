# Deploying Host Profiler with the OpenTelemetry Helm chart

## Overview

Use this guide when your cluster does not run the Datadog Agent and you manage deployments with Helm. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently and sends profiles directly to Datadog. For cluster-wide host profiling, this guide uses the recommended OpenTelemetry Collector DaemonSet deployment.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Before deploying, make sure you have:

- [OpenTelemetry Collector Helm chart](https://opentelemetry.io/docs/platforms/kubernetes/helm/collector/) version **0.152.1** or later.
- A namespace for the Host Profiler. You can reuse an existing namespace or create a dedicated one.
- A Datadog API key available to the Collector as `DD_API_KEY`.

The example values read `DD_API_KEY` from a Kubernetes Secret named `datadog-secret`, using the key `api-key`, in the same namespace as the Helm release. If you use another secret-management mechanism, adapt the configuration files accordingly.

> **Note:** Do not put the raw API key directly in Helm values or Collector configuration; those may be stored in the cluster.

## Adapt the Helm values files

Before deploying, update the provided Helm values files for your environment. These files are passed to the OpenTelemetry Collector Helm chart with `--values`:

1. In [`helm/collector-values.yaml`](helm/collector-values.yaml):
   - Set `DD_SITE` if your Datadog site is not `datadoghq.com`. See [Datadog sites](https://docs.datadoghq.com/getting_started/site/).
   - Adapt the `DD_API_KEY` secret reference if you do not use the example `datadog-secret` Kubernetes Secret.
   - To use another Datadog container registry, replace the `registry.datadoghq.com` prefix in the Host Profiler image with your preferred registry prefix. See [Changing your container registry](https://docs.datadoghq.com/containers/guide/changing_container_registry/).
   - Review the remaining pod settings, including resource requests and limits. For all supported values, see the [OpenTelemetry Collector Helm chart values](https://github.com/open-telemetry/opentelemetry-helm-charts/blob/main/charts/opentelemetry-collector/values.yaml). For expected overhead, default limits, and tuning guidance, see [Overhead and resource usage](../faq.md#what-overhead-should-i-expect).

2. In [`helm/collector-config-values.yaml`](helm/collector-config-values.yaml):
   - Review the OpenTelemetry Collector pipelines and Datadog export configuration.
   - Adapt it like any other [OpenTelemetry Collector configuration](https://opentelemetry.io/docs/collector/configuration/).

3. Choose a network policy values file:
   - If your cluster enforces Kubernetes NetworkPolicy, use [`helm/network-policy-values.yaml`](helm/network-policy-values.yaml) by default.
   - If your cluster uses Cilium and you want FQDN-scoped egress enforcement, use [`helm/cilium-network-policy-values.yaml`](helm/cilium-network-policy-values.yaml) instead.

If your cluster does not enforce NetworkPolicy resources, these values do not restrict egress; use your cluster's supported network controls instead.

## Deploy

Deploy or update the OpenTelemetry Collector Helm release with the provided values files. Adapt this command to your Helm workflow and chosen namespace.

The example below uses the Kubernetes NetworkPolicy values file. If your cluster uses Cilium, replace `helm/network-policy-values.yaml` with `helm/cilium-network-policy-values.yaml` before running it.

```shell
helm upgrade --install <RELEASE_NAME> open-telemetry/opentelemetry-collector \
  --namespace <NAMESPACE> \
  --values helm/collector-values.yaml \
  --values helm/collector-config-values.yaml \
  --values helm/network-policy-values.yaml
```

The provided Helm values configure the required capabilities and seccomp profile automatically. An init container installs the seccomp profile onto each node, so no manual seccomp setup is required.

After you apply the values, Helm rolls out an OpenTelemetry Collector DaemonSet with the Host Profiler. Wait for that rollout to complete before verifying profiles.

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.

## AppArmor (optional)

AppArmor provides extra hardening on Linux distributions and Kubernetes clusters where AppArmor is available. The Host Profiler does not require AppArmor to run.

Use this section only if your nodes support AppArmor and you already manage node-local AppArmor profiles. AppArmor profiles must be loaded on each node before Kubernetes can apply them to a pod.

To enable the provided profile, load [`apparmor-profile`](../apparmor-profile) on each node, then update `securityContext` in [`helm/collector-values.yaml`](helm/collector-values.yaml):

```yaml
securityContext:
  # ... existing fields ...
  appArmorProfile:
    type: Localhost
    localhostProfile: host-profiler
```

The provided profile limits what the Host Profiler container can execute. It allows `objcopy`, which is used for debug symbol extraction.
