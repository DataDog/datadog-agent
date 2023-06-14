// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/metrics"
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

	ksmCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{})

	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
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
	tests := []struct {
		name            string
		metricsComplete []ksmstore.DDMetric
		metricsFailed   []ksmstore.DDMetric
		expected        *serviceCheck
	}{
		{
			name: "Last job succeeded",
			metricsComplete: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-112",
						"condition": "true",
					},
					Val: 1,
				},
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-114",
						"condition": "true",
					},
					Val: 1,
				},
			},
			metricsFailed: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-113",
						"condition": "true",
					},
					Val: 1,
				},
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  metrics.ServiceCheckOK,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
		{
			name: "Last job failed",
			metricsFailed: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-112",
						"condition": "true",
					},
					Val: 1,
				},
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-114",
						"condition": "true",
					},
					Val: 1,
				},
			},
			metricsComplete: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "foo",
						"job_name":  "bar-113",
						"condition": "true",
					},
					Val: 1,
				},
			},
			expected: &serviceCheck{
				name:    "kubernetes_state.cronjob.complete",
				status:  metrics.ServiceCheckCritical,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
	}

	ksmCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{})

	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
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
