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
var _ metricAggregator = &podScheduledTimeAggregator{}
var _ metricAggregator = &podReadyTimeAggregator{}

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
				status:  servicecheck.ServiceCheckOK,
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
				status:  servicecheck.ServiceCheckCritical,
				tags:    []string{"namespace:foo", "cronjob:bar"},
				message: "",
			},
		},
	}

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ksmCheck := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{}, fakeTagger, nil)

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

func Test_podTimeToReadyAggregator(t *testing.T) {
	tests := []struct {
		name             string
		scheduledMetrics []ksmstore.DDMetric
		readyMetrics     []ksmstore.DDMetric
		expected         []metricsExpected
	}{
		{
			name: "Single pod with both metrics",
			scheduledMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "my-pod",
						"uid":       "abc123",
					},
					Val: 1000.0, // scheduled at time 1000
				},
			},
			readyMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "my-pod",
						"uid":       "abc123",
					},
					Val: 1005.0, // ready at time 1005
				},
			},
			expected: []metricsExpected{
				{
					name: "kubernetes_state.pod.time_to_ready",
					val:  5.0, // 1005 - 1000 = 5 seconds
					tags: []string{"kube_namespace:default", "pod_name:my-pod", "uid:abc123"},
				},
			},
		},
		{
			name: "Multiple pods",
			scheduledMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "ns1",
						"pod":       "pod1",
					},
					Val: 100.0,
				},
				{
					Labels: map[string]string{
						"namespace": "ns2",
						"pod":       "pod2",
					},
					Val: 200.0,
				},
			},
			readyMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "ns1",
						"pod":       "pod1",
					},
					Val: 110.0,
				},
				{
					Labels: map[string]string{
						"namespace": "ns2",
						"pod":       "pod2",
					},
					Val: 230.0,
				},
			},
			expected: []metricsExpected{
				{
					name: "kubernetes_state.pod.time_to_ready",
					val:  10.0,
					tags: []string{"kube_namespace:ns1", "pod_name:pod1"},
				},
				{
					name: "kubernetes_state.pod.time_to_ready",
					val:  30.0,
					tags: []string{"kube_namespace:ns2", "pod_name:pod2"},
				},
			},
		},
		{
			name: "Pod with only scheduled time (no ready metric)",
			scheduledMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "pending-pod",
					},
					Val: 1000.0,
				},
			},
			readyMetrics: []ksmstore.DDMetric{},
			expected:     []metricsExpected{}, // No metric emitted
		},
		{
			name:             "Pod with only ready time (no scheduled metric)",
			scheduledMetrics: []ksmstore.DDMetric{},
			readyMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "ready-pod",
					},
					Val: 1005.0,
				},
			},
			expected: []metricsExpected{}, // No metric emitted (scheduledTime == 0)
		},
		{
			name: "Ready time locked - second ready update ignored",
			scheduledMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "my-pod",
					},
					Val: 1000.0,
				},
			},
			readyMetrics: []ksmstore.DDMetric{
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "my-pod",
					},
					Val: 1005.0, // First ready time
				},
				{
					Labels: map[string]string{
						"namespace": "default",
						"pod":       "my-pod",
					},
					Val: 1050.0, // Second ready time (after container restart) - should be ignored
				},
			},
			expected: []metricsExpected{
				{
					name: "kubernetes_state.pod.time_to_ready",
					val:  5.0, // Should still be 5, not 50
					tags: []string{"kube_namespace:default", "pod_name:my-pod"},
				},
			},
		},
	}

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ksmCheck := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{
		LabelsMapper: defaultLabelsMapper(),
	}, fakeTagger, nil)

	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()

		t.Run(tt.name, func(t *testing.T) {
			correlator := newPodTimeToReadyCorrelator()
			aggScheduled := &podScheduledTimeAggregator{correlator: correlator}
			aggReady := &podReadyTimeAggregator{correlator: correlator}

			// Accumulate scheduled metrics
			for _, metric := range tt.scheduledMetrics {
				aggScheduled.accumulate(metric)
			}
			// Accumulate ready metrics
			for _, metric := range tt.readyMetrics {
				aggReady.accumulate(metric)
			}

			// Flush both aggregators
			aggScheduled.flush(s, ksmCheck, newLabelJoiner(ksmCheck.instance.labelJoins))
			aggReady.flush(s, ksmCheck, newLabelJoiner(ksmCheck.instance.labelJoins))

			s.AssertNumberOfCalls(t, "Gauge", len(tt.expected))
			for _, expected := range tt.expected {
				s.AssertMetric(t, "Gauge", expected.name, expected.val, expected.hostname, expected.tags)
			}
		})
	}
}
