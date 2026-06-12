# Deploying Host Profiler with the Datadog Helm chart

## Overview

Use this guide when the Datadog Agent is already installed with Helm. If your Agent is managed by the Datadog Operator, use the [Datadog Operator guide](operator.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Deploy the Datadog Agent with the Datadog Helm chart version **3.220.0** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

1. Add the Host Profiler configuration to the `values.yaml` file for your Datadog Agent Helm release:

```yaml
datadog:
  hostProfiler:
    enabled: true
    image: "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"
```

2. Upgrade your existing Datadog Agent Helm release with the updated values. Adapt this command to your Helm or GitOps workflow:

```shell
helm upgrade <RELEASE_NAME> datadog/datadog \
  --namespace <NAMESPACE> \
  --values values.yaml
```

The Datadog Helm chart configures the required capabilities and seccomp profile automatically.

The Host Profiler infers most configuration from the Datadog Agent configuration. For optional overrides, see [Configuration](configuration.md).

After you apply the updated values, Helm rolls out a new Agent DaemonSet revision with the Host Profiler sidecar. Wait for that rollout to complete before verifying profiles.

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.

## AppArmor (optional)

To use a custom AppArmor profile instead of `unconfined`, load [`apparmor-profile`](../apparmor-profile) on each node, then set:

```yaml
datadog:
  hostProfiler:
    apparmor: localhost/dd-host-profiler
```
