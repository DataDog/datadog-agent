# Deploying Host Profiler with the Datadog Helm chart

## Overview

Use this guide when the Datadog Agent is already installed with Helm. This includes Datadog Agent deployments that also run the Datadog Distribution of OpenTelemetry (DDOT). If your Agent is managed by the Datadog Operator, use the [Datadog Operator guide](operator.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Deploy the Datadog Agent with the Datadog Helm chart version **3.273.0** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

1. Add the Host Profiler configuration to the `values.yaml` file for your Datadog Agent Helm release:

```yaml
datadog:
  hostProfiler:
    enabled: true
    image: "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"
agents:
  containers:
    hostProfiler:
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

The preview image is available in Datadog's production container registries. If your cluster pulls images from another Datadog registry, replace the `registry.datadoghq.com` prefix with your preferred registry prefix. See [Changing your container registry](https://docs.datadoghq.com/containers/guide/changing_container_registry/).

For expected overhead, default limits, and tuning guidance, see [Overhead and resource usage](../faq.md#what-overhead-should-i-expect).

2. Upgrade your existing Datadog Agent Helm release with the updated values. Adapt this command to your Helm or GitOps workflow, and include any existing values files you already use for the release:

```shell
helm upgrade <RELEASE_NAME> datadog/datadog \
  --namespace <NAMESPACE> \
  --values values.yaml
```

The Datadog Helm chart configures the required capabilities and seccomp profile automatically.

The Host Profiler infers most configuration from the Datadog Agent configuration. For optional overrides, see [Configuration](configuration.md).

After you apply the updated values, Helm rolls out a new Agent DaemonSet revision with the Host Profiler sidecar. Wait for that rollout to complete before verifying profiles.

After deploying the Host Profiler, profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes. If profiles do not appear, see the [Troubleshooting](../troubleshooting.md) guide.

## Running as non-root (optional)

By default, the Host Profiler container runs as root. To run it as the `dd-agent` user (UID/GID `100`) instead, set `datadog.hostProfiler.runAsRoot` to `false`:

```yaml
datadog:
  hostProfiler:
    enabled: true
    runAsRoot: false
```

The chart sets `runAsUser`, `runAsGroup`, and `runAsNonRoot` on the host-profiler container and keeps the Linux capabilities the profiler needs.

> [!WARNING]
> Enabling the Agent IPC Unix socket (`agent_ipc.use_socket: true`) breaks the non-root Host Profiler on Agents earlier than **7.84.0**: the profiler fails to start with a configuration sync error.

## SELinux

The Datadog Helm chart configures the Host Profiler with the `spc_t` SELinux type by default. If `spc_t` is not available in your environment, set `agents.containers.hostProfiler.securityContext.seLinuxOptions.type` to an equivalent type supported by your distribution and security policy:

```yaml
agents:
  containers:
    hostProfiler:
      securityContext:
        seLinuxOptions:
          type: <SELINUX_TYPE>
```

The chart applies this type to the Host Profiler and its seccomp setup init container. The replacement must provide the host and process access required by the Host Profiler.

## AppArmor (optional)

AppArmor provides extra hardening on Linux distributions and Kubernetes clusters where AppArmor is available. The Host Profiler does not require AppArmor to run.

Use this section only if your nodes support AppArmor and you already manage node-local AppArmor profiles. AppArmor profiles must be loaded on each node before Kubernetes can apply them to a pod.

To enable the provided profile, load [`apparmor-profile`](../apparmor-profile) on each node, then set:

```yaml
datadog:
  hostProfiler:
    apparmor: localhost/host-profiler
```

The provided profile limits what the Host Profiler container can execute. It allows `objcopy`, which is used for debug symbol extraction.

## Partial deployment (optional)

The bundled Host Profiler runs wherever the Agent DaemonSet runs.

### Run the Agent and Host Profiler on a subset of nodes

Use one Helm release. In that release’s Helm values, add the [Host Profiler configuration](#deploy) and set `agents.nodeSelector` or `agents.affinity.nodeAffinity`. Nodes that don’t match the selector will stop running the Datadog Agent from this release.

Follow [Assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/) for syntax. Use a node selector or **required** node affinity; preferred affinity does not restrict placement.

If taints block scheduling on the target nodes, add matching `agents.tolerations`. [Tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) allow scheduling on tainted nodes but do not select nodes.

### Profile a subset of nodes while keeping existing Agent coverage

Use two Agent releases with non-overlapping node scopes. Preserve any existing scheduling constraints. For example, use required node affinity with `NotIn` on the primary release and `In` on the second release, using the same label key and value.

| Release | Nodes | `datadog.hostProfiler.enabled` |
| --- | --- | --- |
| Primary | All previously monitored nodes except the nodes you want to profile | `false` |
| Second | Only the nodes you want to profile | `true` |

Set the [Host Profiler image and resource values](#deploy) in the second release. If the primary release runs a Cluster Agent, install the second release in the same namespace and use the primary release's Cluster Agent Service and token Secret names to join it. Disable duplicate Cluster Agent and Operator deployments and prevent the second release from installing CRDs owned by the primary release:

```yaml
datadog:
  operator:
    enabled: false
clusterAgent:
  enabled: false
existingClusterAgent:
  join: true
  serviceName: "<PRIMARY_CLUSTER_AGENT_SERVICE>"
  tokenSecretName: "<PRIMARY_CLUSTER_AGENT_TOKEN_SECRET>"
datadog-crds:
  crds:
    datadogMetrics: false
    datadogPodAutoscalers: false
    datadogPodAutoscalerClusterProfiles: false
    datadogInstrumentations: false
datadog-instrumentation-crd:
  crds:
    datadogInstrumentations: false
```

Keep both `datadogInstrumentations` entries. Chart versions before `3.241.0` use `datadog-crds` for the instrumentation CRD; `3.241.0` and later use `datadog-instrumentation-crd`.

If the primary release does not run a Cluster Agent, omit `existingClusterAgent`.

### Verify placement

After rollout, verify that each node you intend to monitor runs one Agent pod from these releases and that no other nodes run one. Only Agent pods on the selected nodes should contain the `host-profiler` container.
