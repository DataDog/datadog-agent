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

func Test_podPhaseTransformer(t *testing.T) {
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
			name: "Active",
			args: args{
				name: "kube_pod_status_phase",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"pod":       "foo",
						"phase":     "Failed",
						"namespace": "default",
					},
				},
				tags: []string{"pod:foo", "phase:Failed", "namespace:default"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.pod.status_phase",
				val:  1,
				tags: []string{"pod:foo", "phase:Failed", "namespace:default"},
			},
		},
		{
			name: "Not active",
			args: args{
				name: "kube_pod_status_phase",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"pod":       "foo",
						"phase":     "Failed",
						"namespace": "default",
					},
				},
				tags: []string{"pod:foo", "phase:Failed", "namespace:default"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			podPhaseTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_containerWaitingReasonTransformer(t *testing.T) {
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
			name: "CLB",
			args: args{
				name: "kube_pod_container_status_waiting_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "CrashLoopBackOff",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:CrashLoopBackOff"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.waiting",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:CrashLoopBackOff"},
			},
		},
		{
			name: "ErrImagePull",
			args: args{
				name: "kube_pod_container_status_waiting_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "ErrImagePull",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ErrImagePull"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.waiting",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ErrImagePull"},
			},
		},
		{
			name: "ImagePullBackoff",
			args: args{
				name: "kube_pod_container_status_waiting_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "ImagePullBackoff",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ImagePullBackoff"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.waiting",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ImagePullBackoff"},
			},
		},
		{
			name: "ContainerCreating",
			args: args{
				name: "kube_pod_container_status_waiting_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "ContainerCreating",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ContainerCreating"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.waiting",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ContainerCreating"},
			},
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			containerWaitingReasonTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_containerTerminatedReasonTransformer(t *testing.T) {
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
			name: "OOMKilled",
			args: args{
				name: "kube_pod_container_status_terminated_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "OOMKilled",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:OOMKilled"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.terminated",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:OOMKilled"},
			},
		},
		{
			name: "ContainerCannotRun",
			args: args{
				name: "kube_pod_container_status_terminated_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "ContainerCannotRun",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ContainerCannotRun"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.terminated",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:ContainerCannotRun"},
			},
		},
		{
			name: "Error",
			args: args{
				name: "kube_pod_container_status_terminated_reason",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"container": "foo",
						"pod":       "bar",
						"namespace": "default",
						"reason":    "Error",
					},
				},
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:Error"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.container.status_report.count.terminated",
				val:  1,
				tags: []string{"container:foo", "pod:bar", "namespace:default", "reason:Error"},
			},
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			containerTerminatedReasonTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}
