// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// ksmMetricPrefix defines the KSM metrics namespace
const ksmMetricPrefix = "kubernetes_state."

// defaultMetricNamesMapper returns a map that translates KSM metric names to Datadog metric names
func defaultMetricNamesMapper() map[string]string {
	return map[string]string{
		"kube_apiservice_status_condition":                                                         "apiservice.condition",
		"kube_customresourcedefinition_status_condition":                                           "crd.condition",
		"kube_daemonset_status_current_number_scheduled":                                           "daemonset.scheduled",
		"kube_daemonset_status_desired_number_scheduled":                                           "daemonset.desired",
		"kube_daemonset_status_number_misscheduled":                                                "daemonset.misscheduled",
		"kube_daemonset_status_number_ready":                                                       "daemonset.ready",
		"kube_daemonset_status_updated_number_scheduled":                                           "daemonset.updated",
		"kube_deployment_spec_paused":                                                              "deployment.paused",
		"kube_deployment_spec_replicas":                                                            "deployment.replicas_desired",
		"kube_deployment_spec_strategy_rollingupdate_max_unavailable":                              "deployment.rollingupdate.max_unavailable",
		"kube_deployment_spec_strategy_rollingupdate_max_surge":                                    "deployment.rollingupdate.max_surge",
		"kube_deployment_status_replicas":                                                          "deployment.replicas",
		"kube_deployment_status_replicas_ready":                                                    "deployment.replicas_ready",
		"kube_deployment_status_replicas_available":                                                "deployment.replicas_available",
		"kube_deployment_status_replicas_unavailable":                                              "deployment.replicas_unavailable",
		"kube_deployment_status_replicas_updated":                                                  "deployment.replicas_updated",
		"kube_deployment_status_condition":                                                         "deployment.condition",
		"kube_daemonset_status_number_unavailable":                                                 "daemonset.daemons_unavailable",
		"kube_daemonset_status_number_available":                                                   "daemonset.daemons_available",
		"kube_endpoint_address_available":                                                          "endpoint.address_available",
		"kube_endpoint_address_not_ready":                                                          "endpoint.address_not_ready",
		"kube_pod_container_status_terminated":                                                     "container.terminated",
		"kube_pod_container_status_waiting":                                                        "container.waiting",
		"kube_pod_init_container_status_waiting":                                                   "initcontainer.waiting",
		"kube_pod_init_container_status_restarts_total":                                            "initcontainer.restarts",
		"kube_persistentvolumeclaim_status_phase":                                                  "persistentvolumeclaim.status",
		"kube_persistentvolumeclaim_access_mode":                                                   "persistentvolumeclaim.access_mode",
		"kube_persistentvolumeclaim_resource_requests_storage_bytes":                               "persistentvolumeclaim.request_storage",
		"kube_persistentvolume_capacity_bytes":                                                     "persistentvolume.capacity",
		"kube_pod_container_status_ready":                                                          "container.ready",
		"kube_pod_container_status_restarts_total":                                                 "container.restarts",
		"kube_pod_container_status_running":                                                        "container.running",
		"kube_pod_status_ready":                                                                    "pod.ready",
		"kube_pod_status_scheduled":                                                                "pod.scheduled",
		"kube_pod_spec_volumes_persistentvolumeclaims_readonly":                                    "pod.volumes.persistentvolumeclaims_readonly",
		"kube_pod_status_unschedulable":                                                            "pod.unschedulable",
		"kube_pod_tolerations":                                                                     "pod.tolerations",
		"kube_poddisruptionbudget_status_current_healthy":                                          "pdb.pods_healthy",
		"kube_poddisruptionbudget_status_desired_healthy":                                          "pdb.pods_desired",
		"kube_poddisruptionbudget_status_pod_disruptions_allowed":                                  "pdb.disruptions_allowed",
		"kube_poddisruptionbudget_status_expected_pods":                                            "pdb.pods_total",
		"kube_secret_type":                                                                         "secret.type",
		"kube_replicaset_spec_replicas":                                                            "replicaset.replicas_desired",
		"kube_replicaset_status_fully_labeled_replicas":                                            "replicaset.fully_labeled_replicas",
		"kube_replicaset_status_ready_replicas":                                                    "replicaset.replicas_ready",
		"kube_replicaset_status_replicas":                                                          "replicaset.replicas",
		"kube_replicationcontroller_spec_replicas":                                                 "replicationcontroller.replicas_desired",
		"kube_replicationcontroller_status_available_replicas":                                     "replicationcontroller.replicas_available",
		"kube_replicationcontroller_status_fully_labeled_replicas":                                 "replicationcontroller.fully_labeled_replicas",
		"kube_replicationcontroller_status_ready_replicas":                                         "replicationcontroller.replicas_ready",
		"kube_replicationcontroller_status_replicas":                                               "replicationcontroller.replicas",
		"kube_statefulset_replicas":                                                                "statefulset.replicas_desired",
		"kube_statefulset_status_replicas":                                                         "statefulset.replicas",
		"kube_statefulset_status_replicas_current":                                                 "statefulset.replicas_current",
		"kube_statefulset_status_replicas_ready":                                                   "statefulset.replicas_ready",
		"kube_statefulset_status_replicas_updated":                                                 "statefulset.replicas_updated",
		"kube_horizontalpodautoscaler_spec_min_replicas":                                           "hpa.min_replicas",
		"kube_horizontalpodautoscaler_spec_max_replicas":                                           "hpa.max_replicas",
		"kube_horizontalpodautoscaler_spec_target_metric":                                          "hpa.spec_target_metric",
		"kube_horizontalpodautoscaler_status_condition":                                            "hpa.condition",
		"kube_horizontalpodautoscaler_status_desired_replicas":                                     "hpa.desired_replicas",
		"kube_horizontalpodautoscaler_status_current_replicas":                                     "hpa.current_replicas",
		"kube_horizontalpodautoscaler_status_target_metric":                                        "hpa.status_target_metric",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_lowerbound":     "vpa.lower_bound",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_target":         "vpa.target",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_uncappedtarget": "vpa.uncapped_target",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_upperbound":     "vpa.upperbound",
		"kube_verticalpodautoscaler_spec_updatepolicy_updatemode":                                  "vpa.update_mode",
		"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_minallowed":             "vpa.spec_container_minallowed",
		"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_maxallowed":             "vpa.spec_container_maxallowed",
		"kube_cronjob_spec_suspend":                                                                "cronjob.spec_suspend",
		"kube_job_duration":                                                                        "job.duration",
		"kube_ingress_path":                                                                        "ingress.path",
	}
}

