# Deploying Host Profiler with the Datadog Operator

## Overview

Use this guide when the Datadog Agent is already installed with the Datadog Operator. This includes Datadog Agent deployments that also run the Datadog Distribution of OpenTelemetry (DDOT). If your Agent is installed with Helm, use the [Datadog Helm chart guide](helm.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Your Datadog Agent must be managed by the Datadog Operator version **1.32.0** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

Update your existing `DatadogAgent` Custom Resource with the following Host Profiler feature and image override. Merge this snippet into your existing resource rather than replacing unrelated fields:

```yaml
metadata:
  annotations:
    experimental.agent.datadoghq.com/image-override-config: |
      {"host-profiler": {"name": "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"}}
spec:
  features:
    hostProfiler:
      enabled: true
  override:
    nodeAgent:
      containers:
        host-profiler:
          # Explicit zero requests avoid reserving CPU or memory on every node,
          # while limits cap runaway usage.
          resources:
            requests:
              cpu: "0"
              memory: "0"
            limits:
              cpu: "500m"
              memory: "1Gi"
```


The preview image is available in Datadog's production container registries. If your cluster pulls images from another Datadog registry, replace the `registry.datadoghq.com` prefix in the image override with your preferred registry prefix. See [Changing your container registry](https://docs.datadoghq.com/containers/guide/changing_container_registry/).

For expected overhead, default limits, and tuning guidance, see [Overhead and resource usage](../faq.md#what-overhead-should-i-expect).

Apply the updated `DatadogAgent` Custom Resource through your usual workflow.

The Host Profiler infers most configuration from the Datadog Agent configuration. For optional overrides, see [Configuration](configuration.md).

After you apply the updated Custom Resource, the Operator rolls out a new Agent DaemonSet revision with the Host Profiler sidecar. Wait for that rollout to complete before verifying profiles.

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.

## Running as non-root (optional)

By default, the Host Profiler container runs as root. To run it as the `dd-agent` user (UID/GID `100`) instead, set `runAsRoot` to `false` in the Host Profiler feature of your `DatadogAgent` Custom Resource:

```yaml
spec:
  features:
    hostProfiler:
      runAsRoot: false
```

The Operator sets `runAsUser`, `runAsGroup`, and `runAsNonRoot` on the host-profiler container and keeps the Linux capabilities the profiler needs.

> [!WARNING]
> Enabling the Agent IPC Unix socket (`agent_ipc.use_socket: true`) breaks the non-root Host Profiler on Agents earlier than **7.84.0**: the profiler fails to start with a configuration sync error.

## Seccomp

The Operator installs a localhost seccomp profile for the Host Profiler, along with an init container that copies the profile to each node, and enables it by default. The profile restricts the syscalls available to the Host Profiler container beyond what its capabilities allow.

To run the Host Profiler without the seccomp profile, or to use the variant that also permits logging syscalls:

```yaml
spec:
  features:
    hostProfiler:
      seccomp:
        # false runs the host-profiler container Unconfined (no profile, no init container).
        enabled: false
        # true uses the seccomp profile that also permits logging syscalls.
        loggingEnabled: true
```

`loggingEnabled` has no effect when the seccomp profile is disabled.

## SELinux

The Datadog Operator configures the Host Profiler with the `spc_t` SELinux type by default. If `spc_t` is not available in your environment, set `spec.features.hostProfiler.seLinuxType` to an equivalent type supported by your distribution and security policy:

```yaml
spec:
  features:
    hostProfiler:
      seLinuxType: <SELINUX_TYPE>
```

The Operator applies this type to the Host Profiler and its seccomp setup init container. The replacement must provide the host and process access required by the Host Profiler.

## AppArmor (optional)

AppArmor provides extra hardening on Linux distributions and Kubernetes clusters where AppArmor is available. The Host Profiler does not require AppArmor to run.

Use this section only if your nodes support AppArmor and you already manage node-local AppArmor profiles. AppArmor profiles must be loaded on each node before Kubernetes can apply them to a pod.

To enable the provided profile, load [`apparmor-profile`](../apparmor-profile) on each node, then add `appArmorProfileName` to the host-profiler container override in your `DatadogAgent` Custom Resource:

```yaml
spec:
  override:
    nodeAgent:
      containers:
        host-profiler:
          appArmorProfileName: localhost/host-profiler
```

The provided profile limits what the Host Profiler container can execute. It allows `objcopy`, which is used for debug symbol extraction.

## Partial deployment (optional)

Follow [this guide](https://docs.datadoghq.com/containers/datadog_operator/datadog_agent_profiles/) to enable `DatadogAgentProfiles`, which allow to override certain Operator settings on a subset of nodes. Create a `DatadogAgentProfile` in the same namespace as the `DatadogAgent` and select nodes with `spec.profileAffinity.profileNodeAffinity`.

| Where to run the Host Profiler | `DatadogAgent` | `DatadogAgentProfile` |
| --- | --- | --- |
| Matching nodes only | Leave `spec.features.hostProfiler` unset | Set `spec.config.features.hostProfiler.enabled: true` |
| All nodes except matching nodes | Set `spec.features.hostProfiler.enabled: true` | Set `spec.config.features.hostProfiler.enabled: false` |

After rollout, verify that Agent pods still run on both matching and non-matching nodes, and that only the pods you intend to profile contain the `host-profiler` container.

Keep the Host Profiler image override annotation and shared container overrides on the `DatadogAgent`. For different resource requests or limits on matching nodes, set `spec.config.override.nodeAgent.containers.host-profiler.resources` in the profile.

## Migrating from annotations

Earlier Operator versions configured the Host Profiler with `agent.datadoghq.com/host-profiler-*` annotations. The Datadog Operator version **1.32.0** replaces them with the `spec.features.hostProfiler` fields used in this guide.

If both are present, the Custom Resource takes precedence for each field it defines; annotations only fill in fields left unset in the spec. The Operator reports leftover annotations as deprecated in the `DatadogAgent` status, visible with `kubectl describe datadogagent`, and in its logs.

Move each annotation to its replacement and remove the annotation. For a `DatadogAgentProfile`, use `spec.config.features.hostProfiler` in place of `spec.features.hostProfiler` in the table:

| Annotation | Replacement |
|:------------|:-------------|
| `agent.datadoghq.com/host-profiler-enabled` | `spec.features.hostProfiler.enabled` |
| `agent.datadoghq.com/host-profiler-seccomp-enabled` | `spec.features.hostProfiler.seccomp.enabled` |
| `agent.datadoghq.com/host-profiler-logging-seccomp-enabled` | `spec.features.hostProfiler.seccomp.loggingEnabled` |
| `agent.datadoghq.com/host-profiler-non-root-enabled` | `spec.features.hostProfiler.runAsRoot` (inverted: the annotation's `"true"` maps to `runAsRoot: false`) |
| `agent.datadoghq.com/host-profiler-selinux-type` | `spec.features.hostProfiler.seLinuxType` |

The `experimental.agent.datadoghq.com/image-override-config` annotation has no replacement yet; keep using it to set the preview Host Profiler image.
