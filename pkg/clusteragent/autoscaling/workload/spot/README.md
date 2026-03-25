# Spot instance scheduling

Cluster Agent can schedule eligible workloads on spot instances.
It works in conjunction with the Karpenter cluster autoscaler that provisions spot instances.

Spot scheduling configuration has the following options:
- spot instance percentage - defines the percentage of total workload replicas that are allowed to be scheduled on spot instances
- minimal on-demand replicas - defines the minimum number of replicas that must be scheduled on on-demand instances
- spot schedule timeout - defines the timeout after which the agent falls back to on-demand scheduling
  in case a pod could not be placed on a spot instance (default: 1 minute)
- spot disabled interval - defines the interval during which spot scheduling is disabled due to previous failure
  to schedule a pod on a spot instance (default: 2 minutes)

Cluster Agent defines default values for spot scheduling configuration options and allows overriding them per workload.

## Scheduling algorithm

At pod admission time, Cluster Agent checks whether the pod belongs to a spot-eligible workload,
counts existing pods for the same workload owner, and selects spot placement based on the configured:
- spot instance percentage
- minimum on-demand replicas
- spot disabled timestamp

To schedule a pod on a spot instance, Cluster Agent adds a `karpenter.sh/capacity-type=spot` nodeSelector and
a toleration because spot nodes carry a `karpenter.sh/capacity-type=spot:NoSchedule` taint.

Important:
- using nodeSelector causes Karpenter to provision a new spot node when no suitable spot node is available yet.
- Kubernetes does not allow removal of nodeSelector [[1]](#pod-updates-may-not-change) after pod creation, so Cluster Agent cannot directly fix
  spot-assigned pods that fail to schedule — it must trigger the workload to recreate them.

On-demand pods are not modified as they have no matching toleration, so the Kubernetes scheduler will not place them on spot nodes.

When a pod is assigned to a spot instance at admission time, Cluster Agent begins tracking it.
Cluster Agent periodically checks all tracked pods and if spot-assigned pods are pending longer than the configured
timeout it disables spot scheduling for a configured interval and evicts the stuck spot-assigned pods.
The workload controller replaces the evicted pods, and since spot scheduling is disabled at this point,
newly admitted pods are scheduled on-demand (on-demand fallback).

The disabled-until timestamp is persisted to a ConfigMap (`spot-scheduler-state`) so that all Cluster Agent
replicas share the same fallback state. Non-leader replicas sync the disabled state from the ConfigMap every
10 seconds, ensuring their admission webhooks also route replacement pods to on-demand.

Cluster Agent re-enables spot scheduling after the spot disabled interval elapses.

### Rebalancing

The leader periodically checks whether each owner's actual spot/on-demand ratio matches the configured target.
When a deviation is detected, it evicts one excess pod per owner per stabilization period (1 minute), letting the
workload controller recreate it under the current scheduling policy. Rebalancing is skipped while spot scheduling
is disabled (on-demand fallback period) or when there are in-flight admissions.

Rebalancing handles the following cases:

- **Admission race:** concurrent Cluster Agent replicas admit pods without shared count state — one replica may
  assign too many or too few spot pods.
- **Scale-down:** the workload controller deletes pods without regard to type, leaving the remaining
  spot/on-demand ratio wrong.
- **Node removal:** spot or on-demand node removal shifts all affected pods to the other type; rebalancing
  restores the ratio.
- **Auto-recovery after fallback:** once the disabled interval elapses, all pods remain on-demand until
  rebalancing evicts the excess ones and the workload controller recreates them as spot — no manual rollout
  restart required.

<a id="pod-updates-may-not-change"></a>1: Pod updates may not change fields other than `spec.containers[*].image`,`spec.initContainers[*].image`,`spec.activeDeadlineSeconds`,`spec.tolerations` (only additions to existing tolerations),`spec.terminationGracePeriodSeconds` (allow it to be set to 1 if it was previously negative)

### TODO

- [ ] Fallback ConfigMap RBAC
- [ ] Move spot configuration to the Deployment/StatefulSet annotations
- [ ] Add StatefulSet tests
- [ ] Implement Argo Rollout support
- [ ] Emit Kubernetes events
- [ ] Add metrics and observability

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
        - name: DD_AUTOSCALING_WORKLOAD_SPOT_ENABLED
          value: "true" # enable spot scheduling feature
        - name: DD_AUTOSCALING_WORKLOAD_SPOT_PERCENTAGE
          value: "70" # split pods 70/30% between spot and on-demand nodes
        - name: DD_AUTOSCALING_WORKLOAD_SPOT_MIN_ON_DEMAND_REPLICAS
          value: "2" # schedule at least two pods onto on-demand node
        - name: DD_AUTOSCALING_WORKLOAD_SPOT_SCHEDULE_TIMEOUT
          value: "1m5s" # disable spot scheduling after assigned pods are pending for longer than timeout
        - name: DD_AUTOSCALING_WORKLOAD_SPOT_DISABLED_INTERVAL
          value: "2m10s" # re-enable spot scheduling after this interval elapses
# ...
```

### Workload annotations

Default configuration can be overriden per workload via `podTemplate` annotations:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      annotations:
        autoscaling.datadoghq.com/spot-enabled: "true" # enable spot scheduling for this Deployment
        autoscaling.datadoghq.com/spot-percentage: "50" # split pods 50/50% between spot and on-demand nodes
        autoscaling.datadoghq.com/spot-min-on-demand-replicas: "1" # schedule at least one pod onto on-demand node
      labels:
        app: nginx
        # Set automatically by Cluster Agent on spot-assigned pods (not user-configurable):
        # autoscaling.datadoghq.com/spot-assigned: "true" # spot-assigned pods
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        ports:
        - containerPort: 80
```

Pods scheduled on spot instances have `autoscaling.datadoghq.com/spot-assigned=true` label.

Use `kubectl get pods` with `-Lautoscaling.datadoghq.com/spot-assigned` to see which pods are scheduled on spot instances:

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


