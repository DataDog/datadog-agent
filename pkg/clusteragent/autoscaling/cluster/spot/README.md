# Spot instance scheduling

Cluster Agent can schedule eligible workloads on spot instances.
It works in conjunction with a cluster autoscaler that provisions spot nodes.

Spot scheduling configuration has the following options:
- Spot instance percentage - defines the percentage of total workload replicas that are allowed to be scheduled on spot
  instances.
- Minimal on-demand replicas - defines the minimum number of replicas that must be scheduled on on-demand instances.
- Spot schedule timeout - defines the timeout after which the agent falls back to on-demand scheduling in case a pod
  could not be placed on a spot instance.
- Fallback duration - defines the duration during which spot scheduling is disabled due to previous failure to schedule
  a pod on a spot instance.
- Rebalance stabilization period - defines the period between rebalancing decisions for a workload to avoid pod churn.

Cluster Agent defines default values for spot scheduling configuration options and allows overriding them per workload.

## Scheduling algorithm

At pod admission time, Cluster Agent checks whether the pod belongs to a spot-eligible workload,
counts existing pods for the same workload, and selects spot placement based on the configured:
- spot instance percentage
- minimum on-demand replicas

To schedule a pod on a spot instance, Cluster Agent adds a nodeSelector matching the spot node label and a toleration for the spot node taint.
It also adds the `autoscaling.datadoghq.com/spot-assigned=true` label so the pod can be identified as spot-assigned.

Cluster Agent does not modify on-demand pods for resilience: on-demand pods schedule correctly
when the admission webhook is unavailable and other components can not depend on modifications (e.g. presence of a label).

