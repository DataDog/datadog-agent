# Deploying Host Profiler with the Datadog Helm chart

## Overview

Use this guide when the Datadog Agent is already installed with Helm. This includes Datadog Agent deployments that also run the Datadog Distribution of OpenTelemetry (DDOT). If your Agent is managed by the Datadog Operator, use the [Datadog Operator guide](operator.md) instead.

The Host Profiler runs as a sidecar in the Datadog Agent DaemonSet, and the Agent enriches profiles with Datadog infrastructure metadata.

Review the [supported environments](../README.md#supported-environments) before continuing.

## Prerequisites

Deploy the Datadog Agent with the Datadog Helm chart version **3.240.2** or later. See the [Datadog Agent installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

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

## Selective deployment (optional)

By default, the Datadog Agent DaemonSet, and therefore the Host Profiler sidecar, runs on every node in the cluster. To limit the Agent DaemonSet to a subset of nodes, set one of the following fields in your `values.yaml`.

### `agents.nodeSelector`

Matches nodes by exact label value:

```yaml
agents:
  nodeSelector:
    eks.amazonaws.com/nodegroup: ng1
```

### `agents.affinity.nodeAffinity`

Use node affinity instead of `nodeSelector` for `In`/`NotIn` matching, multiple label conditions, or a soft preference rather than a hard requirement:

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

### `agents.tolerations`

Target nodes may already carry a taint, for example a reserved nodegroup or a team's dedicated node pool. The Host Profiler cannot schedule on a tainted node without a matching toleration, even when `nodeSelector` or `affinity` matches that node:

```yaml
agents:
  tolerations:
    - key: dedicated
      operator: Equal
      value: host-profiler
      effect: NoSchedule
```

### Running a second, node-scoped Agent release

To keep the existing cluster-wide Agent release unchanged and dedicate a second Helm release to the Host Profiler:

1. Scope the second release to a subset of nodes with `nodeSelector` or `affinity`, and exclude those same nodes from the primary release's Agent DaemonSet, for example with `agents.affinity.nodeAffinity` on the primary release. Two Agent DaemonSets must not schedule on the same node.

2. Disable the Cluster Agent and the Datadog Operator on the second release. Only one Datadog Agent release per cluster can run them:

   ```yaml
   datadog:
     operator:
       enabled: false
   clusterAgent:
     enabled: false
   ```

3. Point the second release at the primary release's Cluster Agent:

   ```yaml
   existingClusterAgent:
     join: true
     serviceName: "<PRIMARY_RELEASE_NAME>-datadog-cluster-agent"
     tokenSecretName: "<PRIMARY_RELEASE_NAME>-datadog-cluster-agent"
   ```

4. If the second release sets `datadog.autoscaling.workload.enabled`, `datadog.instrumentationCrd.enabled`, or `clusterAgent.metricsProvider.useDatadogMetrics`, disable the `datadog-crds` subchart. Otherwise the second release tries to create CRDs the primary release already owns:

   ```yaml
   datadog-crds:
     crds:
       datadogMetrics: false
       datadogPodAutoscalers: false
       datadogPodAutoscalerClusterProfiles: false
       datadogInstrumentations: false
   ```

See the [Datadog Helm chart values](https://github.com/DataDog/helm-charts/blob/main/charts/datadog/values.yaml) for the full field list.