// defaultLabelsMapper returns a map that contains the default labels to tag names mapping
func defaultLabelsMapper() map[string]string {
	return map[string]string{
		"namespace":                           tags.KubeNamespace,
		"job_name":                            tags.KubeJob,
		"cronjob":                             tags.KubeCronjob,
		"pod":                                 tags.KubePod,
		"priority_class":                      tags.KubePriorityClass,
		"daemonset":                           tags.KubeDaemonSet,
		"replicationcontroller":               tags.KubeReplicationController,
		"replicaset":                          tags.KubeReplicaSet,
		"statefulset":                         tags.KubeStatefulSet,
		"deployment":                          tags.KubeDeployment,
		"container":                           tags.KubeContainerName,
		"container_id":                        tags.ContainerID,
		"image":                               tags.ImageName,
		"endpoint":                            "kube_endpoint",
		"label_topology_kubernetes_io_region": "kube_region",
		"label_topology_kubernetes_io_zone":   "kube_zone",
		"label_failure_domain_beta_kubernetes_io_region": "kube_region",
		"label_failure_domain_beta_kubernetes_io_zone":   "kube_zone",
		"ingress": "kube_ingress",

		// Standard Datadog labels
		"label_tags_datadoghq_com_env":     tags.Env,
		"label_tags_datadoghq_com_service": tags.Service,
		"label_tags_datadoghq_com_version": tags.Version,

		// Standard Kubernetes labels
		"label_app_kubernetes_io_name":       tags.KubeAppName,
		"label_app_kubernetes_io_instance":   tags.KubeAppInstance,
		"label_app_kubernetes_io_version":    tags.KubeAppVersion,
		"label_app_kubernetes_io_component":  tags.KubeAppComponent,
		"label_app_kubernetes_io_part_of":    tags.KubeAppPartOf,
		"label_app_kubernetes_io_managed_by": tags.KubeAppManagedBy,

		// Standard Helm labels
		"label_helm_sh_chart": "helm_chart",
	}
}

