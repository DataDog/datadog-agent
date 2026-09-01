// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
	// Sorted, like the label pairs a real Gather returns, so the resulting tag order is stable.
	for _, name := range slices.Sorted(maps.Keys(labels)) {
		metric.Label = append(metric.Label, &dto.LabelPair{Name: stringPtr(name), Value: stringPtr(labels[name])})
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

func histogramMetricFamily(name string, count uint64, sum float64) *dto.MetricFamily {
	metricType := dto.MetricType_HISTOGRAM
	return &dto.MetricFamily{
		Name: stringPtr(name),
		Type: &metricType,
		Metric: []*dto.Metric{{
			Histogram: &dto.Histogram{
				SampleCount: &count,
				SampleSum:   &sum,
				Bucket: []*dto.Bucket{
					{UpperBound: float64Ptr(1), CumulativeCount: &count},
					{UpperBound: float64Ptr(10), CumulativeCount: &count},
				},
			},
		}},
	}
}

func typedMetricFamily(name string, metricType dto.MetricType) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: stringPtr(name),
		Type: &metricType,
		Metric: []*dto.Metric{{
			Untyped: &dto.Untyped{Value: float64Ptr(3)},
			Summary: &dto.Summary{SampleSum: float64Ptr(3)},
		}},
	}
}

// fakeTelemetry serves canned metric families for each of the two telemetry registries.
type fakeTelemetry struct {
	telemetry.Component

	defaultMfs []*dto.MetricFamily
	regularMfs []*dto.MetricFamily
}

func (f *fakeTelemetry) Gather(defaultGather bool) ([]*dto.MetricFamily, error) {
	if defaultGather {
		return f.defaultMfs, nil
	}
	return f.regularMfs, nil
}

// newTestCheck builds a configured check backed by the given registries, plus a mock sender.
func newTestCheck(t *testing.T, instance string, defaultMfs, regularMfs []*dto.MetricFamily) (*checkImpl, *mocksender.MockSender) {
	t.Helper()

	sm := mocksender.CreateDefaultDemultiplexer(t)
	c := &checkImpl{
		CheckBase: corechecks.NewCheckBase(CheckName),
		telemetry: &fakeTelemetry{defaultMfs: defaultMfs, regularMfs: regularMfs},
	}
	require.NoError(t, c.Configure(sm, integration.FakeConfigHash, integration.Data(instance), nil, "test", "provider"))

	return c, mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
}

// expectRunScaffolding registers the calls every Run() makes regardless of what it reports.
func expectRunScaffolding(s *mocksender.MockSender) {
	s.On("SetNoIndex", mock.AnythingOfType("bool")).Return()
	s.On("Commit").Return().Times(1)
}

// callIndex returns the position in the recorded call order of the first call to method whose
// leading arguments match argPrefix, or -1 if there is none.
func callIndex(s *mocksender.MockSender, method string, argPrefix ...interface{}) int {
	for i, call := range s.Calls {
		if call.Method != method || len(call.Arguments) < len(argPrefix) {
			continue
		}

		match := true
		for j, want := range argPrefix {
			if !reflect.DeepEqual(call.Arguments[j], want) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
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

// defaultRegistryFixture is a stand-in for what the default registry holds: a couple of plain
// families plus the merge metrics that are folded together with the regular registry.
func defaultRegistryFixture() []*dto.MetricFamily {
	return []*dto.MetricFamily{
		gaugeMetricFamily(
			"test__gauge",
			gaugeMetric(map[string]string{"foo": "bar"}, 1),
			gaugeMetric(map[string]string{"foo": "baz"}, 2),
		),
		counterMetricFamily("test__counter", 4),
		gaugeMetricFamily(
			pointSentMetric,
			gaugeMetric(map[string]string{domainLabel: "https://api.datadoghq.com"}, 10),
		),
	}
}

// regularRegistryFixture mixes an allowlisted family, a family that is only reported in advanced
// mode, and a remote-agent merge metric.
func regularRegistryFixture() []*dto.MetricFamily {
	return []*dto.MetricFamily{
		counterMetricFamily("logs__decoded", 7),
		gaugeMetricFamily(
			"scheduler__queue_size",
			gaugeMetric(map[string]string{"interval": "15", "shadow": "false"}, 3),
		),
		counterMetricFamily("some__internal_only_metric", 99),
		gaugeMetricFamily(
			pointSentMetric,
			gaugeMetric(map[string]string{
				domainLabel:  "https://api.datadoghq.com",
				emitterLabel: "agent-data-plane",
			}, 12),
		),
	}
}

func TestRunWithoutInternalTelemetry(t *testing.T) {
	c, s := newTestCheck(t, "", defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)

	// Only the default registry is reported, plus the merged point.sent covering both registries.
	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 22.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)

	require.NoError(t, c.Run())

	// Any regular-registry metric would be an unexpected call, which the mock fails on, but assert
	// the intent explicitly so a future SetupAcceptAll cannot silently weaken this.
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.AssertExpectations(t)
}

func TestRunWithInternalTelemetryEnabled(t *testing.T) {
	c, s := newTestCheck(t, "internal_telemetry:\n  enabled: true\n", defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)

	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 22.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	// Allowlisted regular-registry families.
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", 7.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.scheduler.queue_size", 3.0, "", []string{"interval:15", "shadow:false"}).Return().Times(1)

	require.NoError(t, c.Run())

	// Not on the allowlist, so it stays internal until advanced mode is turned on.
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", "datadog.agent.some.internal_only_metric", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.AssertExpectations(t)
}

func TestRunWithInternalTelemetryAdvanced(t *testing.T) {
	c, s := newTestCheck(t, "internal_telemetry:\n  advanced: true\n", defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)

	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 22.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", 7.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.scheduler.queue_size", 3.0, "", []string{"interval:15", "shadow:false"}).Return().Times(1)
	// Advanced mode reports everything, including families that are not on the allowlist.
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.some.internal_only_metric", 99.0, "", []string{}, true).Return().Times(1)

	require.NoError(t, c.Run())

	// point.sent lives in both registries; the merge path owns it, so advanced mode must not
	// report the regular-registry copy a second time.
	s.AssertNumberOfCalls(t, "Gauge", 4)
	s.AssertExpectations(t)
}

