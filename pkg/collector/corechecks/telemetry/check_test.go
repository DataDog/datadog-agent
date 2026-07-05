// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const domainLabel = "domain"

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func gaugeMetric(labels map[string]string, value float64) *dto.Metric {
	metric := &dto.Metric{Gauge: &dto.Gauge{Value: float64Ptr(value)}}
	for name, value := range labels {
		metric.Label = append(metric.Label, &dto.LabelPair{Name: stringPtr(name), Value: stringPtr(value)})
	}
	return metric
}

func gaugeMetricFamily(name string, metrics ...*dto.Metric) *dto.MetricFamily {
	metricType := dto.MetricType_GAUGE
	return &dto.MetricFamily{
		Name:   stringPtr(name),
		Type:   &metricType,
		Metric: metrics,
	}
}

func counterMetricFamily(name string, value float64) *dto.MetricFamily {
	metricType := dto.MetricType_COUNTER
	return &dto.MetricFamily{
		Name: stringPtr(name),
		Type: &metricType,
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: float64Ptr(value)},
		}},
	}
}

func TestCollectAndMergeRegularRegistryMetrics(t *testing.T) {
	defaultMfs := []*dto.MetricFamily{
		gaugeMetricFamily(
			"point__sent",
			gaugeMetric(map[string]string{domainLabel: "https://api.datadoghq.com"}, 10),
			gaugeMetric(map[string]string{}, 1),
		),
		gaugeMetricFamily(
			"point__dropped",
			gaugeMetric(map[string]string{domainLabel: "https://api.datadoghq.com"}, 2),
		),
	}
	remoteMfs := []*dto.MetricFamily{
		gaugeMetricFamily(
			"point__sent",
			gaugeMetric(map[string]string{
				domainLabel:  "https://api.datadoghq.com",
				emitterLabel: "agent-data-plane",
			}, 12),
			gaugeMetric(map[string]string{
				domainLabel:  "https://api.datadoghq.eu",
				emitterLabel: "other-remote-agent",
			}, 5),
			gaugeMetric(map[string]string{domainLabel: "https://api.datadoghq.com"}, 100),
		),
		gaugeMetricFamily(
			"point__dropped",
			gaugeMetric(map[string]string{
				domainLabel:  "https://api.datadoghq.com",
				emitterLabel: "agent-data-plane",
			}, 3),
		),
	}

	labelsByMetric := discoverMergeLabels(defaultMfs, remoteMfs)
	values := collectMergeMetrics(defaultMfs, false, labelsByMetric)
	values.merge(collectMergeMetrics(remoteMfs, true, labelsByMetric))

	require.Equal(t, []string{domainLabel}, labelsByMetric[pointSentMetric])
	require.Equal(t, []string{domainLabel}, labelsByMetric[pointDroppedMetric])

	sentDefaultDomain := values[pointSentMetric][mergeKey([]string{"domain:https://api.datadoghq.com"})]
	require.Equal(t, mergeMetricSample{tags: []string{"domain:https://api.datadoghq.com"}, value: 22}, sentDefaultDomain)

	sentEmptyDomain := values[pointSentMetric][mergeKey([]string{"domain:"})]
	require.Equal(t, mergeMetricSample{tags: []string{"domain:"}, value: 1}, sentEmptyDomain)

	sentRemoteOnlyDomain := values[pointSentMetric][mergeKey([]string{"domain:https://api.datadoghq.eu"})]
	require.Equal(t, mergeMetricSample{tags: []string{"domain:https://api.datadoghq.eu"}, value: 5}, sentRemoteOnlyDomain)

	droppedDefaultDomain := values[pointDroppedMetric][mergeKey([]string{"domain:https://api.datadoghq.com"})]
	require.Equal(t, mergeMetricSample{tags: []string{"domain:https://api.datadoghq.com"}, value: 5}, droppedDefaultDomain)
}

func TestCollectMergeMetricsSkipsNonGaugeMetrics(t *testing.T) {
	mfs := []*dto.MetricFamily{counterMetricFamily(pointSentMetric, 12)}

	values := collectMergeMetrics(mfs, false, map[string][]string{pointSentMetric: {}})

	require.Empty(t, values)
}

func TestDiscoverMergeLabelsFallsBackToRegularRegistry(t *testing.T) {
	defaultMfs := []*dto.MetricFamily{}
	regularMfs := []*dto.MetricFamily{
		gaugeMetricFamily(
			pointSentMetric,
			gaugeMetric(map[string]string{
				domainLabel:  "https://api.datadoghq.com",
				emitterLabel: "agent-data-plane",
			}, 12),
		),
	}

	labelsByMetric := discoverMergeLabels(defaultMfs, regularMfs)
	values := collectMergeMetrics(regularMfs, true, labelsByMetric)

	require.Equal(t, []string{domainLabel}, labelsByMetric[pointSentMetric])
	require.Equal(t, mergeMetricSample{
		tags:  []string{"domain:https://api.datadoghq.com"},
		value: 12,
	}, values[pointSentMetric][mergeKey([]string{"domain:https://api.datadoghq.com"})])
}

func TestSendMergedMetrics(t *testing.T) {
	sm := mocksender.CreateDefaultDemultiplexer(t)
	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName)}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider")

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("Gauge", "datadog.agent.point.sent", 22.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 1.0, "", []string{"domain:"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.dropped", 5.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)

	values := newMergeMetricValues()
	values.add(pointSentMetric, []string{"domain:"}, 1)
	values.add(pointSentMetric, []string{"domain:https://api.datadoghq.com"}, 22)
	values.add(pointDroppedMetric, []string{"domain:https://api.datadoghq.com"}, 5)

	c.sendMergedMetrics(values, s)

	s.AssertExpectations(t)
}

func TestCheck(t *testing.T) {
	reg := prometheus.NewRegistry()

	func() {
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Subsystem: "test", Name: "_gauge"}, []string{"foo"})
		gauge.WithLabelValues("bar").Set(1.0)
		gauge.WithLabelValues("baz").Set(2.0)
		reg.MustRegister(gauge)

		count := prometheus.NewCounter(prometheus.CounterOpts{Subsystem: "test", Name: "_counter"})
		count.Add(4.0)
		reg.MustRegister(count)
	}()

	sm := mocksender.CreateDefaultDemultiplexer(t)

	c := &checkImpl{CheckBase: corechecks.NewCheckBase(CheckName)}
	c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider")

	s := mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Commit").Return().Times(1)

	mfs, err := reg.Gather()
	require.Nil(t, err)

	c.handleMetricFamilies(mfs, s)
	s.AssertExpectations(t)
}
