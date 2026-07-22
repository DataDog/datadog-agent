# Deploying Host Profiler with the Datadog Operator

## Overview

Use this guide when the Datadog Agent is already installed with the Datadog Operator. This includes Datadog Agent deployments that also run the Datadog Distribution of OpenTelemetry (DDOT). If your Agent is installed with Helm, use the [Datadog Helm chart guide](helm.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Your Datadog Agent must be managed by the Datadog Operator version **1.25.0** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

Update your existing `DatadogAgent` Custom Resource with the following annotations and host-profiler container override. Merge this snippet into your existing resource rather than replacing unrelated fields:

```yaml
metadata:
  annotations:
    # Enable the Host Profiler sidecar and set the preview Host Profiler image.
    agent.datadoghq.com/host-profiler-enabled: "true"
    experimental.agent.datadoghq.com/image-override-config: |
      {"host-profiler": {"name": "registry.datadoghq.com/ddot-ebpf:dev-hp-preview-1.0-09ad4bc1-zstd-nydus"}}
spec:
  override:
    nodeAgent:
      containers:
        host-profiler:
          # Required for current Datadog Operator versions.
          # Future Operator versions are expected to configure the Host Profiler
          # security context automatically.
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              # Drop default capabilities and add only the ones the Host Profiler needs.
              drop: ["ALL"]
              add: ["BPF", "PERFMON", "SYS_PTRACE", "SYS_RESOURCE", "DAC_READ_SEARCH", "SYSLOG", "CHECKPOINT_RESTORE", "IPC_LOCK"]
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

## Seccomp (optional)

Seccomp provides extra hardening by restricting the syscalls available to the Host Profiler container. The Host Profiler does not require seccomp to run in this preview.

Current Operator versions do not install or configure the Host Profiler seccomp profile automatically. A future Operator version is expected to configure seccomp by default.

Use this section only if you already manage node-local seccomp profiles or want to add the extra hardening manually. The profile is available at `/etc/dd-host-profiler/seccomp.json` inside the Host Profiler image and must be copied to `/var/lib/kubelet/seccomp/host-profiler.json` on every node.

Then add `seccompProfile` to the same host-profiler container override in your `DatadogAgent` Custom Resource:

```yaml
spec:
  override:
    nodeAgent:
      containers:
        host-profiler:
          securityContext:
            seccompProfile:
              type: Localhost
              localhostProfile: host-profiler.json
```

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
