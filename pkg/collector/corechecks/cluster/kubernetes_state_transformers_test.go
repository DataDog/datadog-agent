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

func Test_cronJobNextScheduleTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
		now    func() time.Time
	}
	type serviceCheck struct {
		name    string
		status  metrics.ServiceCheckStatus
		tags    []string
		message string
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
				tags: []string{"cronjob:foo", "namespace:default"},
				now:  func() time.Time { return time.Unix(int64(1595501615-2), 0) },
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.on_schedule_check",
				status:  metrics.ServiceCheckOK,
				tags:    []string{"cronjob:foo", "namespace:default"},
				message: "",
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
			cronJobNextScheduleTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, "", tt.expected.tags, tt.expected.message)
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobCompleteTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type serviceCheck struct {
		name   string
		status metrics.ServiceCheckStatus
		tags   []string
	}
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
			jobCompleteTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, "", tt.expected.tags, "")
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobFailedTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type serviceCheck struct {
		name   string
		status metrics.ServiceCheckStatus
		tags   []string
	}
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
			jobFailedTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, "", tt.expected.tags, "")
				s.AssertNumberOfCalls(t, "ServiceCheck", 1)
			} else {
				s.AssertNotCalled(t, "ServiceCheck")
			}
		})
	}
}

func Test_jobStatusSucceededTransformer(t *testing.T) {
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
			jobStatusSucceededTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}

func Test_jobStatusFailedTransformer(t *testing.T) {
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
			jobStatusFailedTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}
