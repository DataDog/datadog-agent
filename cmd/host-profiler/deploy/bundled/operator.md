# Deploying Host Profiler with the Datadog Operator

## Overview

Use this guide when the Datadog Agent is already installed with the Datadog Operator. If your Agent is installed with Helm, use the [Datadog Helm chart guide](helm.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Your Datadog Agent must be managed by the Datadog Operator version **1.25.0** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

Update your existing `DatadogAgent` Custom Resource by adding the following annotations and host-profiler container override:

```yaml
metadata:
  annotations:
    # Enable the Host Profiler sidecar and set the preview Host Profiler image.
    agent.datadoghq.com/host-profiler-enabled: "true"
    experimental.agent.datadoghq.com/image-override-config: |
      {"host-profiler": {"name": "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"}}
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
```

Apply the updated `DatadogAgent` Custom Resource through your usual workflow.

The Host Profiler infers most configuration from the Datadog Agent configuration. For optional overrides, see [Configuration](configuration.md).

After you apply the updated Custom Resource, the Operator rolls out a new Agent DaemonSet revision with the Host Profiler sidecar. Wait for that rollout to complete before verifying profiles.

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.

## Seccomp (recommended)

Current Operator versions do not install the Host Profiler seccomp profile automatically.

To use seccomp, provision the profile to each node through your cluster's node management tooling. The profile is available at `/etc/dd-host-profiler/seccomp.json` inside the Host Profiler image and must be copied to `/var/lib/kubelet/seccomp/host-profiler.json` on every node.

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