Important:
- using nodeSelector causes cluster autoscaler to provision a new spot node when no suitable spot node is available yet.
- Kubernetes does not allow removal of nodeSelector [[1]](#pod-updates-may-not-change) after pod creation,
  so Cluster Agent cannot directly fix spot-assigned pods that fail to schedule — it must evict them and let the workload to recreate them.
- spot nodes carry a `NoSchedule` taint to repel unrelated workloads.

The spot node label and taint are currently Karpenter-specific [[2]](#karpenter-nodepool):
- label: `karpenter.sh/capacity-type=spot`
- taint: `karpenter.sh/capacity-type=spot:NoSchedule`

When a pod is assigned to a spot instance at admission time, Cluster Agent begins tracking it.
Cluster Agent periodically checks all tracked pods and if spot-assigned pods for a given workload are pending longer
than the configured timeout it disables spot scheduling for that workload for a configured duration and evicts
the pending spot-assigned pods. The workload controller replaces the evicted pods, and since spot scheduling is
disabled for the workload at this point, newly admitted pods are scheduled on-demand (on-demand fallback).

The disabled-until timestamp is persisted as the `autoscaling.datadoghq.com/spot-disabled-until` annotation on the workload.
Cluster Agent re-enables spot scheduling for the workload after the fallback duration elapses.

<a id="pod-updates-may-not-change"></a>1. Pod updates may not change fields other than `spec.containers[*].image`,`spec.initContainers[*].image`,`spec.activeDeadlineSeconds`,`spec.tolerations` (only additions to existing tolerations),`spec.terminationGracePeriodSeconds` (allow it to be set to 1 if it was previously negative)

<a id="karpenter-nodepool"></a>2. Example Karpenter spot NodePool configuration:

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: spot
spec:
  template:
    spec:
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot"]
      taints:
        - key: karpenter.sh/capacity-type
          value: spot
          effect: NoSchedule
...
```

### Rebalancing

The leader periodically checks whether each workload actual spot/on-demand ratio matches the configured target.
When a deviation is detected, it evicts one excess pod per workload per stabilization period, letting the
workload controller recreate it under the current scheduling policy. Rebalancing is skipped while spot scheduling
is disabled (on-demand fallback duration) or when there are in-flight admissions.

Rebalancing handles the following cases:

- Admission race: concurrent Cluster Agent replicas admit pods without shared count state — one replica may
  assign too many or too few spot pods.
- Workload scale-down: the workload controller deletes pods without regard to type, leaving the remaining
  spot/on-demand ratio wrong.
- Node scale-down: spot or on-demand node removal shifts all affected pods to the other type; rebalancing
  restores the ratio.
- On-demand fallback recovery: once the fallback duration elapses, all pods remain on-demand until
  rebalancing evicts the excess ones and the workload controller recreates them as spot.


### TODO

- [ ] Complete StatefulSet support (needs patch permission)
- [ ] Implement Argo Rollout support
- [ ] Implement CronJob support (needs patch permission)
- [ ] Emit Kubernetes events
- [ ] Add metrics and observability
- [ ] Add spot-related labels to the agent's out-of-the-box Kubernetes tag extraction so they are automatically collected as tags on all telemetry.
  See [out-of-the-box tags documentation](https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags)
  and the corresponding configuration in this repository.
  Labels to add:
  - `autoscaling.datadoghq.com/spot-assigned` (pod label) — identifies pods scheduled on spot instances
  - `autoscaling.datadoghq.com/spot-enabled` (workload label) — identifies spot-enabled workloads

## Spot scheduling configuration

### Default configuration

Spot scheduling is enabled and default configuration is specified in DatadogAgent CRD:
```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  override:
    clusterAgent:
      env:
        # Spot scheduling config. Use environment variables until config added to CRD.
        - name: DD_AUTOSCALING_CLUSTER_SPOT_ENABLED
          value: "true" # enable spot scheduling feature
        - name: DD_AUTOSCALING_CLUSTER_SPOT_PERCENTAGE
          value: "70" # split pods 70/30% between spot and on-demand nodes
        - name: DD_AUTOSCALING_CLUSTER_SPOT_MIN_ON_DEMAND_REPLICAS
          value: "2" # schedule at least two pods onto on-demand node
        - name: DD_AUTOSCALING_CLUSTER_SPOT_SCHEDULE_TIMEOUT
          value: "1m" # disable spot scheduling after assigned pods are pending for longer than timeout
        - name: DD_AUTOSCALING_CLUSTER_SPOT_FALLBACK_DURATION
          value: "2m" # re-enable spot scheduling after this duration elapses
        - name: DD_AUTOSCALING_CLUSTER_SPOT_REBALANCE_STABILIZATION_PERIOD
          value: "1m" # minimum time between rebalancing decisions
# ...
```

### Workload configuration

To enable spot scheduling for a workload add `autoscaling.datadoghq.com/spot-enabled: "true"` label.
Default configuration can be overridden via annotations:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app: nginx
    autoscaling.datadoghq.com/spot-enabled: "true" # Enable spot scheduling
  annotations:
    autoscaling.datadoghq.com/spot-percentage: "50" # Split pods 50/50% between spot and on-demand nodes
    autoscaling.datadoghq.com/spot-min-on-demand-replicas: "1" # schedule at least one pod onto on-demand node
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        ports:
        - containerPort: 80
```

Label and annotation changes take effect gradually due to rebalancing, use `kubectl rollout restart` to speed up the change.

Pods scheduled on spot instances have `autoscaling.datadoghq.com/spot-assigned=true` label,
use `kubectl get pods` with `-Lautoscaling.datadoghq.com/spot-assigned` to see which pods are scheduled on spot instances:

```console
$ kubectl get pods -lapp=nginx -Lautoscaling.datadoghq.com/spot-assigned
NAME                     READY   STATUS    RESTARTS   AGE     SPOT-ASSIGNED
nginx-6f8f465d8c-2mtzt   1/1     Running   0          5m25s   true
nginx-6f8f465d8c-4s9nz   1/1     Running   0          5m26s
nginx-6f8f465d8c-5p7ps   1/1     Running   0          5m29s
nginx-6f8f465d8c-7nlw6   1/1     Running   0          5m26s   true
nginx-6f8f465d8c-8pdqp   1/1     Running   0          5m27s
nginx-6f8f465d8c-frgvp   1/1     Running   0          5m27s   true
nginx-6f8f465d8c-kmr7h   1/1     Running   0          5m29s   true
nginx-6f8f465d8c-p548f   1/1     Running   0          5m29s
nginx-6f8f465d8c-s6cnj   1/1     Running   0          5m29s   true
nginx-6f8f465d8c-sn6dw   1/1     Running   0          5m29s
```
