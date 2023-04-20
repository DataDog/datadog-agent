# Kubernetes State Core Check

## Data Collected

### Metrics

`kubernetes_state.apiservice.count`
: Number of Kubernetes API services.

`kubernetes_state.apiservice.condition`
: The condition of this API service. Tags:`apiservice` `condition` `status`.

`kubernetes_state.daemonset.count`
: Number of DaemonSets. Tags:`kube_namespace`.

`kubernetes_state.daemonset.scheduled`
: The number of nodes running at least one daemon pod and are supposed to. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.desired`
: The number of nodes that should be running the daemon pod. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.misscheduled`
: The number of nodes running a daemon pod but are not supposed to. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.ready`
: The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and ready. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.updated`
: The total number of nodes that are running updated daemon pod. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.daemons_unavailable`
: The number of nodes that should be running the daemon pod and have none of the daemon pod running and available. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.daemonset.daemons_available`
: The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and available. Tags:`kube_daemon_set` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.count`
: Number of deployments. Tags:`kube_namespace`.

`kubernetes_state.deployment.paused`
: Whether the deployment is paused and will not be processed by the deployment controller. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas_desired`
: Number of desired pods for a deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.rollingupdate.max_unavailable`
: Maximum number of unavailable replicas during a rolling update of a deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.rollingupdate.max_surge`
: Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas`
: The number of replicas per deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas_available`
: The number of available replicas per deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas_ready	`
: The number of ready replicas per deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas_unavailable`
: The number of unavailable replicas per deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.replicas_updated`
: The number of updated replicas per deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.deployment.condition`
: The current status conditions of a deployment. Tags:`kube_deployment` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.endpoint.count`
: Number of endpoints. Tags:`kube_namespace`.

`kubernetes_state.endpoint.address_available`
: Number of addresses available in endpoint. Tags:`endpoint` `kube_namespace`.

`kubernetes_state.endpoint.address_not_ready`
: Number of addresses not ready in endpoint. Tags:`endpoint` `kube_namespace`.

`kubernetes_state.namespace.count`
: Number of namespaces. Tags:`phase`.

`kubernetes_state.node.count`
: Number of nodes. Tags:`kernel_version` `os_image` `container_runtime_version` `kubelet_version`.

`kubernetes_state.node.cpu_allocatable`
: The allocatable CPU of a node that is available for scheduling. Tags:`node` `resource` `unit`.

`kubernetes_state.node.memory_allocatable`
: The allocatable memory of a node that is available for scheduling. Tags:`node` `resource` `unit`.

`kubernetes_state.node.pods_allocatable`
: The allocatable memory of a node that is available for scheduling. Tags:`node` `resource` `unit`.

`kubernetes_state.node.ephemeral_storage_allocatable`
: The allocatable ephemeral-storage of a node that is available for scheduling. Tags:`node` `resource` `unit`.

`kubernetes_state.node.network_bandwidth_allocatable`
: The allocatable network bandwidth of a node that is available for scheduling. Tags:`node` `resource` `unit`.

`kubernetes_state.node.cpu_capacity`
: The CPU capacity of a node. Tags:`node` `resource` `unit`.

`kubernetes_state.node.memory_capacity`
: The memory capacity of a node. Tags:`node` `resource` `unit`.

`kubernetes_state.node.pods_capacity`
: The pods capacity of a node. Tags:`node` `resource` `unit`.

`kubernetes_state.node.ephemeral_storage_capacity`
: The ephemeral-storage capacity of a node. Tags:`node` `resource` `unit`.

`kubernetes_state.node.network_bandwidth_capacity`
: The network bandwidth capacity of a node. Tags:`node` `resource` `unit`.

`kubernetes_state.node.by_condition`
: The condition of a cluster node. Tags:`condition` `node` `status`.

`kubernetes_state.node.status`
: Whether the node can schedule new pods. Tags:`node` `status`.

`kubernetes_state.node.age`
: The time in seconds since the creation of the node. Tags:`node`.

`kubernetes_state.container.terminated`
: Describes whether the container is currently in terminated state. Tags:`kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels).

