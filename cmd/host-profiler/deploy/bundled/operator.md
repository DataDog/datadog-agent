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

## Selective Deployment (optional)

By default, enabling the `agent.datadoghq.com/host-profiler-enabled` annotation on the `DatadogAgent` Custom Resource turns on the Host Profiler sidecar on every node. To limit it to a subset of nodes, use a [`DatadogAgentProfile`](https://github.com/DataDog/datadog-operator/blob/main/docs/datadog_agent_profiles.md) (DAP) instead of setting the annotation on the `DatadogAgent` Custom Resource directly. This requires Datadog Operator **v1.30.0** or later.

DAP is disabled by default. Enable it in the [datadog-operator Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator) values, or as `--set` command-line flags, before creating a profile:

- `datadogAgentProfile.enabled=true`: instructs the Operator deployment to start the `DatadogAgentProfile` controller.
- `datadogCRDs.crds.datadogAgentProfiles=true`: installs the `DatadogAgentProfile` CRD.

```yaml
datadogAgentProfile:
  enabled: true
datadogCRDs:
  crds:
    datadogAgentProfiles: true
```

For OLM deployments, where container args cannot be set, enable the controller through an environment variable in the `Subscription` instead:

```yaml
config:
  env:
    - name: DD_AGENT_PROFILE_CONTROLLER_ENABLED
      value: "true"
```

A `DatadogAgentProfile` can carry the following Host Profiler annotations, which override the same-named annotations on the `DatadogAgent` Custom Resource for nodes matched by `profileAffinity`:

- `agent.datadoghq.com/host-profiler-enabled`: enables the Host Profiler.
- `agent.datadoghq.com/host-profiler-seccomp-enabled`: controls whether the Host Profiler applies its localhost seccomp profile, and the init container that installs it on the node. Defaults to enabled; set to `"false"` to disable both.
- `agent.datadoghq.com/host-profiler-logging-seccomp-enabled`: enables verbose logging for the seccomp profile. Has no effect if seccomp is disabled.
- `experimental.agent.datadoghq.com/image-override-config`: overrides the Host Profiler container image.

Remove these annotations from the `DatadogAgent` Custom Resource, then create a `DatadogAgentProfile` that carries them and scopes them with `profileAffinity`:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentProfile
metadata:
  name: host-profiler
  namespace: <NAMESPACE>
  annotations:
    agent.datadoghq.com/host-profiler-enabled: "true"
    experimental.agent.datadoghq.com/image-override-config: |
      {"host-profiler": {"name": "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"}}
spec:
  profileAffinity:
    profileNodeAffinity:
      - key: eks.amazonaws.com/nodegroup
        operator: In
        values:
          - ng1
  config: {}
```

Apply the `DatadogAgentProfile` through your usual workflow. The Datadog Operator reconciles it and rolls out the Host Profiler sidecar only on nodes matching `profileNodeAffinity`.
