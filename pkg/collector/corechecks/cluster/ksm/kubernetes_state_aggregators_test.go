// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

var _ metricAggregator = &sumValuesAggregator{}
var _ metricAggregator = &countObjectsAggregator{}
var _ metricAggregator = &lastCronJobCompleteAggregator{}
var _ metricAggregator = &lastCronJobFailedAggregator{}

func Test_counterAggregator(t *testing.T) {
	tests := []struct {
		name          string
		ddMetricName  string
		allowedLabels []string
		metrics       []ksmstore.DDMetric
		expected      []metricsExpected
	}{
		{
			name:          "One allowed label",
			ddMetricName:  "my.count",
			allowedLabels: []string{"foo"},
			metrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar1",
					},
					Val: 1,
				},
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar2",
					},
					Val: 2,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar1",
					},
					Val: 4,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar2",
					},
					Val: 8,
				},
			},
			expected: []metricsExpected{
				{
					name: "kubernetes_state.my.count",
					val:  1 + 2,
					tags: []string{"foo:foo1"},
				},
				{
					name: "kubernetes_state.my.count",
					val:  4 + 8,
					tags: []string{"foo:foo2"},
				},
			},
		},
		{
			name:          "Two allowed labels",
			ddMetricName:  "my.count",
			allowedLabels: []string{"foo", "bar"},
			metrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar1",
						"baz": "baz1",
					},
					Val: 1,
				},
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar1",
						"baz": "baz2",
					},
					Val: 2,
				},
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar2",
						"baz": "baz1",
					},
					Val: 4,
				},
				{
					Labels: map[string]string{
						"foo": "foo1",
						"bar": "bar2",
						"baz": "baz2",
					},
					Val: 8,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar1",
						"baz": "baz1",
					},
					Val: 16,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar1",
						"baz": "baz2",
					},
					Val: 32,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar2",
						"baz": "baz1",
					},
					Val: 64,
				},
				{
					Labels: map[string]string{
						"foo": "foo2",
						"bar": "bar2",
						"baz": "baz2",
					},
					Val: 128,
				},
			},
			expected: []metricsExpected{
				{
					name: "kubernetes_state.my.count",
					val:  1 + 2,
					tags: []string{"foo:foo1", "bar:bar1"},
				},
				{
					name: "kubernetes_state.my.count",
					val:  4 + 8,
					tags: []string{"foo:foo1", "bar:bar2"},
				},
				{
					name: "kubernetes_state.my.count",
					val:  16 + 32,
					tags: []string{"foo:foo2", "bar:bar1"},
				},
				{
					name: "kubernetes_state.my.count",
					val:  64 + 128,
					tags: []string{"foo:foo2", "bar:bar2"},
				},
			},
		},
	}

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ksmCheck := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{}, fakeTagger, nil)

	for _, tt := range tests {
		s := mocksender.NewMockSender(t, "ksm")
		s.SetupAcceptAll()

		t.Run(tt.name, func(t *testing.T) {
			agg := newSumValuesAggregator(tt.ddMetricName, "", tt.allowedLabels)
			for _, metric := range tt.metrics {
				agg.accumulate(metric)
			}

			agg.flush(s, ksmCheck, newLabelJoiner(ksmCheck.instance.labelJoins))

			s.AssertNumberOfCalls(t, "Gauge", len(tt.expected))
			for _, expected := range tt.expected {
				s.AssertMetric(t, "Gauge", expected.name, expected.val, expected.hostname, expected.tags)
			}
		})
	}
}

func Test_lastCronJobAggregator(t *testing.T) {
	// jobStartTime builds a kube_job_{complete,failed}_start_time metric
	jobStartTime := func(jobName string, startTime float64) ksmstore.DDMetric {
		return ksmstore.DDMetric{
			Labels: map[string]string{
				"namespace": "foo",
				"job_name":  jobName,
			},
			Val: startTime,
		}
	}

	tests := []struct {
		name            string
		metricsComplete []ksmstore.DDMetric
		metricsFailed   []ksmstore.DDMetric
		expected        *serviceCheck
	}{
		{
			name: "Last job succeeded",
			metricsComplete: []ksmstore.DDMetric{
				jobStartTime("bar-112", 1000),
				jobStartTime("bar-114", 1120),
			},
			metricsFailed: []ksmstore.DDMetric{
				jobStartTime("bar-113", 1060),
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  servicecheck.ServiceCheckOK,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
		{
			name: "Last job failed",
			metricsFailed: []ksmstore.DDMetric{
				jobStartTime("bar-112", 1000),
				jobStartTime("bar-114", 1120),
			},
			metricsComplete: []ksmstore.DDMetric{
				jobStartTime("bar-113", 1060),
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  servicecheck.ServiceCheckCritical,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
		{
			// A manually created Job (e.g. named by Argo CD with a yymmddHHMM
			// suffix) has a much higher suffix than the scheduled Jobs, which
			// use minutes since epoch. Its failure must not outlive later
			// successful scheduled runs.
			name: "Manual job with higher suffix failed before later scheduled jobs succeeded",
			metricsFailed: []ksmstore.DDMetric{
				jobStartTime("bar-2601011200", 1000),
			},
			metricsComplete: []ksmstore.DDMetric{
				jobStartTime("bar-29000001", 1060),
				jobStartTime("bar-29000002", 1120),
				jobStartTime("bar-29000003", 1180),
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  servicecheck.ServiceCheckOK,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
		{
			name: "Manual job with higher suffix succeeded before a later scheduled job failed",
			metricsComplete: []ksmstore.DDMetric{
				jobStartTime("bar-29000001", 1000),
				jobStartTime("bar-2601011200", 1060),
			},
			metricsFailed: []ksmstore.DDMetric{
				jobStartTime("bar-29000002", 1120),
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  servicecheck.ServiceCheckCritical,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
		{
			name: "Same start time falls back to the job name suffix",
			metricsComplete: []ksmstore.DDMetric{
				jobStartTime("bar-113", 1000),
			},
			metricsFailed: []ksmstore.DDMetric{
				jobStartTime("bar-112", 1000),
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  servicecheck.ServiceCheckOK,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
	}

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ksmCheck := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{}, fakeTagger, nil)

	for _, tt := range tests {
		s := mocksender.NewMockSender(t, "ksm")
		s.SetupAcceptAll()

		t.Run(tt.name, func(t *testing.T) {
			agg := newLastCronJobAggregator()
			aggComplete := &lastCronJobCompleteAggregator{aggregator: agg}
			aggFailed := &lastCronJobFailedAggregator{aggregator: agg}

			for _, metric := range tt.metricsComplete {
				aggComplete.accumulate(metric)
			}
			for _, metric := range tt.metricsFailed {
				aggFailed.accumulate(metric)
			}

			agg.flush(s, ksmCheck, newLabelJoiner(ksmCheck.instance.labelJoins))

			s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, "", tt.expected.tags, tt.expected.message)
			s.AssertNumberOfCalls(t, "ServiceCheck", 1)

			// Ingest the metrics in the other order
			for _, metric := range tt.metricsFailed {
				aggFailed.accumulate(metric)
			}
			for _, metric := range tt.metricsComplete {
				aggComplete.accumulate(metric)
			}

			agg.flush(s, ksmCheck, newLabelJoiner(ksmCheck.instance.labelJoins))

			s.AssertServiceCheck(t, tt.expected.name, tt.expected.status, "", tt.expected.tags, tt.expected.message)
			s.AssertNumberOfCalls(t, "ServiceCheck", 2)
		})
	}
}