func TestRunReportsInternalTelemetryAsIndexed(t *testing.T) {
	c, s := newTestCheck(t, "internal_telemetry:\n  enabled: true\n", defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)

	s.On("Gauge", mock.AnythingOfType("string"), mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return()
	s.On("MonotonicCountWithFlushFirstValue", mock.AnythingOfType("string"), mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"), true).Return()

	require.NoError(t, c.Run())

	// Default-registry metrics stay no-index; internal telemetry is reported after the sender is
	// switched back to indexing, matching what the go_expvar instance produced.
	noIndexOn := callIndex(s, "SetNoIndex", true)
	noIndexOff := callIndex(s, "SetNoIndex", false)
	defaultMetric := callIndex(s, "Gauge", "datadog.agent.test.gauge")
	internalMetric := callIndex(s, "MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded")

	require.NotEqual(t, -1, noIndexOn)
	require.NotEqual(t, -1, noIndexOff)
	require.NotEqual(t, -1, defaultMetric)
	require.NotEqual(t, -1, internalMetric)
	require.Less(t, noIndexOn, defaultMetric, "default registry metrics must be reported while no-index is on")
	require.Less(t, defaultMetric, noIndexOff, "no-index must only be turned off after the default registry is reported")
	require.Less(t, noIndexOff, internalMetric, "internal telemetry must be reported as indexed")
}

func TestSendMetricFamilyHistogramDropsBuckets(t *testing.T) {
	c, s := newTestCheck(t, "", nil, nil)

	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.histogram.sum", 12.5, "", []string{}, true).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.histogram.count", 3.0, "", []string{}, true).Return().Times(1)

	c.sendMetricFamily(histogramMetricFamily("test__histogram", 3, 12.5), s)

	s.AssertNumberOfCalls(t, "MonotonicCountWithFlushFirstValue", 2)
	s.AssertExpectations(t)
}

func TestSendMetricFamilySkipsUnsupportedTypes(t *testing.T) {
	c, s := newTestCheck(t, "", nil, nil)

	for _, metricType := range []dto.MetricType{dto.MetricType_SUMMARY, dto.MetricType_UNTYPED} {
		c.sendMetricFamily(typedMetricFamily("test__unsupported", metricType), s)
	}

	s.AssertNumberOfCalls(t, "Gauge", 0)
	s.AssertNumberOfCalls(t, "MonotonicCountWithFlushFirstValue", 0)
}

func TestParseInstanceConfig(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		expected internalTelemetryConfig
	}{
		{
			name:     "empty instance",
			instance: "",
			expected: internalTelemetryConfig{},
		},
		{
			name:     "null instance",
			instance: "null",
			expected: internalTelemetryConfig{},
		},
		{
			name:     "unrelated keys only",
			instance: "min_collection_interval: 30\n",
			expected: internalTelemetryConfig{},
		},
		{
			name:     "enabled",
			instance: "internal_telemetry:\n  enabled: true\n",
			expected: internalTelemetryConfig{Enabled: true},
		},
		{
			name:     "advanced implies enabled",
			instance: "internal_telemetry:\n  advanced: true\n",
			expected: internalTelemetryConfig{Enabled: true, Advanced: true},
		},
		{
			name:     "explicitly disabled",
			instance: "internal_telemetry:\n  enabled: false\n",
			expected: internalTelemetryConfig{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseInstanceConfig(integration.Data(test.instance))
			require.NoError(t, err)
			require.Equal(t, test.expected, cfg.InternalTelemetry)
		})
	}
}

func TestParseInstanceConfigRejectsMalformedYAML(t *testing.T) {
	_, err := parseInstanceConfig(integration.Data("internal_telemetry: [oops\n"))
	require.Error(t, err)
}