`kubernetes_state.container.cpu_limit`
: The value of CPU limit by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.memory_limit`
: The value of memory limit by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.network_bandwidth_limit`
: The value of network bandwidth limit by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.cpu_requested`
: The value of CPU requested by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.memory_requested`
: The value of memory requested by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.network_bandwidth_requested`
: The value of network bandwidth requested by a container. Tags:`kube_namespace` `pod_name` `kube_container_name` `node` `resource` `unit` (`env` `service` `version` from standard labels).

`kubernetes_state.container.ready`
: Describes whether the containers readiness check succeeded. Tags:`kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels).

`kubernetes_state.container.restarts`
: The number of container restarts per container. Tags:`kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels).

`kubernetes_state.container.running`
: Describes whether the container is currently in running state. Tags:`kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels).

`kubernetes_state.container.waiting`
: Describes whether the container is currently in waiting state. Tags:`kube_namespace` `pod_name` `kube_container_name` (`env` `service` `version` from standard labels).

`kubernetes_state.container.status_report.count.waiting`
: Describes the reason the container is currently in waiting state. Tags:`kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels).

`kubernetes_state.container.status_report.count.terminated`
: Describes the reason the container is currently in terminated state. Tags:`kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels).

`kubernetes_state.container.status_report.count.waiting`
: Describes the reason the container is currently in waiting state. Tags:`kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels).

`kubernetes_state.container.status_report.count.terminated`
: Describes the reason the container is currently in terminated state. Tags:`kube_namespace` `pod_name` `kube_container_name` `reason` (`env` `service` `version` from standard labels).

`kubernetes_state.crd.count`
: Number of custom resource definition.

`kubernetes_state.crd.condition`
: The condition of this custom resource definition. Tags:`customresourcedefinition` `condition` `status`.

`kubernetes_state.pod.ready`
: Describes whether the pod is ready to serve requests. Tags:`node` `kube_namespace` `pod_name` `condition` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.scheduled`
: Describes the status of the scheduling process for the pod. Tags:`node` `kube_namespace` `pod_name` `condition` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.volumes.persistentvolumeclaims_readonly`
: Describes whether a persistentvolumeclaim is mounted read only. Tags:`node` `kube_namespace` `pod_name` `volume` `persistentvolumeclaim` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.unschedulable`
: Describes the unschedulable status for the pod. Tags:`kube_namespace` `pod_name` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.status_phase`
: The pods current phase. Tags:`node` `kube_namespace` `pod_name` `pod_phase` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.age`
: The time in seconds since the creation of the pod. Tags:`node` `kube_namespace` `pod_name` `pod_phase` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.uptime`
: The time in seconds since the pod has been scheduled and acknowledged by the Kubelet. Tags:`node` `kube_namespace` `pod_name` `pod_phase` (`env` `service` `version` from standard labels).

`kubernetes_state.pod.count`
: Number of Pods. Tags:`node` `kube_namespace` `kube_<owner kind>`.

`kubernetes_state.persistentvolumeclaim.status`
: The phase the persistent volume claim is currently in. Tags:`kube_namespace` `persistentvolumeclaim` `phase` `storageclass`.

`kubernetes_state.persistentvolumeclaim.access_mode`
: The access mode(s) specified by the persistent volume claim. Tags:`kube_namespace` `persistentvolumeclaim` `access_mode` `storageclass`.

`kubernetes_state.persistentvolumeclaim.request_storage`
: The capacity of storage requested by the persistent volume claim. Tags:`kube_namespace` `persistentvolumeclaim` `storageclass`.

`kubernetes_state.persistentvolume.capacity`
: Persistentvolume capacity in bytes. Tags:`persistentvolume` `storageclass`.

`kubernetes_state.persistentvolume.by_phase`
: The phase indicates if a volume is available, bound to a claim, or released by a claim. Tags:`persistentvolume` `storageclass` `phase`.

`kubernetes_state.pdb.pods_healthy`
: Current number of healthy pods. Tags:`kube_namespace` `poddisruptionbudget`.

`kubernetes_state.pdb.pods_desired`
: Minimum desired number of healthy pods. Tags:`kube_namespace` `poddisruptionbudget`.

`kubernetes_state.pdb.disruptions_allowed`
: Number of pod disruptions that are currently allowed. Tags:`kube_namespace` `poddisruptionbudget`.

`kubernetes_state.pdb.pods_total`
: Total number of pods counted by this disruption budget. Tags:`kube_namespace` `poddisruptionbudget`.

`kubernetes_state.secret.type`
: Type about secret. Tags:`kube_namespace` `secret` `type`.

`kubernetes_state.replicaset.count`
: Number of ReplicaSets Tags:`kube_namespace` `kube_deployment`.

`kubernetes_state.replicaset.replicas_desired`
: Number of desired pods for a ReplicaSet. Tags:`kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels).

