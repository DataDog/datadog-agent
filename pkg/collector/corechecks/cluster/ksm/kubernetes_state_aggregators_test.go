// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

var _ metricAggregator = &sumValuesAggregator{}
var _ metricAggregator = &countObjectsAggregator{}
var _ metricAggregator = &resourceAggregator{}
var _ metricAggregator = &lastCronJobCompleteAggregator{}
var _ metricAggregator = &lastCronJobFailedAggregator{}

func Test_MetricAggregators(t *testing.T) {
	tests := []struct {
		name                  string
		labelsAsTags          map[string]map[string]string
		ddMetricsFams         []ksmstore.DDMetricsFam
		expectedMetrics       []metricsExpected
		expectedServiceChecks []serviceCheck
	}{
		{
			name: "sumValuesAggregator aggregates namespace.count",
			labelsAsTags: map[string]map[string]string{
				"namespace": {
					"test_label_1": "tag1",
				},
				"pod": {
					"test_label_2": "tag2",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_namespace_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"namespace":          "default",
							"label_test_label_1": "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_namespace_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"namespace": "default",
							"phase":     "Active",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_namespace_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"namespace": "test1",
							"phase":     "Active",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_namespace_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"namespace": "test2",
							"phase":     "Active",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.namespace.count",
					tags:     []string{"phase:Active", "tag1:value1"},
					hostname: "",
				},
				{
					val:      2,
					name:     "kubernetes_state.namespace.count",
					tags:     []string{"phase:Active"},
					hostname: "",
				},
			},
		},
		{
			name: "sumValuesAggregator aggregates persistentvolumes.by_phase",
			labelsAsTags: map[string]map[string]string{
				"persistentvolumes": {
					"test_label_1": "tag1",
				},
				"persistentvolume": {
					"test_label_2": "tag2",
				},
				"node": {
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_persistentvolume_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume":             "pv-available-1",
							"label_tags_datadoghq_com_env": "test",
							"label_test_label_1":           "value1",
							"label_test_label_2":           "value2",
							"label_test_label_3":           "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-available-1",
							"phase":            "Available",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-available-2",
							"phase":            "Available",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-pending-1",
							"phase":            "Pending",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-released-1",
							"phase":            "Released",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-available-3",
							"phase":            "Available",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_persistentvolume_status_phase",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"persistentvolume": "pv-released-2",
							"phase":            "Released",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.persistentvolumes.by_phase",
					tags:     []string{"phase:Available", "tag2:value2", "env:test"},
					hostname: "",
				},
				{
					val:      2,
					name:     "kubernetes_state.persistentvolumes.by_phase",
					tags:     []string{"phase:Available"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.persistentvolumes.by_phase",
					tags:     []string{"phase:Pending"},
					hostname: "",
				},
				{
					val:      2,
					name:     "kubernetes_state.persistentvolumes.by_phase",
					tags:     []string{"phase:Released"},
					hostname: "",
				},
			},
		},
		{
			name: "countObjectsAggregator aggregates configmap.count",
			labelsAsTags: map[string]map[string]string{
				"configmap": {
					"test_label_1": "tag1",
					"test_label_2": "tag2",
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_configmap_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap":          "default-configmap-1",
							"namespace":          "default",
							"label_test_label_1": "value1",
							"label_test_label_2": "value2",
							"label_test_label_3": "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap":          "default-configmap-2",
							"namespace":          "default",
							"label_test_label_1": "value1",
							"label_test_label_3": "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap":                    "test-configmap-2",
							"namespace":                    "test",
							"label_tags_datadoghq_com_env": "unittest",
							"label_helm_sh_chart":          "unittest",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap": "default-configmap-1",
							"namespace": "default",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap": "default-configmap-2",
							"namespace": "default",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap": "default-configmap-3",
							"namespace": "default",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap": "test-configmap-1",
							"namespace": "test",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_configmap_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"configmap": "test-configmap-2",
							"namespace": "test",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.configmap.count",
					tags:     []string{"kube_namespace:default"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.configmap.count",
					tags:     []string{"kube_namespace:test"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.configmap.count",
					tags:     []string{"helm_chart:unittest", "env:unittest", "kube_namespace:test"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.configmap.count",
					tags:     []string{"kube_namespace:default", "tag1:value1", "tag2:value2", "tag3:value3"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.configmap.count",
					tags:     []string{"kube_namespace:default", "tag1:value1", "tag3:value3"},
					hostname: "",
				},
			},
		},
		{
			name: "countObjectsAggregator aggregates pod.count",
			labelsAsTags: map[string]map[string]string{
				"pod": {
					"test_label_1": "tag1",
					"test_label_2": "tag2",
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_pod_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":                              "datadog-agent-cptrq",
							"namespace":                        "datadog",
							"uid":                              "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"label_tags_datadoghq_com_env":     "test",
							"label_tags_datadoghq_com_service": "datadog-agent",
							"label_tags_datadoghq_com_version": "7",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":                              "datadog-agent-xrjxw",
							"namespace":                        "datadog",
							"uid":                              "4afcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"label_tags_datadoghq_com_env":     "test",
							"label_tags_datadoghq_com_service": "datadog-agent",
							"label_tags_datadoghq_com_version": "7",
							"label_test_label_2":               "value2",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":                              "datadog-agent-sbzqv",
							"namespace":                        "datadog",
							"uid":                              "5afcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"label_tags_datadoghq_com_env":     "test",
							"label_tags_datadoghq_com_service": "datadog-agent",
							"label_tags_datadoghq_com_version": "7",
							"label_test_label_3":               "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":             "datadog-agent-cptrq",
							"namespace":       "datadog",
							"uid":             "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"hostip":          "10.139.52.153",
							"podip":           "10.49.37.201",
							"node":            "nodeA",
							"created_by_kind": "DaemonSet",
							"created_by_name": "datadog-agent",
							"priority_class":  "",
							"host_network":    "false",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":             "datadog-agent-xrjxw",
							"namespace":       "datadog",
							"uid":             "4afcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"hostip":          "10.111.234.116",
							"podip":           "10.8.217.222",
							"node":            "nodeB",
							"created_by_kind": "DaemonSet",
							"created_by_name": "datadog-agent",
							"priority_class":  "",
							"host_network":    "false",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":             "datadog-agent-sbzqv",
							"namespace":       "datadog",
							"uid":             "5afcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"hostip":          "10.161.24.133",
							"podip":           "10.209.11.206",
							"node":            "nodeC",
							"created_by_kind": "DaemonSet",
							"created_by_name": "datadog-agent",
							"priority_class":  "",
							"host_network":    "false",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.pod.count",
					tags:     []string{"kube_namespace:datadog", "tag1:value1", "env:test", "service:datadog-agent", "version:7", "kube_daemon_set:datadog-agent", "node:nodeA"},
					hostname: "nodeA",
				},
				{
					val:      1,
					name:     "kubernetes_state.pod.count",
					tags:     []string{"kube_namespace:datadog", "tag2:value2", "env:test", "service:datadog-agent", "version:7", "kube_daemon_set:datadog-agent", "node:nodeB"},
					hostname: "nodeB",
				},
				{
					val:      1,
					name:     "kubernetes_state.pod.count",
					tags:     []string{"kube_namespace:datadog", "tag3:value3", "env:test", "service:datadog-agent", "version:7", "kube_daemon_set:datadog-agent", "node:nodeC"},
					hostname: "nodeC",
				},
			},
		},
		{
			name:         "countObjectsAggregator aggregates job.count",
			labelsAsTags: map[string]map[string]string{},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_job_owner",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "a-1562319360",
							"namespace": "test-ns-a",
							// Why are the following tags empty?
							// See https://github.com/kubernetes/kube-state-metrics/issues/1919
							"owner_kind":          "",
							"owner_name":          "",
							"owner_is_controller": "",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_owner",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":            "a-1562319361",
							"namespace":           "test-ns-a",
							"owner_kind":          "",
							"owner_name":          "",
							"owner_is_controller": "",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_owner",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":            "b-1562319360",
							"namespace":           "test-ns-b",
							"owner_kind":          "",
							"owner_name":          "",
							"owner_is_controller": "",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_owner",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":            "b-1562319361",
							"namespace":           "test-ns-b",
							"owner_kind":          "",
							"owner_name":          "",
							"owner_is_controller": "",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      2,
					name:     "kubernetes_state.job.count",
					tags:     []string{"kube_namespace:test-ns-a"},
					hostname: "",
				},
				{
					val:      2,
					name:     "kubernetes_state.job.count",
					tags:     []string{"kube_namespace:test-ns-b"},
					hostname: "",
				},
			},
		},
		{
			name:         "countObjectsAggregator aggregates cronjob.count",
			labelsAsTags: map[string]map[string]string{},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_cronjob_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"cronjob":                          "hello",
							"namespace":                        "test-ns-a",
							"label_tags_datadoghq_com_env":     "test-env",
							"label_tags_datadoghq_com_service": "hello-service",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_tag_1":                      "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_cronjob_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"cronjob":                          "hello2",
							"namespace":                        "test-ns-a",
							"label_tags_datadoghq_com_env":     "test-env",
							"label_tags_datadoghq_com_service": "hello-service2",
							"label_tags_datadoghq_com_version": "2.0.0",
							"label_tag_1":                      "value1",
							"label_tag_2":                      "value2",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.cronjob.count",
					tags:     []string{"kube_namespace:test-ns-a", "env:test-env", "service:hello-service", "version:1.0.0"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.cronjob.count",
					tags:     []string{"kube_namespace:test-ns-a", "env:test-env", "service:hello-service2", "version:2.0.0"},
					hostname: "",
				},
			},
		},
		{
			name:         "countObjectsAggregator aggregates node.count",
			labelsAsTags: map[string]map[string]string{},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_node_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":                                "node-a",
							"label_tags_datadoghq_com_env":        "test",
							"label_app_kubernetes_io_name":        "test-kubernetes-io-name",
							"label_app_kubernetes_io_instance":    "test-kubernetes-io-instance",
							"label_app_kubernetes_io_version":     "test-kubernetes-io-version",
							"label_app_kubernetes_io_component":   "test-kubernetes-io-component",
							"label_app_kubernetes_io_part_of":     "test-kubernetes-io-part-of",
							"label_app_kubernetes_io_managed_by":  "test-kubernetes-io-managed-by",
							"label_topology_kubernetes_io_region": "test-topology-kubernetes-io-region",
							"label_topology_kubernetes_io_zone":   "test-topology-kubernetes-io-zone",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_node_info",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":                      "node-a",
							"kernel_version":            "kernel-version-1",
							"os_image":                  "os-image-1",
							"container_runtime_version": "container-runtime-version-1",
							"kubelet_version":           "kubelet-version-1",
							"kubeproxy_version":         "kubelet-proxy-version-1",
							"pod_cidr":                  "10.10.0.0/23",
							"provider_id":               "provider-id-1",
							"system_uuid":               "0190247a-0235-7df1-ba57-b3e3f55e69d9",
							"internal_ip":               "10.10.0.5",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:  1,
					name: "kubernetes_state.node.count",
					tags: []string{
						"kube_app_instance:test-kubernetes-io-instance",
						"kube_app_name:test-kubernetes-io-name",
						"kernel_version:kernel-version-1",
						"kubelet_version:kubelet-version-1",
						"kube_zone:test-topology-kubernetes-io-zone",
						"os_image:os-image-1",
						"kube_app_managed_by:test-kubernetes-io-managed-by",
						"kube_app_version:test-kubernetes-io-version",
						"container_runtime_version:container-runtime-version-1",
						"kube_app_component:test-kubernetes-io-component",
						"kube_region:test-topology-kubernetes-io-region",
						"kube_app_part_of:test-kubernetes-io-part-of",
						"env:test",
					},
					hostname: "",
				},
			},
		},
		{
			name:         "countObjectsAggregator aggregates replicaset.count",
			labelsAsTags: map[string]map[string]string{},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_replicaset_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"replicaset":                       "squirtle-54cd675574",
							"namespace":                        "pokemon",
							"label_tags_datadoghq_com_env":     "ddenv",
							"label_tags_datadoghq_com_service": "ddservice",
							"label_tags_datadoghq_com_version": "ddversion",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_replicaset_owner",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"replicaset":          "squirtle-54cd675574",
							"namespace":           "pokemon",
							"owner_kind":          "Deployment",
							"owner_name":          "squirtle",
							"owner_is_controller": "true",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:  1,
					name: "kubernetes_state.replicaset.count",
					tags: []string{
						"kube_namespace:pokemon",
						"kube_deployment:squirtle",
						"owner_is_controller:true",
						"env:ddenv",
						"service:ddservice",
						"version:ddversion",
					},
					hostname: "",
				},
			},
		},
		{
			name:         "resourceAggregator aggregates node.cpu_allocatable and node.cpu_allocatable.total",
			labelsAsTags: map[string]map[string]string{},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_node_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":                         "node-a",
							"label_tags_datadoghq_com_env": "test-env-a",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_node_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":                         "node-b",
							"label_tags_datadoghq_com_env": "test-env-b",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_node_status_allocatable",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":     "node-a",
							"resource": "cpu",
							"unit":     "cpu",
						},
						Val: 0.5,
					}},
				},
				{
					Name: "kube_node_status_allocatable",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":     "node-b",
							"resource": "cpu",
							"unit":     "cpu",
						},
						Val: 1.5,
					}},
				},
				{
					Name: "kube_node_status_allocatable",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":     "node-c",
							"resource": "cpu",
							"unit":     "cpu",
						},
						Val: 0.75,
					}},
				},
				{
					Name: "kube_node_status_allocatable",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"node":     "node-d",
							"resource": "cpu",
							"unit":     "cpu",
						},
						Val: 0.8,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      0.5,
					name:     "kubernetes_state.node.cpu_allocatable",
					tags:     []string{"env:test-env-a", "resource:cpu", "unit:cpu", "node:node-a"},
					hostname: "node-a",
				},
				{
					val:      1.5,
					name:     "kubernetes_state.node.cpu_allocatable",
					tags:     []string{"env:test-env-b", "resource:cpu", "unit:cpu", "node:node-b"},
					hostname: "node-b",
				},
				{
					val:      0.75,
					name:     "kubernetes_state.node.cpu_allocatable",
					tags:     []string{"resource:cpu", "unit:cpu", "node:node-c"},
					hostname: "node-c",
				},
				{
					val:      0.8,
					name:     "kubernetes_state.node.cpu_allocatable",
					tags:     []string{"resource:cpu", "unit:cpu", "node:node-d"},
					hostname: "node-d",
				},
				{
					val:      0.5,
					name:     "kubernetes_state.node.cpu_allocatable.total",
					tags:     []string{"env:test-env-a"},
					hostname: "",
				},
				{
					val:      1.5,
					name:     "kubernetes_state.node.cpu_allocatable.total",
					tags:     []string{"env:test-env-b"},
					hostname: "",
				},
				{
					val:      1.55,
					name:     "kubernetes_state.node.cpu_allocatable.total",
					tags:     []string{},
					hostname: "",
				},
			},
		},
		{
			name: "resourceAggregator aggregates container.memory_requested.total",
			labelsAsTags: map[string]map[string]string{
				"pod": {
					"test_label_1": "tag1",
					"test_label_2": "tag2",
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_pod_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"pod":                              "abc-797f764658-458wg",
							"namespace":                        "ns-a",
							"uid":                              "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
							"label_tags_datadoghq_com_env":     "test",
							"label_tags_datadoghq_com_service": "abc-service",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
							"label_test_label_2":               "value2",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_pod_container_resource_with_owner_tag_requests",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"container":  "container-abc",
							"namespace":  "ns-a",
							"node":       "node-a",
							"resource":   "memory",
							"unit":       "byte",
							"owner_kind": "Deployment",
							"owner_name": "deployment-abc",
							"pod":        "abc-797f764658-458wg",
							"uid":        "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
						},
						Val: 10,
					}},
				},
				{
					Name: "kube_pod_container_resource_with_owner_tag_requests",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"container":  "container-def",
							"namespace":  "ns-a",
							"node":       "node-a",
							"resource":   "memory",
							"unit":       "byte",
							"owner_kind": "Deployment",
							"owner_name": "deployment-abc",
							"pod":        "abc-797f764658-458wg",
							"uid":        "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
						},
						Val: 10,
					}},
				},
				{
					Name: "kube_pod_container_resource_with_owner_tag_requests",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"container":  "container-hij",
							"namespace":  "ns-a",
							"node":       "node-a",
							"resource":   "memory",
							"unit":       "byte",
							"owner_kind": "Deployment",
							"owner_name": "deployment-abc",
							"pod":        "abc-797f764658-458wg",
							"uid":        "3bfcf55c-d399-433f-9a1e-64bb1dbefaa8",
						},
						Val: 10,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				// We don't expect kube_container_name here because this is a pod-level metric.
				{
					val:      30,
					name:     "kubernetes_state.container.memory_requested.total",
					tags:     []string{"tag1:value1", "tag2:value2", "env:test", "service:abc-service", "kube_namespace:ns-a", "node:node-a", "resource:memory", "unit:byte", "version:1.0.0", "kube_deployment:deployment-abc", "pod_name:abc-797f764658-458wg"},
					hostname: "node-a",
				},
			},
		},
		{
			name: "lastCronJobCompleteAggregator aggregates job.completion.succeeded and job.complete",
			labelsAsTags: map[string]map[string]string{
				"job": {
					"test_label_1": "tag1",
					"test_label_2": "tag2",
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562319360",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562319361",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562391362",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "konnichiwa-1562133906",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-konnichiwa",
							"label_tags_datadoghq_com_version": "1.2.0",
							"label_test_label_2":               "value2",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "bonjour-1562134910",
							"namespace":                        "ns-test-1",
							"label_tags_datadoghq_com_version": "2.0.0",
							"label_test_label_3":               "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_complete",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562319360",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_complete",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562319361",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_complete",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562391362",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_complete",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "konnichiwa-1562133906",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_complete",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "bonjour-1562134910",
							"namespace": "ns-test-1",
							"condition": "true",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.job.completion.succeeded",
					tags:     []string{"kube_job:hello-1562319360", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.succeeded",
					tags:     []string{"kube_job:hello-1562319361", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.succeeded",
					tags:     []string{"kube_job:hello-1562391362", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.succeeded",
					tags:     []string{"kube_job:konnichiwa-1562133906", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag2:value2", "service:service-konnichiwa", "version:1.2.0", "kube_cronjob:konnichiwa"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.succeeded",
					tags:     []string{"kube_job:bonjour-1562134910", "kube_namespace:ns-test-1", "kube_namespace:ns-test-1", "condition:true", "tag3:value3", "version:2.0.0", "kube_cronjob:bonjour"},
					hostname: "",
				},
			},
			expectedServiceChecks: []serviceCheck{
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckOK,
					hostname: "",
					tags:     []string{"kube_job:hello-1562319360", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckOK,
					hostname: "",
					tags:     []string{"kube_job:hello-1562319361", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckOK,
					hostname: "",
					tags:     []string{"kube_job:hello-1562391362", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckOK,
					hostname: "",
					tags:     []string{"kube_job:konnichiwa-1562133906", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag2:value2", "service:service-konnichiwa", "version:1.2.0", "kube_cronjob:konnichiwa"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckOK,
					hostname: "",
					tags:     []string{"kube_job:bonjour-1562134910", "kube_namespace:ns-test-1", "kube_namespace:ns-test-1", "condition:true", "tag3:value3", "version:2.0.0", "kube_cronjob:bonjour"},
					message:  "",
				},
			},
		},
		{
			name: "lastCronJobFailedAggregator aggregates job.completion.failure and job.complete",
			labelsAsTags: map[string]map[string]string{
				"job": {
					"test_label_1": "tag1",
					"test_label_2": "tag2",
					"test_label_3": "tag3",
				},
			},
			ddMetricsFams: []ksmstore.DDMetricsFam{
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562319360",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562319361",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "hello-1562391362",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-hello",
							"label_tags_datadoghq_com_version": "1.0.0",
							"label_test_label_1":               "value1",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "konnichiwa-1562133906",
							"namespace":                        "ns-test",
							"label_tags_datadoghq_com_env":     "env-test",
							"label_tags_datadoghq_com_service": "service-konnichiwa",
							"label_tags_datadoghq_com_version": "1.2.0",
							"label_test_label_2":               "value2",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_labels",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":                         "bonjour-1562134910",
							"namespace":                        "ns-test-1",
							"label_tags_datadoghq_com_version": "2.0.0",
							"label_test_label_3":               "value3",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_failed",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562319360",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_failed",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562319361",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_failed",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "hello-1562391362",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_failed",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "konnichiwa-1562133906",
							"namespace": "ns-test",
							"condition": "true",
						},
						Val: 1,
					}},
				},
				{
					Name: "kube_job_failed",
					ListMetrics: []ksmstore.DDMetric{{
						Labels: map[string]string{
							"job_name":  "bonjour-1562134910",
							"namespace": "ns-test-1",
							"condition": "true",
						},
						Val: 1,
					}},
				},
			},
			expectedMetrics: []metricsExpected{
				{
					val:      1,
					name:     "kubernetes_state.job.completion.failed",
					tags:     []string{"kube_job:hello-1562319360", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.failed",
					tags:     []string{"kube_job:hello-1562319361", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.failed",
					tags:     []string{"kube_job:hello-1562391362", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.failed",
					tags:     []string{"kube_job:konnichiwa-1562133906", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag2:value2", "service:service-konnichiwa", "version:1.2.0", "kube_cronjob:konnichiwa"},
					hostname: "",
				},
				{
					val:      1,
					name:     "kubernetes_state.job.completion.failed",
					tags:     []string{"kube_job:bonjour-1562134910", "kube_namespace:ns-test-1", "kube_namespace:ns-test-1", "condition:true", "tag3:value3", "version:2.0.0", "kube_cronjob:bonjour"},
					hostname: "",
				},
			},
			expectedServiceChecks: []serviceCheck{
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckCritical,
					hostname: "",
					tags:     []string{"kube_job:hello-1562319360", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckCritical,
					hostname: "",
					tags:     []string{"kube_job:hello-1562319361", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckCritical,
					hostname: "",
					tags:     []string{"kube_job:hello-1562391362", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag1:value1", "service:service-hello", "version:1.0.0", "kube_cronjob:hello"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckCritical,
					hostname: "",
					tags:     []string{"kube_job:konnichiwa-1562133906", "kube_namespace:ns-test", "kube_namespace:ns-test", "env:env-test", "condition:true", "tag2:value2", "service:service-konnichiwa", "version:1.2.0", "kube_cronjob:konnichiwa"},
					message:  "",
				},
				{
					name:     "kubernetes_state.job.complete",
					status:   servicecheck.ServiceCheckCritical,
					hostname: "",
					tags:     []string{"kube_job:bonjour-1562134910", "kube_namespace:ns-test-1", "kube_namespace:ns-test-1", "condition:true", "tag3:value3", "version:2.0.0", "kube_cronjob:bonjour"},
					message:  "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mocksender.NewMockSender("ksm")
			s.SetupAcceptAll()
			if tt.labelsAsTags == nil {
				tt.labelsAsTags = make(map[string]map[string]string)
			}
			ksmCheck := newKSMCheck(
				core.NewCheckBase(CheckName),
				&KSMConfig{
					LabelJoins:   make(map[string]*JoinsConfigWithoutLabelsMapping),
					LabelsAsTags: tt.labelsAsTags,
					LabelsMapper: make(map[string]string),
				},
			)
			ksmCheck.mergeLabelJoins(defaultLabelJoins())
			ksmCheck.processLabelJoins()
			ksmCheck.processLabelsAsTags()
			ksmCheck.mergeAnnotationsAsTags(defaultAnnotationsAsTags())
			ksmCheck.processAnnotationsAsTags()
			ksmCheck.mergeLabelsMapper(defaultLabelsMapper())
			ddMetricsFamilies := map[string][]ksmstore.DDMetricsFam{
				"test": nil,
			}
			ddMetricsFamilies["test"] = append(ddMetricsFamilies["test"], tt.ddMetricsFams...)
			lj := newLabelJoiner(ksmCheck.instance.labelJoins)
			// NOTE: Metrics with a value of 0 are not inseterted into labelJoiner.
			// See (*KSMCheck).metricFilter() for more details.
			lj.insertFamilies(ddMetricsFamilies)
			ksmCheck.processMetrics(s, ddMetricsFamilies, lj, time.Now())
			for _, m := range tt.expectedMetrics {
				s.AssertMetric(t, "Gauge", m.name, m.val, m.hostname, m.tags)
			}
			for _, sc := range tt.expectedServiceChecks {
				s.AssertServiceCheck(t, sc.name, sc.status, sc.hostname, sc.tags, sc.message)
			}
		})
	}
}
