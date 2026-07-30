# Deploying Host Profiler with the Datadog Helm chart

## Overview

Use this guide when the Datadog Agent is already installed with Helm. This includes Datadog Agent deployments that also run the Datadog Distribution of OpenTelemetry (DDOT). If your Agent is managed by the Datadog Operator, use the [Datadog Operator guide](operator.md) instead.

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

## Selective Deployment (optional)

By default, the Datadog Agent DaemonSet (therefore the Host Profiler) is scheduled on every node in the cluster. Use one of the following options in your `values.yaml` to limit the Agent DaemonSet to a subset of nodes.

1. `agents.nodeSelector`

This option matches nodes by exact label value:

```yaml
agents:
  nodeSelector:
    eks.amazonaws.com/nodegroup: ng1
```

2. `agents.affinity.nodeAffinity`

Use this instead of `nodeSelector` when you need In/NotIn matching, multiple label conditions, or a soft preference rather than a hard requirement:

```yaml
agents:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: eks.amazonaws.com/nodegroup
                operator: In
                values: [ng1]
```

3. `agents.tolerations`

The Datadog Helm chart supports taints and tolerations. Taint the nodes first, then add a matching toleration:

```yaml
agents:
  tolerations:
    - key: dedicated
      operator: Equal
      value: host-profiler
      effect: NoSchedule
```

### Deploying a second, node-scoped Agent release

If you already run the Datadog Agent cluster-wide and want a second Helm release dedicated to the Host Profiler, scope the second release to a subset of nodes with `nodeSelector` or `affinity` above, and make sure your primary release's Agent DaemonSet does not also schedule on those same nodes (for example, by excluding them with `agents.affinity.nodeAffinity` on the primary release). Also, only one Datadog Agent release in the cluster can run the Cluster Agent and the Datadog Operator. In the second release's `values.yaml`:

- Disable the Cluster Agent and Datadog Operator so the second release does not deploy duplicates:

```yaml
datadog:
  operator:
    enabled: false
clusterAgent:
  enabled: false
```

- Point the second release at the existing Cluster Agent from your primary release instead:

```yaml
existingClusterAgent:
  join: true
  serviceName: "<PRIMARY_RELEASE_NAME>-datadog-cluster-agent"
  tokenSecretName: "<PRIMARY_RELEASE_NAME>-datadog-cluster-agent"
```

See the [Datadog Helm chart values](https://github.com/DataDog/helm-charts/blob/main/charts/datadog/values.yaml) for the full field list.