`kubernetes_state.replicaset.fully_labeled_replicas`
: The number of fully labeled replicas per ReplicaSet. Tags:`kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels).

`kubernetes_state.replicaset.replicas_ready`
: The number of ready replicas per ReplicaSet. Tags:`kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels).

`kubernetes_state.replicaset.replicas`
: The number of replicas per ReplicaSet. Tags:`kube_namespace` `kube_replica_set` (`env` `service` `version` from standard labels).

`kubernetes_state.replicationcontroller.replicas_desired`
: Number of desired pods for a ReplicationController. Tags:`kube_namespace` `kube_replication_controller`.

`kubernetes_state.replicationcontroller.replicas_available`
: The number of available replicas per ReplicationController. Tags:`kube_namespace` `kube_replication_controller`.

`kubernetes_state.replicationcontroller.fully_labeled_replicas`
: The number of fully labeled replicas per ReplicationController. Tags:`kube_namespace` `kube_replication_controller`.

`kubernetes_state.replicationcontroller.replicas_ready`
: The number of ready replicas per ReplicationController. Tags:`kube_namespace` `kube_replication_controller`.

`kubernetes_state.replicationcontroller.replicas`
: The number of replicas per ReplicationController. Tags:`kube_namespace` `kube_replication_controller`.

`kubernetes_state.statefulset.count`
: Number of StatefulSets Tags:`kube_namespace`.

`kubernetes_state.statefulset.replicas_desired`
: Number of desired pods for a StatefulSet. Tags:`kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels).

`kubernetes_state.statefulset.replicas`
: The number of replicas per StatefulSet. Tags:`kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels).

`kubernetes_state.statefulset.replicas_current`
: The number of current replicas per StatefulSet. Tags:`kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels).

`kubernetes_state.statefulset.replicas_ready`
: The number of ready replicas per StatefulSet. Tags:`kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels).

`kubernetes_state.statefulset.replicas_updated`
: The number of updated replicas per StatefulSet. Tags:`kube_namespace` `kube_stateful_set` (`env` `service` `version` from standard labels).

`kubernetes_state.hpa.count`
: Number of horizontal pod autoscaler. Tags: `kube_namespace`.

`kubernetes_state.hpa.min_replicas`
: Lower limit for the number of pods that can be set by the autoscaler, default 1. Tags:`kube_namespace` `horizontalpodautoscaler`.

`kubernetes_state.hpa.max_replicas`
: Upper limit for the number of pods that can be set by the autoscaler; cannot be smaller than MinReplicas. Tags:`kube_namespace` `horizontalpodautoscaler`.

`kubernetes_state.hpa.condition`
: The condition of this autoscaler. Tags:`kube_namespace` `horizontalpodautoscaler` `condition` `status`.

`kubernetes_state.hpa.desired_replicas`
: Desired number of replicas of pods managed by this autoscaler. Tags:`kube_namespace` `horizontalpodautoscaler`.

`kubernetes_state.hpa.current_replicas`
: Current number of replicas of pods managed by this autoscaler. Tags:`kube_namespace` `horizontalpodautoscaler`.

`kubernetes_state.hpa.spec_target_metric`
: The metric specifications used by this autoscaler when calculating the desired replica count. Tags:`kube_namespace` `horizontalpodautoscaler` `metric_name` `metric_target_type`.

`kubernetes_state.hpa.status_target_metric`
: The current metric status used by this autoscaler when calculating the desired replica count. Tags:`kube_namespace` `horizontalpodautoscaler` `metric_name` `metric_target_type`.

`kubernetes_state.vpa.count`
: Number of vertical pod autoscaler. Tags: `kube_namespace`.

