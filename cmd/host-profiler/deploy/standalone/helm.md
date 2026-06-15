# Deploying Host Profiler with the OpenTelemetry Helm chart

## Overview

Use this guide when your cluster does not run the Datadog Agent and you manage deployments with Helm. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently as an OpenTelemetry Collector DaemonSet and sends profiles directly to Datadog.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

This guide requires the [OpenTelemetry Collector Helm chart](https://opentelemetry.io/docs/platforms/kubernetes/helm/collector/) version **0.152.1** or later from the `open-telemetry` Helm repository: `https://open-telemetry.github.io/opentelemetry-helm-charts`.

Create the required Kubernetes resources before deploying:

- A namespace for the Host Profiler. You can reuse an existing namespace or create a dedicated one.
- A Datadog API key exposed to the Collector as `DD_API_KEY`. The example values read it from a Kubernetes Secret named `datadog-secret` with an `api-key` key in the same namespace as the Helm release. If you use another secret-management mechanism, adapt [`helm/pod-spec.yaml`](helm/pod-spec.yaml) accordingly.

Do not put the raw API key directly in Helm values or Collector configuration; those may be stored in the cluster.

## Adapt the configuration

Before deploying, adapt the provided configuration files to your environment:

- [`helm/pod-spec.yaml`](helm/pod-spec.yaml): pod-level settings. Review the file, and **adapt the `DD_API_KEY` secret reference and `DD_SITE`** before deploying. See [Datadog sites](https://docs.datadoghq.com/getting_started/site/) for the correct `DD_SITE` value.
- [`helm/otel-config.yaml`](helm/otel-config.yaml): OpenTelemetry Collector pipelines and Datadog export. Review the file and adapt it like any other [OpenTelemetry Collector configuration](https://opentelemetry.io/docs/collector/configuration/).

Choose the network policy manifest that matches your cluster networking:

- If your cluster uses Cilium, use [`cilium-network-policy.yaml`](cilium-network-policy.yaml) for FQDN-scoped egress enforcement.
- Otherwise, use [`network-policy.yaml`](network-policy.yaml).

If you use a namespace other than `dd-host-profiler`, update the policy namespace before applying it. If you change the generated resource names in [`helm/pod-spec.yaml`](helm/pod-spec.yaml), update the policy selectors too.

## Deploy

Deploy or update the OpenTelemetry Collector Helm release with the provided values files. Adapt this command to your Helm workflow and chosen namespace:

```shell
helm upgrade --install <RELEASE_NAME> open-telemetry/opentelemetry-collector \
  --namespace <NAMESPACE> \
  --values cmd/host-profiler/deploy/standalone/helm/pod-spec.yaml \
  --values cmd/host-profiler/deploy/standalone/helm/otel-config.yaml
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
