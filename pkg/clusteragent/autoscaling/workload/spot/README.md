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
timeout it disables spot scheduling for a configured interval and triggers a rollout restart on the owning workload
(Deployment or StatefulSet) by patching `spec.template.metadata.annotations[autoscaling.datadoghq.com/spot-disabled-until]`
with a timestamp, equivalent to `kubectl rollout restart`. This causes the workload controller to replace the stuck
spot-assigned pods. Since spot scheduling is disabled at this point, newly admitted pods are scheduled on-demand (on-demand fallback).

Cluster Agent re-enables spot scheduling after the spot disabled interval elapses.

<a id="pod-updates-may-not-change"></a>1: Pod updates may not change fields other than `spec.containers[*].image`,`spec.initContainers[*].image`,`spec.activeDeadlineSeconds`,`spec.tolerations` (only additions to existing tolerations),`spec.terminationGracePeriodSeconds` (allow it to be set to 1 if it was previously negative)

### TODO

- [ ] Emit Kubernetes events
- [ ] Add metrics and observability
- [ ] Refactor pod admission subscription to not depend on PodPatcher (currently via workload.PodPatcherDelegate)
- [ ] Downscaling behaviour (similar to https://github.com/kubernetes/kubernetes/issues/124149): consider adding annotation
      `controller.kubernetes.io/pod-deletion-cost` to Deployment pods to keep on-demand/spot ratio during downscaling (see https://kubernetes.io/docs/reference/labels-annotations-taints/#pod-deletion-cost)

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
        # Set automatically on spot-assigned pods by Cluster Agent (not user-configurable):
        # autoscaling.datadoghq.com/spot-assigned: "true"
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


