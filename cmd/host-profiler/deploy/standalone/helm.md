# Deploying Host Profiler with the OpenTelemetry Helm chart

## Overview

Use this guide when your cluster does not run the Datadog Agent and you manage deployments with Helm. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent guides from the [deployment overview](../README.md#deployment) instead.

The Host Profiler runs independently as an OpenTelemetry Collector DaemonSet and sends profiles directly to Datadog.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

This guide requires the [OpenTelemetry Collector Helm chart](https://opentelemetry.io/docs/platforms/kubernetes/helm/collector/) version **0.152.1** or later.

Create the required Kubernetes resources before deploying:

- A namespace for the Host Profiler. You can reuse an existing namespace or create a dedicated one.
- Secret `datadog-secret` in that namespace, with an `api-key` key containing your Datadog API key.

## Setup

Use the OpenTelemetry Collector Helm chart from the `open-telemetry` Helm repository: `https://open-telemetry.github.io/opentelemetry-helm-charts`.

## Configuration

### Image and Datadog site

In [`helm/pod-spec.yaml`](helm/pod-spec.yaml), update `image.tag` if you want a different image version, and set `DD_SITE` under `extraEnvs` to your Datadog site if you are not using `datadoghq.com`.

### OpenTelemetry Collector configuration

The Collector configuration lives in [`helm/otel-config.yaml`](helm/otel-config.yaml) and can be configured like a regular Collector. See the [OpenTelemetry Collector configuration documentation](https://opentelemetry.io/docs/collector/configuration/).

## Deploy

Deploy or update the OpenTelemetry Collector Helm release with the provided values files. Adapt this command to your Helm workflow and chosen namespace:

```shell
helm upgrade --install <RELEASE_NAME> open-telemetry/opentelemetry-collector \
  --namespace <NAMESPACE> \
  --values cmd/host-profiler/deploy/standalone/helm/pod-spec.yaml \
  --values cmd/host-profiler/deploy/standalone/helm/otel-config.yaml
```

On non-Cilium clusters, apply [`network-policy.yaml`](network-policy.yaml) to allow the Host Profiler egress it needs. If you use a namespace other than `dd-host-profiler`, update the policy namespace before applying it.

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

On clusters with Cilium, use [`cilium-network-policy.yaml`](cilium-network-policy.yaml) instead of the standard network policy to get FQDN-scoped egress enforcement.

## Verification

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.