// defaultLabelJoins returns a map that contains the default label joins configuration
func defaultLabelJoins() map[string]*JoinsConfigWithoutLabelsMapping {
	defaultStandardLabels := []string{
		// Standard Datadog labels
		"label_tags_datadoghq_com_env",
		"label_tags_datadoghq_com_service",
		"label_tags_datadoghq_com_version",

		// Standard Kubernetes labels
		"label_app_kubernetes_io_name",
		"label_app_kubernetes_io_instance",
		"label_app_kubernetes_io_version",
		"label_app_kubernetes_io_component",
		"label_app_kubernetes_io_part_of",
		"label_app_kubernetes_io_managed_by",

		// Standard Helm labels
		"label_helm_sh_chart",
	}

	return map[string]*JoinsConfigWithoutLabelsMapping{
		// Metrics definition: https://github.com/kubernetes/kube-state-metrics/tree/main/docs/metrics
		// Cluster
		"kube_namespace_status_phase": {
			LabelsToMatch: getLabelToMatchForKind("namespace"),
			LabelsToGet:   []string{"phase"},
		},
		"kube_node_labels": {
			LabelsToMatch: getLabelToMatchForKind("node"),
			LabelsToGet: []string{
				"label_tags_datadoghq_com_env",
				"label_app_kubernetes_io_name",
				"label_app_kubernetes_io_instance",
				"label_app_kubernetes_io_version",
				"label_app_kubernetes_io_component",
				"label_app_kubernetes_io_part_of",
				"label_app_kubernetes_io_managed_by",
				"label_topology_kubernetes_io_region",            // k8s v1.17+
				"label_topology_kubernetes_io_zone",              // k8s v1.17+
				"label_failure_domain_beta_kubernetes_io_region", // k8s < v1.17
				"label_failure_domain_beta_kubernetes_io_zone",   // k8s < v1.17
			},
		},
		"kube_node_info": {
			LabelsToMatch: getLabelToMatchForKind("node"),
			LabelsToGet:   []string{"kernel_version", "os_image", "container_runtime_version", "kubelet_version"},
		},
		"kube_node_status_allocatable": {
			LabelsToMatch: getLabelToMatchForKind("node"),
			LabelsToGet:   []string{},
		},
		"kube_node_status_capacity": {
			LabelsToMatch: getLabelToMatchForKind("node"),
			LabelsToGet:   []string{},
		},
		// Service
		"kube_endpoint_labels": {
			LabelsToMatch: getLabelToMatchForKind("endpoint"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_ingress_labels": {
			LabelsToMatch: getLabelToMatchForKind("ingress"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_service_spec_type": {
			LabelsToMatch: getLabelToMatchForKind("service"),
			LabelsToGet:   []string{"namespace", "type"},
		},
		// Storage
		"kube_configmap_labels": {
			LabelsToMatch: getLabelToMatchForKind("configmap"),
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_configmap_info": {
			LabelsToMatch: getLabelToMatchForKind("configmap"),
			LabelsToGet:   []string{"namespace"},
		},
		"kube_persistentvolume_labels": {
			LabelsToMatch: getLabelToMatchForKind("persistentvolume"),
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_persistentvolume_status_phase": {
			LabelsToMatch: getLabelToMatchForKind("persistentvolume"),
			LabelsToGet:   []string{"phase"},
		},
		"kube_persistentvolume_info": {
			LabelsToMatch: getLabelToMatchForKind("persistentvolume"),
			LabelsToGet:   []string{"storageclass"},
		},
		"kube_persistentvolumeclaim_info": {
			LabelsToMatch: getLabelToMatchForKind("persistentvolumeclaim"),
			LabelsToGet:   []string{"storageclass"},
		},
		"kube_secret_info": {
			LabelsToMatch: getLabelToMatchForKind("secret"),
			LabelsToGet:   []string{"secret", "namespace"},
		},
		// Workload
		"kube_cronjob_labels": {
			LabelsToMatch: getLabelToMatchForKind("cronjob"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_daemonset_labels": {
			LabelsToMatch: getLabelToMatchForKind("daemonset"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_deployment_labels": {
			LabelsToMatch: getLabelToMatchForKind("deployment"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_horizontalpodautoscaler_labels": {
			LabelsToMatch: getLabelToMatchForKind("horizontalpodautoscaler"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_job_owner": {
			LabelsToMatch: getLabelToMatchForKind("job"),
			LabelsToGet:   []string{"namespace", "owner_kind", "owner_name", "owner_is_controller"},
		},
		"kube_job_complete": {
			LabelsToMatch: getLabelToMatchForKind("job"),
			LabelsToGet:   []string{"namespace", "condition"},
		},
		"kube_job_failed": {
			LabelsToMatch: getLabelToMatchForKind("job"),
			LabelsToGet:   []string{"namespace", "condition"},
		},
		"kube_job_labels": {
			LabelsToMatch: getLabelToMatchForKind("job"),
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_pod_labels": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_pod_status_phase": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   []string{"phase"},
		},
		"kube_pod_status_reason": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   []string{"reason"},
		},
		"kube_pod_info": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   []string{"node", "created_by_kind", "created_by_name", "priority_class", "namespace"},
		},
		"kube_replicaset_labels": {
			LabelsToMatch: getLabelToMatchForKind("replicaset"),
			LabelsToGet:   append(defaultStandardLabels, []string{"namespace"}...),
		},
		"kube_replicaset_owner": {
			LabelsToMatch: getLabelToMatchForKind("replicaset"),
			LabelsToGet:   []string{"namespace", "owner_kind", "owner_name", "owner_is_controller"},
		},
		"kube_statefulset_labels": {
			LabelsToMatch: getLabelToMatchForKind("statefulset"),
			LabelsToGet:   defaultStandardLabels,
		},
		// Custom, defined in the datadog-agent side
		"kube_pod_container_resource_with_owner_tag_requests": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   []string{"pod", "node", "resource", "unit", "owner_kind", "owner_name", "namespace"},
		},
		"kube_pod_container_resource_with_owner_tag_limits": {
			LabelsToMatch: getLabelToMatchForKind("pod"),
			LabelsToGet:   []string{"pod", "node", "resource", "unit", "owner_kind", "owner_name", "namespace"},
		},
	}
}

// getLabelToMatchForKind returns the set of labels use to match
// a resource.
// this function centralized the logic about label_joins.labelToMatch
// configuration, because some resource like `job` is use a non standard
// label or and because some other resource doesn't need the `namespace` label.
func getLabelToMatchForKind(kind string) []string {
	switch kind {
	case "apiservice": // API Services are not namespaced
		return []string{"apiservice"}
	case "customresourcedefinition": // CRD are not namespaced
		return []string{"customresourcedefinition"}
	case "job": // job metrics use specific label
		return []string{"job_name", "namespace"}
	case "node": // persistent nodes are not namespaced
		return []string{"node"}
	case "persistentvolume": // persistent volumes are not namespaced
		return []string{"persistentvolume"}
	case "namespace": // namespace are not namespaced
		return []string{"namespace"}
	default:
		return []string{kind, "namespace"}
	}
}

func defaultAnnotationsAsTags() map[string]map[string]string {
	return map[string]map[string]string{
		"pod":        {kubernetes.RcIDAnnotKey: tags.RemoteConfigID, kubernetes.RcRevisionAnnotKey: tags.RemoteConfigRevision},
		"deployment": {kubernetes.RcIDAnnotKey: tags.RemoteConfigID, kubernetes.RcRevisionAnnotKey: tags.RemoteConfigRevision},
	}
}
