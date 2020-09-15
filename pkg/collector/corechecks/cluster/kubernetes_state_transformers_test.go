// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/stretchr/testify/assert"
)

type args struct {
	name     string
	metric   ksmstore.DDMetric
	hostname string
	tags     []string
	now      func() time.Time
}

type serviceCheck struct {
	name     string
	status   metrics.ServiceCheckStatus
	tags     []string
	hostname string
	message  string
}

func Test_resourcequotaTransformer(t *testing.T) {
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
				tags:     []string{"resourcequota:gke-resource-quotas", "foo:bar"},
				hostname: "foo",
			},
			expected: &metricsExpected{
				name:     "kubernetes_state.resourcequota.pods.limit",
				val:      15000,
				tags:     []string{"resourcequota:gke-resource-quotas", "foo:bar"},
				hostname: "foo",
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
			resourcequotaTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_cronJobNextScheduleTransformer(t *testing.T) {
	type serviceCheck struct {
		name     string
		status   metrics.ServiceCheckStatus
		hostname string
		tags     []string
		message  string
	}
	tests := []struct {
		name     string
		args     args
		expected *serviceCheck
	}{
		{
			name: "On schedule",
			args: args{
				name: "kube_cronjob_next_schedule_time",
				metric: ksmstore.DDMetric{
					Val: 1595501615,
					Labels: map[string]string{
						"cronjob":   "foo",
						"namespace": "default",
					},
				},
				tags:     []string{"cronjob:foo", "namespace:default"},
				hostname: "foo",
				now:      func() time.Time { return time.Unix(int64(1595501615-2), 0) },
			},
			expected: &serviceCheck{
				name:     "kubernetes_state.cronjob.on_schedule_check",
				status:   metrics.ServiceCheckOK,
				tags:     []string{"cronjob:foo", "namespace:default"},
				hostname: "foo",
				message:  "",
			},
		},
		{
			name: "Late",
			args: args{
				name: "kube_cronjob_next_schedule_time",
				metric: ksmstore.DDMetric{
					Val: 1595501615,
					Labels: map[string]string{
						"cronjob":   "foo",
						"namespace": "default",
					},
				},
				tags: []string{"cronjob:foo", "namespace:default"},
				now:  func() time.Time { return time.Unix(int64(1595501615+2), 0) },
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.on_schedule_check",
				status:  metrics.ServiceCheckCritical,
				tags:    []string{"cronjob:foo", "namespace:default"},
				message: "The cron job check scheduled at 2020-07-23 10:53:35 +0000 UTC is 2 seconds late",
			},
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			now = tt.args.now
			cronJobNextScheduleTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, tt.args.hostname, tt.args.tags, tt.expected.message)
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobCompleteTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *serviceCheck
	}{
		{
			name: "nominal case, job_name tag",
			args: args{
				name: "kube_job_complete",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: &serviceCheck{
				name:   "kubernetes_state.job.complete",
				status: metrics.ServiceCheckOK,
				tags:   []string{"job_name:foo", "namespace:default"},
			},
		},
		{
			name: "nominal case, job tag",
			args: args{
				name: "kube_job_complete",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job":       "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job:foo-1509998340", "namespace:default"},
			},
			expected: &serviceCheck{
				name:   "kubernetes_state.job.complete",
				status: metrics.ServiceCheckOK,
				tags:   []string{"job:foo", "namespace:default"},
			},
		},
		{
			name: "inactive",
			args: args{
				name: "kube_job_complete",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			jobCompleteTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, tt.args.hostname, tt.args.tags, "")
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobFailedTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *serviceCheck
	}{
		{
			name: "nominal case, job_name tag",
			args: args{
				name: "kube_job_failed",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: &serviceCheck{
				name:   "kubernetes_state.job.complete",
				status: metrics.ServiceCheckCritical,
				tags:   []string{"job_name:foo", "namespace:default"},
			},
		},
		{
			name: "nominal case, job tag",
			args: args{
				name: "kube_job_failed",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job":       "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job:foo-1509998340", "namespace:default"},
			},
			expected: &serviceCheck{
				name:   "kubernetes_state.job.complete",
				status: metrics.ServiceCheckCritical,
				tags:   []string{"job:foo", "namespace:default"},
			},
		},
		{
			name: "inactive",
			args: args{
				name: "kube_job_failed",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			jobFailedTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, tt.args.hostname, tt.args.tags, "")
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobStatusSucceededTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "nominal case, job_name tag",
			args: args{
				name: "kube_job_status_succeeded",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.job.succeeded",
				val:  1,
				tags: []string{"job_name:foo", "namespace:default"},
			},
		},
		{
			name: "nominal case, job tag",
			args: args{
				name: "kube_job_status_succeeded",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job":       "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job:foo-1509998340", "namespace:default"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.job.succeeded",
				val:  1,
				tags: []string{"job:foo", "namespace:default"},
			},
		},
		{
			name: "inactive",
			args: args{
				name: "kube_job_status_succeeded",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			jobStatusSucceededTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_jobStatusFailedTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "nominal case, job_name tag",
			args: args{
				name: "kube_job_status_failed",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.job.failed",
				val:  1,
				tags: []string{"job_name:foo", "namespace:default"},
			},
		},
		{
			name: "nominal case, job tag",
			args: args{
				name: "kube_job_status_failed",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"job":       "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job:foo-1509998340", "namespace:default"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.job.failed",
				val:  1,
				tags: []string{"job:foo", "namespace:default"},
			},
		},
		{
			name: "inactive",
			args: args{
				name: "kube_job_status_failed",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"job_name":  "foo-1509998340",
						"namespace": "default",
					},
				},
				tags: []string{"job_name:foo-1509998340", "namespace:default"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			jobStatusFailedTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_pvPhaseTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "Active",
			args: args{
				name: "kube_persistentvolume_status_phase",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"persistentvolume": "local-pv-103fef5d",
						"phase":            "Available",
					},
				},
				tags: []string{"persistentvolume:local-pv-103fef5d", "phase:Available"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.persistentvolume.by_phase",
				val:  1,
				tags: []string{"persistentvolume:local-pv-103fef5d", "phase:Available"},
			},
		},
		{
			name: "Not active",
			args: args{
				name: "kube_persistentvolume_status_phase",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"persistentvolume": "local-pv-103fef5d",
						"phase":            "Available",
					},
				},
				tags: []string{"persistentvolume:local-pv-103fef5d", "phase:Available"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			pvPhaseTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_serviceTypeTransformer(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "Active",
			args: args{
				name: "kube_service_spec_type",
				metric: ksmstore.DDMetric{
					Val: 1,
					Labels: map[string]string{
						"namespace": "default",
						"service":   "kubernetes",
						"type":      "ClusterIP",
					},
				},
				tags: []string{"namespace:default", "service:kubernetes", "type:ClusterIP"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.service.type",
				val:  1,
				tags: []string{"namespace:default", "service:kubernetes", "type:ClusterIP"},
			},
		},
		{
			name: "Not active",
			args: args{
				name: "kube_service_spec_type",
				metric: ksmstore.DDMetric{
					Val: 0,
					Labels: map[string]string{
						"namespace": "default",
						"service":   "kubernetes",
						"type":      "ClusterIP",
					},
				},
				tags: []string{"namespace:default", "service:kubernetes", "type:ClusterIP"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			serviceTypeTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_podPhaseTransformer(t *testing.T) {
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
			podPhaseTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_containerWaitingReasonTransformer(t *testing.T) {
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
			containerWaitingReasonTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_containerTerminatedReasonTransformer(t *testing.T) {
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
			containerTerminatedReasonTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_limitrangeTransformer(t *testing.T) {
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
			limitrangeTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_nodeUnschedulableTransformer(t *testing.T) {
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
			nodeUnschedulableTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, tt.args.hostname, tt.args.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_nodeConditionTransformer(t *testing.T) {
	tests := []struct {
		name                 string
		args                 args
		expectedServiceCheck *serviceCheck
		expectedMetric       *metricsExpected
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			expectedMetric: &metricsExpected{
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
			nodeConditionTransformer(s, tt.args.name, tt.args.metric, tt.args.hostname, tt.args.tags)
			if tt.expectedServiceCheck != nil {
				s.AssertServiceCheck(t, tt.expectedServiceCheck.name, tt.expectedServiceCheck.status, tt.expectedServiceCheck.hostname, tt.expectedServiceCheck.tags, tt.expectedServiceCheck.message)
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
			if tt.expectedMetric != nil {
				s.AssertMetric(t, "Gauge", tt.expectedMetric.name, tt.expectedMetric.val, tt.expectedMetric.hostname, tt.expectedMetric.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_validateJob(t *testing.T) {
	tests := []struct {
		name  string
		val   float64
		tags  []string
		want  []string
		want1 bool
	}{
		{
			name:  "kube_job",
			val:   1.0,
			tags:  []string{"foo:bar", "kube_job:foo-1600167000"},
			want:  []string{"foo:bar", "kube_job:foo"},
			want1: true,
		},
		{
			name:  "job",
			val:   1.0,
			tags:  []string{"foo:bar", "job:foo-1600167000"},
			want:  []string{"foo:bar", "job:foo"},
			want1: true,
		},
		{
			name:  "job_name and kube_job",
			val:   1.0,
			tags:  []string{"foo:bar", "job_name:foo-1600167000", "kube_job:foo-1600167000"},
			want:  []string{"foo:bar", "job_name:foo", "kube_job:foo"},
			want1: true,
		},
		{
			name:  "invalid",
			val:   0.0,
			tags:  []string{"foo:bar", "job_name:foo"},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := validateJob(tt.val, tt.tags)
			assert.ElementsMatch(t, got, tt.want)
			assert.Equal(t, got1, tt.want1)
		})
	}
}