`kubernetes_state.vpa.lower_bound`
: Minimum resources the container can use before the VerticalPodAutoscaler updater evicts it. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.vpa.target`
: Target resources the VerticalPodAutoscaler recommends for the container. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.vpa.uncapped_target`
: Target resources the VerticalPodAutoscaler recommends for the container ignoring bounds. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.vpa.upperbound`
: Maximum resources the container can use before the VerticalPodAutoscaler updater evicts it. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.vpa.update_mode`
: Update mode of the VerticalPodAutoscaler. Tags:`kube_namespace` `verticalpodautoscaler` `target_api_version` `target_kind` `target_name` `update_mode`.

`kubernetes_state.vpa.spec_container_minallowed`
: Minimum resources the VerticalPodAutoscaler can set for containers matching the name. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.vpa.spec_container_maxallowed`
: Maximum resources the VerticalPodAutoscaler can set for containers matching the name. Tags:`kube_namespace` `verticalpodautoscaler` `kube_container_name` `resource` `target_api_version` `target_kind` `target_name` `unit`.

`kubernetes_state.cronjob.count`
: Number of cronjobs. Tags:`kube_namespace`.

`kubernetes_state.cronjob.spec_suspend`
: Suspend flag tells the controller to suspend subsequent executions. Tags:`kube_namespace` `kube_cronjob` (`env` `service` `version` from standard labels).

`kubernetes_state.cronjob.duration_since_last_schedule`
: The duration since the last time the cronjob was scheduled. Tags:`kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.count`
: Number of jobs. Tags:`kube_namespace` `kube_cronjob`.

`kubernetes_state.job.failed`
: The number of pods which reached Phase Failed. Tags:`kube_job` or `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.succeeded`
: The number of pods which reached Phase Succeeded. Tags:`kube_job` or `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.completion.succeeded`
: The job has completed its execution. Tags:`kube_job` or `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.completion.failed`
: The job has failed its execution. Tags:`kube_job` or `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.duration
: Time elapsed between the start and completion time of the job, or the current time if the job is still running. Tags:`kube_job` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.resourcequota.<resource>.limit`
: Information about resource quota limits by resource. Tags:`kube_namespace` `resourcequota`.

`kubernetes_state.resourcequota.<resource>.used`
: Information about resource quota usage by resource. Tags:`kube_namespace` `resourcequota`.

`kubernetes_state.limitrange.cpu.min`
: Information about CPU limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.cpu.max`
: Information about CPU limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.cpu.default`
: Information about CPU limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.cpu.default_request`
: Information about CPU limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.cpu.max_limit_request_ratio`
: Information about CPU limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.memory.min`
: Information about memory limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.memory.max`
: Information about memory limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.memory.default`
: Information about memory limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.memory.default_request`
: Information about memory limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.limitrange.memory.max_limit_request_ratio`
: Information about memory limit range usage by constraint. Tags:`kube_namespace` `limitrange` `type`.

`kubernetes_state.service.count`
: Number of services. Tags:`kube_namespace` `type`.

`kubernetes_state.service.type`
: Service types. Tags:`kube_namespace` `kube_service` `type`.

`kubernetes_state.ingress.count`
: Number of ingresses. Tags:`kube_namespace`.

`kubernetes_state.ingress.path`
: Information about the ingress path. Tags:`kube_namespace` `kube_ingress_path` `kube_ingress` `kube_service` `kube_service_port` `kube_ingress_host` .

### Events

The Kubernetes State Metrics Core check does not include any events.

### Service Checks

`kubernetes_state.cronjob.complete`
: Whether the last job of the cronjob is failed or not. Tags:`kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.cronjob.on_schedule_check`
: Alert if the cronjob's next schedule is in the past. Tags:`kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.job.complete`
: Whether the job is failed or not. Tags:`kube_job` or `kube_cronjob` `kube_namespace` (`env` `service` `version` from standard labels).

`kubernetes_state.node.ready`
: Whether the node is ready. Tags:`node` `condition` `status`.

`kubernetes_state.node.out_of_disk`
: Whether the node is out of disk. Tags:`node` `condition` `status`.

`kubernetes_state.node.disk_pressure`
: Whether the node is under disk pressure. Tags:`node` `condition` `status`.

`kubernetes_state.node.network_unavailable`
: Whether the node network is unavailable. Tags:`node` `condition` `status`.

`kubernetes_state.node.memory_pressure`
: Whether the node network is under memory pressure. Tags:`node` `condition` `status`.
