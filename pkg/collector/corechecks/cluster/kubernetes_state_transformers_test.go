// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func Test_resourcequotaTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type metricsExpected struct {
		val  float64
		name string
		tags []string
	}
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "nominal case, limit",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 15000,
					Labels: map[string]string{
						"resource":      "pods",
						"type":          "hard",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.resourcequota.pods.limit",
				val:  15000,
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
		},
		{
			name: "nominal case, used",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"resource":      "pods",
						"type":          "used",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.resourcequota.pods.used",
				val:  7,
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
		},
		{
			name: "no resource label",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"type":          "used",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: nil,
		},
		{
			name: "no type label",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"resource":      "pods",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			resourcequotaTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_limitrangeTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type metricsExpected struct {
		val  float64
		name string
		tags []string
	}
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "nominal case",
			args: args{
				name: "kube_limitrange",
				metric: ksmstore.DDMetric{
					Val: 0.1,
					Labels: map[string]string{
						"constraint": "defaultRequest",
						"limitrange": "limits",
						"resource":   "cpu",
					},
				},
				tags: []string{"constraint:default_request", "limitrange:limits", "resource:cpu"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.limitrange.cpu.default_request",
				val:  0.1,
				tags: []string{"constraint:default_request", "limitrange:limits", "resource:cpu"},
			},
		},
		{
			name: "no constraint label",
			args: args{
				name: "kube_limitrange",
				metric: ksmstore.DDMetric{
					Val: 0.1,
					Labels: map[string]string{
						"limitrange": "limits",
						"resource":   "cpu",
					},
				},
				tags: []string{"limitrange:limits", "resource:cpu"},
			},
			expected: nil,
		},
		{
			name: "invalid constraint label",
			args: args{
				name: "kube_limitrange",
				metric: ksmstore.DDMetric{
					Val: 0.1,
					Labels: map[string]string{
						"constraint": "foo",
						"limitrange": "limits",
						"resource":   "cpu",
					},
				},
				tags: []string{"constraint:foo", "limitrange:limits", "resource:cpu"},
			},
			expected: nil,
		},
		{
			name: "no resource label",
			args: args{
				name: "kube_limitrange",
				metric: ksmstore.DDMetric{
					Val: 0.1,
					Labels: map[string]string{
						"constraint": "defaultRequest",
						"limitrange": "limits",
					},
				},
				tags: []string{"constraint:default_request", "limitrange:limits"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			limitrangeTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_nodeUnschedulableTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type metricsExpected struct {
		val  float64
		name string
		tags []string
	}
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "schedulable",
			args: args{
				name: "kube_node_spec_unschedulable",
				metric: ksmstore.DDMetric{
					Val: 0.0,
					Labels: map[string]string{
						"node": "foo",
					},
				},
				tags: []string{"node:foo"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.node.status",
				val:  1.0,
				tags: []string{"node:foo", "status:schedulable"},
			},
		},
		{
			name: "unschedulable",
			args: args{
				name: "kube_node_spec_unschedulable",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node": "foo",
					},
				},
				tags: []string{"node:foo"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.node.status",
				val:  1.0,
				tags: []string{"node:foo", "status:unschedulable"},
			},
		},
		{
			name: "invalid",
			args: args{
				name: "kube_node_spec_unschedulable",
				metric: ksmstore.DDMetric{
					Val: 2.0,
					Labels: map[string]string{
						"node": "foo",
					},
				},
				tags: []string{"node:foo"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			nodeUnschedulableTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_nodeConditionTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type serviceCheck struct {
		name    string
		status  metrics.ServiceCheckStatus
		tags    []string
		message string
	}
	type metric struct {
		val  float64
		name string
		tags []string
	}
	tests := []struct {
		name                 string
		args                 args
		expectedServiceCheck *serviceCheck
		expectedMetric       *metric
	}{
		{
			name: "Ready",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:Ready", "status:true"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.ready",
				tags:    []string{"node:foo", "condition:Ready", "status:true"},
				status:  metrics.ServiceCheckOK,
				message: "foo is currently reporting Ready = true",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:Ready", "status:true"},
			},
		},
		{
			name: "Not Ready",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
						"status":    "false",
					},
				},
				tags: []string{"node:foo", "condition:Ready", "status:false"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.ready",
				tags:    []string{"node:foo", "condition:Ready", "status:false"},
				status:  metrics.ServiceCheckCritical,
				message: "foo is currently reporting Ready = false",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:Ready", "status:false"},
			},
		},
		{
			name: "Unknown Readiness",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
						"status":    "unknown",
					},
				},
				tags: []string{"node:foo", "condition:Ready", "status:unknown"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.ready",
				tags:    []string{"node:foo", "condition:Ready", "status:unknown"},
				status:  metrics.ServiceCheckUnknown,
				message: "foo is currently reporting Ready = unknown",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:Ready", "status:unknown"},
			},
		},
		{
			name: "Zero metric value",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 0.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:Ready", "status:true"},
			},
			expectedServiceCheck: nil,
			expectedMetric:       nil,
		},
		{
			name: "Invalid condition label",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "foo",
						"status":    "unknown",
					},
				},
				tags: []string{"node:foo", "condition:foo", "status:unknown"},
			},
			expectedServiceCheck: nil,
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:foo", "status:unknown"},
			},
		},
		{
			name: "Missing condition label",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":   "foo",
						"status": "unknown",
					},
				},
				tags: []string{"node:foo", "status:unknown"},
			},
			expectedServiceCheck: nil,
			expectedMetric:       nil,
		},
		{
			name: "Invalid status label",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
						"status":    "foo",
					},
				},
				tags: []string{"node:foo", "condition:Ready", "status:foo"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.ready",
				tags:    []string{"node:foo", "condition:Ready", "status:foo"},
				status:  metrics.ServiceCheckUnknown,
				message: "foo is currently reporting Ready = foo",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:Ready", "status:foo"},
			},
		},
		{
			name: "Missing status label",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "Ready",
					},
				},
				tags: []string{"node:foo", "condition:Ready"},
			},
			expectedServiceCheck: nil,
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:Ready"},
			},
		},
		{
			name: "Not OutOfDisk",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "OutOfDisk",
						"status":    "false",
					},
				},
				tags: []string{"node:foo", "condition:OutOfDisk", "status:false"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.out_of_disk",
				tags:    []string{"node:foo", "condition:OutOfDisk", "status:false"},
				status:  metrics.ServiceCheckOK,
				message: "foo is currently reporting OutOfDisk = false",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:OutOfDisk", "status:false"},
			},
		},
		{
			name: "OutOfDisk",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "OutOfDisk",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:OutOfDisk", "status:true"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.out_of_disk",
				tags:    []string{"node:foo", "condition:OutOfDisk", "status:true"},
				status:  metrics.ServiceCheckCritical,
				message: "foo is currently reporting OutOfDisk = true",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:OutOfDisk", "status:true"},
			},
		},
		{
			name: "DiskPressure",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "DiskPressure",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:DiskPressure", "status:true"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.disk_pressure",
				tags:    []string{"node:foo", "condition:DiskPressure", "status:true"},
				status:  metrics.ServiceCheckCritical,
				message: "foo is currently reporting DiskPressure = true",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:DiskPressure", "status:true"},
			},
		},
		{
			name: "NetworkUnavailable",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "NetworkUnavailable",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:NetworkUnavailable", "status:true"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.network_unavailable",
				tags:    []string{"node:foo", "condition:NetworkUnavailable", "status:true"},
				status:  metrics.ServiceCheckCritical,
				message: "foo is currently reporting NetworkUnavailable = true",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:NetworkUnavailable", "status:true"},
			},
		},
		{
			name: "MemoryPressure",
			args: args{
				name: "kube_node_status_condition",
				metric: ksmstore.DDMetric{
					Val: 1.0,
					Labels: map[string]string{
						"node":      "foo",
						"condition": "MemoryPressure",
						"status":    "true",
					},
				},
				tags: []string{"node:foo", "condition:MemoryPressure", "status:true"},
			},
			expectedServiceCheck: &serviceCheck{
				name:    "kubernetes_state.node.memory_pressure",
				tags:    []string{"node:foo", "condition:MemoryPressure", "status:true"},
				status:  metrics.ServiceCheckCritical,
				message: "foo is currently reporting MemoryPressure = true",
			},
			expectedMetric: &metric{
				name: "kubernetes_state.node.by_condition",
				val:  1.0,
				tags: []string{"node:foo", "condition:MemoryPressure", "status:true"},
			},
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			nodeConditionTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expectedServiceCheck != nil {
				s.AssertServiceCheck(t, tt.expectedServiceCheck.name, tt.expectedServiceCheck.status, "", tt.expectedServiceCheck.tags, tt.expectedServiceCheck.message)
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
			if tt.expectedMetric != nil {
				s.AssertMetric(t, "Gauge", tt.expectedMetric.name, tt.expectedMetric.val, "", tt.expectedMetric.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}
