// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package ksm

import (
	"regexp"

	"k8s.io/kube-state-metrics/v2/pkg/options"
)

// ksmMetricPrefix defines the KSM metrics namespace
const ksmMetricPrefix = "kubernetes_state."

var (
	// defaultLabelsMapper contains the default label to tag names mapping
	defaultLabelsMapper = map[string]string{
		"namespace":                           "kube_namespace",
		"job_name":                            "kube_job",
		"cronjob":                             "kube_cronjob",
		"pod":                                 "pod_name",
		"phase":                               "pod_phase",
		"daemonset":                           "kube_daemon_set",
		"replicationcontroller":               "kube_replication_controller",
		"replicaset":                          "kube_replica_set",
		"statefulset ":                        "kube_stateful_set",
		"deployment":                          "kube_deployment",
		"service":                             "kube_service",
		"endpoint":                            "kube_endpoint",
		"container":                           "kube_container_name",
		"container_id":                        "container_id",
		"image":                               "image_name",
		"label_tags_datadoghq_com_env":        "env",
		"label_tags_datadoghq_com_service":    "service",
		"label_tags_datadoghq_com_version":    "version",
		"label_topology_kubernetes_io_region": "kube_region",
		"label_topology_kubernetes_io_zone":   "kube_zone",
		"label_failure_domain_beta_kubernetes_io_region": "kube_region",
		"label_failure_domain_beta_kubernetes_io_zone":   "kube_zone",
	}

	// metricNamesMapper translates KSM metric names to Datadog metric names
	metricNamesMapper = map[string]string{
		"kube_daemonset_status_current_number_scheduled":                                           "daemonset.scheduled",
		"kube_daemonset_status_desired_number_scheduled":                                           "daemonset.desired",
		"kube_daemonset_status_number_misscheduled":                                                "daemonset.misscheduled",
		"kube_daemonset_status_number_ready":                                                       "daemonset.ready",
		"kube_daemonset_updated_number_scheduled":                                                  "daemonset.updated",
		"kube_deployment_spec_paused":                                                              "deployment.paused",
		"kube_deployment_spec_replicas":                                                            "deployment.replicas_desired",
		"kube_deployment_spec_strategy_rollingupdate_max_unavailable":                              "deployment.rollingupdate.max_unavailable",
		"kube_deployment_spec_strategy_rollingupdate_max_surge":                                    "deployment.rollingupdate.max_surge",
		"kube_deployment_status_replicas":                                                          "deployment.replicas",
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
		"kube_pod_container_resource_requests_cpu_cores":                                           "container.cpu_requested",
		"kube_pod_container_resource_limits_cpu_cores":                                             "container.cpu_limit",
		"kube_pod_container_resource_limits_memory_bytes":                                          "container.memory_limit",
		"kube_pod_container_resource_requests_memory_bytes":                                        "container.memory_requested",
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
		"kube_horizontalpodautoscaler_status_condition":                                            "hpa.condition",
		"kube_horizontalpodautoscaler_status_desired_replicas":                                     "hpa.desired_replicas",
		"kube_horizontalpodautoscaler_status_current_replicas":                                     "hpa.current_replicas",
		"kube_horizontalpodautoscaler_spec_target_metric":                                          "hpa.spec_target_metric",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_lowerbound":     "vpa.lower_bound",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_target":         "vpa.target",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_uncappedtarget": "vpa.uncapped_target",
		"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_upperbound":     "vpa.upperbound",
		"kube_verticalpodautoscaler_spec_updatepolicy_updatemode":                                  "vpa.update_mode",
		"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_minallowed":             "vpa.spec_container_minallowed",
		"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_maxallowed":             "vpa.spec_container_maxallowed",
		"kube_cronjob_spec_suspend":                                                                "cronjob.spec_suspend",
	}

	// metadata metrics are useful for label joins
	// but shouldn't be submitted to Datadog
	metadataMetricsRegex = regexp.MustCompile(".*_(info|labels|status_reason)")

	// defaultDeniedMetrics used to configure the KSM store to ignore these metrics by KSM engine
	defaultDeniedMetrics = options.MetricSet{
		".*_generation":                                    {},
		".*_metadata_resource_version":                     {},
		"kube_pod_owner":                                   {},
		"kube_pod_restart_policy":                          {},
		"kube_pod_completion_time":                         {},
		"kube_pod_status_scheduled_time":                   {},
		"kube_cronjob_status_active":                       {},
		"kube_node_status_phase":                           {},
		"kube_cronjob_spec_starting_deadline_seconds":      {},
		"kube_job_spec_active_dealine_seconds":             {},
		"kube_job_spec_completions":                        {},
		"kube_job_spec_parallelism":                        {},
		"kube_job_status_active":                           {},
		"kube_job_status_.*_time":                          {},
		"kube_service_spec_external_ip":                    {},
		"kube_service_status_load_balancer_ingress":        {},
		"kube_ingress_path":                                {},
		"kube_statefulset_status_current_revision":         {},
		"kube_statefulset_status_update_revision":          {},
		"kube_pod_container_status_last_terminated_reason": {},
		"kube_lease_renew_time":                            {},
	}

	defaultStandardLabels = []string{
		"label_tags_datadoghq_com_env",
		"label_tags_datadoghq_com_service",
		"label_tags_datadoghq_com_version",
	}

	// defaultLabelJoins contains the default label joins configuration
	defaultLabelJoins = map[string]*JoinsConfig{
		"kube_pod_status_phase": {
			LabelsToMatch: []string{"pod", "namespace"},
			LabelsToGet:   []string{"phase"},
		},
		"kube_pod_info": {
			LabelsToMatch: []string{"pod", "namespace"},
			LabelsToGet:   []string{"node", "created_by_kind", "created_by_name"},
		},
		"kube_persistentvolume_info": {
			LabelsToMatch: []string{"persistentvolume"}, // persistent volumes are not namespaced
			LabelsToGet:   []string{"storageclass"},
		},
		"kube_persistentvolumeclaim_info": {
			LabelsToMatch: []string{"persistentvolumeclaim", "namespace"},
			LabelsToGet:   []string{"storageclass"},
		},
		"kube_pod_labels": {
			LabelsToMatch: []string{"pod", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_pod_status_reason": {
			LabelsToMatch: []string{"pod", "namespace"},
			LabelsToGet:   []string{"reason"},
		},
		"kube_deployment_labels": {
			LabelsToMatch: []string{"deployment", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_replicaset_labels": {
			LabelsToMatch: []string{"replicaset", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_daemonset_labels": {
			LabelsToMatch: []string{"daemonset", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_statefulset_labels": {
			LabelsToMatch: []string{"statefulset", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_job_labels": {
			LabelsToMatch: []string{"job_name", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_cronjob_labels": {
			LabelsToMatch: []string{"cronjob", "namespace"},
			LabelsToGet:   defaultStandardLabels,
		},
		"kube_node_labels": {
			LabelsToMatch: []string{"node"},
			LabelsToGet: []string{
				"label_topology_kubernetes_io_region",            // k8s v1.17+
				"label_topology_kubernetes_io_zone",              // k8s v1.17+
				"label_failure_domain_beta_kubernetes_io_region", // k8s < v1.17
				"label_failure_domain_beta_kubernetes_io_zone",   // k8s < v1.17
			},
		},
	}
)
