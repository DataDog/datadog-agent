# Kubernetes State Core Check

## Metrics

| Name      | Description   | Tags      |
|--------   |---------      |---------  |
| `kubernetes_state.daemonset.count` | Number of DaemonSets | `kube_namespace` |
| `kubernetes_state.daemonset.scheduled` | The number of nodes running at least one daemon pod and are supposed to.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.desired` | The number of nodes that should be running the daemon pod.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.misscheduled` | The number of nodes running a daemon pod but are not supposed to.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.ready` | The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and ready.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.updated` | The total number of nodes that are running updated daemon pod.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.daemons_unavailable` | The number of nodes that should be running the daemon pod and have none of the daemon pod running and available.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.daemonset.daemons_available` | The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and available.   | `kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.count` | Number of deployments | `kube_namespace` |
| `kubernetes_state.deployment.paused` | Whether the deployment is paused and will not be processed by the deployment controller.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.replicas_desired` | Number of desired pods for a deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.rollingupdate.max_unavailable` | Maximum number of unavailable replicas during a rolling update of a deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.rollingupdate.max_surge` | Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.replicas` | The number of replicas per deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.replicas_available` | The number of available replicas per deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.replicas_unavailable` | The number of unavailable replicas per deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.replicas_updated` | The number of updated replicas per deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.deployment.condition` | The current status conditions of a deployment.   | `kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.endpoint.address_available` | Number of addresses available in endpoint.   | `endpoint` `kube_namespace`   |
| `kubernetes_state.endpoint.address_not_ready` | Number of addresses not ready in endpoint.   | `endpoint` `kube_namespace`   |
| `kubernetes_state.namespace.count` | Number of namespaces | `phase` |
| `kubernetes_state.node.count` | Information about a cluster node.   | `node` `kernel_version` `os_image` `container_runtime_version` `kubelet_version` `kubeproxy_version` `provider_id` `pod_cidr`   |
| `kubernetes_state.node.allocatable` | The allocatable for different resources of a node that are available for scheduling.   | `node` `resource` `unit`   |
| `kubernetes_state.node.capacity` | The capacity for different resources of a node.   | `node` `resource` `unit`   |
| `kubernetes_state.node.by_condition` | The condition of a cluster node.   | `condition` `node` `status`   |
| `kubernetes_state.node.status` | Whether the node can schedule new pods.   | `node` `status`   |
| `kubernetes_state.container.terminated` | Describes whether the container is currently in terminated state.   | `kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.resource_limits` | The number of requested limit resource by a container.   | `kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.resource_requests` | The number of requested request resource by a container.   | `kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.ready` | Describes whether the containers readiness check succeeded.   | `kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.restarts` | The number of container restarts per container.   | `kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.running` | Describes whether the container is currently in running state.   | `kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.waiting` | Describes whether the container is currently in waiting state.   | `kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.container.status_report.count.waiting` | Describes the reason the container is currently in waiting state.   | `kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.container.status_report.count.terminated` | Describes the reason the container is currently in terminated state.   | `kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.container.status_report.count.waiting` | Describes the reason the container is currently in waiting state.   | `kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.container.status_report.count.terminated` | Describes the reason the container is currently in terminated state.   | `kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.pod.ready` | Describes whether the pod is ready to serve requests.   | `kube_namespace` `pod_name` `condition` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.pod.scheduled` | Describes the status of the scheduling process for the pod.   | `kube_namespace` `pod_name` `condition` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.pod.volumes.persistentvolumeclaims_readonly` | Describes whether a persistentvolumeclaim is mounted read only.   | `kube_namespace` `pod_name` `volume` `persistentvolumeclaim` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.pod.unschedulable` | Describes the unschedulable status for the pod.   | `kube_namespace` `pod_name` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.pod.status_phase` | The pods current phase.   | `kube_namespace` `pod_name` `phase` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.persistentvolumeclaim.status` | The phase the persistent volume claim is currently in.   | `kube_namespace` `persistentvolumeclaim` `phase` `storageclass`   |
| `kubernetes_state.persistentvolumeclaim.access_mode` | The access mode(s) specified by the persistent volume claim.   | `kube_namespace` `persistentvolumeclaim` `access_mode` `storageclass`   |
| `kubernetes_state.persistentvolumeclaim.request_storage` | The capacity of storage requested by the persistent volume claim.   | `kube_namespace` `persistentvolumeclaim` `storageclass`   |
| `kubernetes_state.persistentvolume.capacity` | Persistentvolume capacity in bytes.   | `persistentvolume` `storageclass`   |
| `kubernetes_state.persistentvolume.by_phase` | The phase indicates if a volume is available, bound to a claim, or released by a claim.   | `persistentvolume` `storageclass` `phase`   |
| `kubernetes_state.pdb.pods_healthy` | Current number of healthy pods.   | `kube_namespace` `poddisruptionbudget`   |
| `kubernetes_state.pdb.pods_desired` | Minimum desired number of healthy pods.   | `kube_namespace` `poddisruptionbudget`   |
| `kubernetes_state.pdb.disruptions_allowed` | Number of pod disruptions that are currently allowed.   | `kube_namespace` `poddisruptionbudget`   |
| `kubernetes_state.pdb.pods_total` | Total number of pods counted by this disruption budget.   | `kube_namespace` `poddisruptionbudget`   |
| `kubernetes_state.secret.type` | Type about secret.   | `kube_namespace` `secret` `type`   |
| `kubernetes_state.replicaset.count` | Number of ReplicaSets | `kube_namespace` `owner_name` `owner_kind` |
| `kubernetes_state.replicaset.replicas_desired` | Number of desired pods for a ReplicaSet.   | `kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.replicaset.fully_labeled_replicas` | The number of fully labeled replicas per ReplicaSet.   | `kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.replicaset.replicas_ready` | The number of ready replicas per ReplicaSet.   | `kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.replicaset.replicas` | The number of replicas per ReplicaSet.   | `kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.replicationcontroller.replicas_desired` | Number of desired pods for a ReplicationController.   | `kube_namespace` `kube_replication_controller`   |
| `kubernetes_state.replicationcontroller.replicas_available` | The number of available replicas per ReplicationController.   | `kube_namespace` `kube_replication_controller`   |
| `kubernetes_state.replicationcontroller.fully_labeled_replicas` | The number of fully labeled replicas per ReplicationController.   | `kube_namespace` `kube_replication_controller`   |
| `kubernetes_state.replicationcontroller.replicas_ready` | The number of ready replicas per ReplicationController.   | `kube_namespace` `kube_replication_controller`   |
| `kubernetes_state.replicationcontroller.replicas` | The number of replicas per ReplicationController.   | `kube_namespace` `kube_replication_controller`   |
| `kubernetes_state.statefulset.count` | Number of StatefulSets | `kube_namespace` |
| `kubernetes_state.statefulset.replicas_desired` | Number of desired pods for a StatefulSet.   | `kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.statefulset.replicas` | The number of replicas per StatefulSet.   | `kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.statefulset.replicas_current` | The number of current replicas per StatefulSet.   | `kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.statefulset.replicas_ready` | The number of ready replicas per StatefulSet.   | `kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.statefulset.replicas_updated` | The number of updated replicas per StatefulSet.   | `kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.hpa.min_replicas` | Lower limit for the number of pods that can be set by the autoscaler, default 1.   | `kube_namespace` `horizontalpodautoscaler`   |
| `kubernetes_state.hpa.max_replicas` | Upper limit for the number of pods that can be set by the autoscaler; cannot be smaller than MinReplicas.   | `kube_namespace` `horizontalpodautoscaler`   |
| `kubernetes_state.hpa.condition` | The condition of this autoscaler.   | `kube_namespace` `horizontalpodautoscaler` `condition` `status`   |
| `kubernetes_state.hpa.desired_replicas` | Desired number of replicas of pods managed by this autoscaler.   | `kube_namespace` `horizontalpodautoscaler`   |
| `kubernetes_state.hpa.current_replicas` | Current number of replicas of pods managed by this autoscaler.   | `kube_namespace` `horizontalpodautoscaler`   |
| `kubernetes_state.hpa.spec_target_metric` | The metric specifications used by this autoscaler when calculating the desired replica count.   | `kube_namespace` `horizontalpodautoscaler` `metric_name` `metric_target_type`    |
| `kubernetes_state.vpa.lower_bound` | Minimum resources the container can use before the VerticalPodAutoscaler updater evicts it.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.vpa.target` | Target resources the VerticalPodAutoscaler recommends for the container.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.vpa.uncapped_target` | Target resources the VerticalPodAutoscaler recommends for the container ignoring bounds.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.vpa.upperbound` | Maximum resources the container can use before the VerticalPodAutoscaler updater evicts it.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.vpa.update_mode` | Update mode of the VerticalPodAutoscaler.   | `kube_namespace` `verticalpodautoscaler` `target_api_version` `target_kind` `target_name` `update_mode`    |
| `kubernetes_state.vpa.spec_container_minallowed` | Minimum resources the VerticalPodAutoscaler can set for containers matching the name.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.vpa.spec_container_maxallowed` | Maximum resources the VerticalPodAutoscaler can set for containers matching the name.   | `kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`    |
| `kubernetes_state.cronjob.spec_suspend` | Suspend flag tells the controller to suspend subsequent executions.   | `kube_namespace` `kube_cronjob` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.cronjob.duration_since_last_schedule` | The duration since the last time the cronjob was scheduled.   | `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.job.count` | Number of jobs | `kube_namespace` `owner_name` `owner_kind` |
| `kubernetes_state.job.failed` | The number of pods which reached Phase Failed.   | `kube_job` `kube_namespace` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.job.succeeded` | The number of pods which reached Phase Succeeded.   | `kube_job` `kube_namespace` (`env` `service` `version` from standard labels)    |
| `kubernetes_state.resourcequota.<resource>.limit` | Information about resource quota limits by resource.   | `kube_namespace` `resourcequota`   |
| `kubernetes_state.resourcequota.<resource>.used` | Information about resource quota usage by resource.   | `kube_namespace` `resourcequota`   |
| `kubernetes_state.limitrange.cpu.<constraint>` | Information about limit range usage by constraint.   | `kube_namespace` `limitrange` `type`   |
| `kubernetes_state.limitrange.memory.<constraint>` | Information about memory limit range usage by constraint.   | `kube_namespace` `limitrange` `type`   |
| `kubernetes_state.service.count` | Number of services | `kube_namespace` `type` |
| `kubernetes_state.service.type` | Service types.   | `kube_namespace` `kube_service` `type`   |

## Service Checks

| Name      | Description   | Tags      |
|--------   |---------      |---------  |
| `kubernetes_state.cronjob.on_schedule_check` | Alert if the cronjob's next schedule is in the past.   | `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.job.complete` | Whether the job is failed or not.   | `kube_job` `kube_namespace` (`env` `service` `version` from standard labels)   |
| `kubernetes_state.node.ready` | Whether the node is ready.   | `node` `condition` `status`   |
| `kubernetes_state.node.out_of_disk` | Whether the node is out of disk.   | `node` `condition` `status`   |
| `kubernetes_state.node.disk_pressure` | Whether the node is under disk pressure   | `node` `condition` `status`   |
| `kubernetes_state.node.network_unavailable` | Whether the node network is unavailable   | `node` `condition` `status`   |
| `kubernetes_state.node.memory_pressure` | Whether the node network is under memory pressure   | `node` `condition` `status`   |
