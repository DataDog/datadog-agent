// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"maps"
	"math"
	"slices"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	domainLabel  = "domain"
	emitterLabel = "emitter"

	pointSentMetric = "point__sent"
)

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func uint64Ptr(value uint64) *uint64 {
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

func counterMetric(labels map[string]string, value float64) *dto.Metric {
	metric := &dto.Metric{Counter: &dto.Counter{Value: float64Ptr(value)}}
	// Sorted, like the label pairs a real Gather returns, so the resulting tag order is stable.
	for _, name := range slices.Sorted(maps.Keys(labels)) {
		metric.Label = append(metric.Label, &dto.LabelPair{Name: stringPtr(name), Value: stringPtr(labels[name])})
	}
	return metric
}

func counterMetricFamilyWith(name string, metrics ...*dto.Metric) *dto.MetricFamily {
	metricType := dto.MetricType_COUNTER
	return &dto.MetricFamily{
		Name:   stringPtr(name),
		Type:   &metricType,
		Metric: metrics,
	}
}

func counterMetricFamily(name string, value float64) *dto.MetricFamily {
	return counterMetricFamilyWith(name, counterMetric(nil, value))
}

// histogramBucket is an (upper bound, cumulative count) pair, matching Prometheus semantics where
// each bucket counts every observation at or below its bound.
type histogramBucket struct {
	upperBound float64
	cumulative uint64
}

func histogramMetricFamily(name string, sampleCount uint64, sampleSum float64, buckets ...histogramBucket) *dto.MetricFamily {
	metricType := dto.MetricType_HISTOGRAM
	histogram := &dto.Histogram{SampleCount: &sampleCount, SampleSum: &sampleSum}
	for _, bucket := range buckets {
		histogram.Bucket = append(histogram.Bucket, &dto.Bucket{
			UpperBound:      float64Ptr(bucket.upperBound),
			CumulativeCount: uint64Ptr(bucket.cumulative),
		})
	}
	return &dto.MetricFamily{
		Name:   stringPtr(name),
		Type:   &metricType,
		Metric: []*dto.Metric{{Histogram: histogram}},
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

// newTestCheck builds a configured check backed by the given registries and Agent config
// overrides, plus a mock sender.
func newTestCheck(t *testing.T, overrides map[string]interface{}, defaultMfs, regularMfs []*dto.MetricFamily) (*checkImpl, *mocksender.MockSender) {
	t.Helper()

	sm := mocksender.CreateDefaultDemultiplexer(t)
	c := &checkImpl{
		CheckBase: corechecks.NewCheckBase(CheckName),
		telemetry: &fakeTelemetry{defaultMfs: defaultMfs, regularMfs: regularMfs},
		config:    config.NewMockWithOverrides(t, overrides),
	}
	require.NoError(t, c.Configure(sm, integration.FakeConfigHash, nil, nil, "test", "provider"))

	return c, mocksender.NewMockSenderWithSenderManager(c.ID(), sm)
}

func internalTelemetryEnabled() map[string]interface{} {
	return map[string]interface{}{internalTelemetryEnabledSetting: true}
}

func internalTelemetryAdvanced() map[string]interface{} {
	return map[string]interface{}{internalTelemetryAdvancedSetting: true}
}

// expectRunScaffolding registers the calls every Run() makes regardless of what it reports.
func expectRunScaffolding(s *mocksender.MockSender) {
	s.On("Commit").Return().Times(1)
}

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

// regularRegistryFixture mixes two allowlisted families, a family only reported in advanced
// mode, and a remote agent copy of a metric the default registry also reports.
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

// expectDefaultRegistry registers the calls made for defaultRegistryFixture plus the overlapping
// remote agent copy of point.sent, which is always reported alongside it.
func expectDefaultRegistry(s *mocksender.MockSender) {
	s.On("Gauge", "datadog.agent.test.gauge", 1.0, "", []string{"foo:bar"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.test.gauge", 2.0, "", []string{"foo:baz"}).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return().Times(1)
	// The Core Agent's own series and the remote agent's are distinct series, reported unchanged
	// so the backend can sum them at query time and callers can still break them down by emitter.
	s.On("Gauge", "datadog.agent.point.sent", 10.0, "", []string{"domain:https://api.datadoghq.com"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 12.0, "", []string{"domain:https://api.datadoghq.com", "emitter:agent-data-plane"}).Return().Times(1)
}

func TestRunWithoutInternalTelemetry(t *testing.T) {
	c, s := newTestCheck(t, nil, defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)
	expectDefaultRegistry(s)

	require.NoError(t, c.Run())

	// Regular-registry families that do not overlap the default registry stay internal. An
	// unexpected call already fails the mock, but assert the intent so a future SetupAcceptAll
	// cannot silently weaken this.
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.AssertExpectations(t)
}

func TestRunWithInternalTelemetryEnabled(t *testing.T) {
	c, s := newTestCheck(t, internalTelemetryEnabled(), defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)
	expectDefaultRegistry(s)
	// Allowlisted regular-registry families.
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", 7.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.scheduler.queue_size", 3.0, "", []string{"interval:15", "shadow:false"}).Return().Times(1)

	require.NoError(t, c.Run())

	// Not on the allowlist, so it stays internal until advanced mode is turned on.
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", "datadog.agent.some.internal_only_metric", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// point.sent overlaps the default registry, so it is reported there and must not be reported
	// a second time by the internal telemetry pass.
	s.AssertNumberOfCalls(t, "Gauge", 5)
	s.AssertExpectations(t)
}

func TestRunWithInternalTelemetryAdvanced(t *testing.T) {
	c, s := newTestCheck(t, internalTelemetryAdvanced(), defaultRegistryFixture(), regularRegistryFixture())

	expectRunScaffolding(s)
	expectDefaultRegistry(s)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", 7.0, "", []string{}, true).Return().Times(1)
	s.On("Gauge", "datadog.agent.scheduler.queue_size", 3.0, "", []string{"interval:15", "shadow:false"}).Return().Times(1)
	// Advanced mode reports everything, including families that are not on the allowlist.
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.some.internal_only_metric", 99.0, "", []string{}, true).Return().Times(1)

	require.NoError(t, c.Run())

	s.AssertNumberOfCalls(t, "Gauge", 5)
	s.AssertExpectations(t)
}

func TestRunReportsOverlappingCounterSeries(t *testing.T) {
	// Counters overlap just like gauges: there is no summing, so no type restriction either.
	defaultMfs := []*dto.MetricFamily{counterMetricFamilyWith("shared__counter", counterMetric(nil, 5))}
	regularMfs := []*dto.MetricFamily{
		counterMetricFamilyWith("shared__counter", counterMetric(map[string]string{emitterLabel: "agent-data-plane"}, 7)),
	}

	c, s := newTestCheck(t, nil, defaultMfs, regularMfs)

	expectRunScaffolding(s)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.shared.counter", 5.0, "", []string{}, true).Return().Times(1)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.shared.counter", 7.0, "", []string{"emitter:agent-data-plane"}, true).Return().Times(1)

	require.NoError(t, c.Run())

	s.AssertExpectations(t)
}

func TestRunSkipsRegularSeriesIdenticalToADefaultSeries(t *testing.T) {
	// Same name and the exact same tags is the same series; reporting it twice would put two
	// points on it in one flush.
	defaultMfs := []*dto.MetricFamily{
		gaugeMetricFamily(pointSentMetric, gaugeMetric(map[string]string{domainLabel: "api"}, 10)),
	}
	regularMfs := []*dto.MetricFamily{
		gaugeMetricFamily(
			pointSentMetric,
			gaugeMetric(map[string]string{domainLabel: "api"}, 99),
			gaugeMetric(map[string]string{domainLabel: "api", emitterLabel: "agent-data-plane"}, 12),
		),
	}

	c, s := newTestCheck(t, nil, defaultMfs, regularMfs)

	expectRunScaffolding(s)
	s.On("Gauge", "datadog.agent.point.sent", 10.0, "", []string{"domain:api"}).Return().Times(1)
	s.On("Gauge", "datadog.agent.point.sent", 12.0, "", []string{"domain:api", "emitter:agent-data-plane"}).Return().Times(1)

	require.NoError(t, c.Run())

	// The duplicate is dropped; only the two distinct series are reported.
	s.AssertNumberOfCalls(t, "Gauge", 2)
	s.AssertExpectations(t)
}

func TestRunReportsRemoteOnlyMetricsThroughInternalTelemetry(t *testing.T) {
	// A remote agent family with no Core Agent counterpart is not part of the default batch, so
	// it is only reported once internal telemetry is enabled.
	regularMfs := []*dto.MetricFamily{
		counterMetricFamilyWith("logs__decoded", counterMetric(map[string]string{emitterLabel: "agent-data-plane"}, 3)),
	}

	c, s := newTestCheck(t, nil, nil, regularMfs)
	expectRunScaffolding(s)
	require.NoError(t, c.Run())
	s.AssertNumberOfCalls(t, "MonotonicCountWithFlushFirstValue", 0)

	c, s = newTestCheck(t, internalTelemetryEnabled(), nil, regularMfs)
	expectRunScaffolding(s)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.logs.decoded", 3.0, "", []string{"emitter:agent-data-plane"}, true).Return().Times(1)
	require.NoError(t, c.Run())
	s.AssertExpectations(t)
}

const histogramMetric = "datadog.agent.test.histogram"

// sendHistogram runs one metric family through the check and returns the mock sender, with the
// bucket call accepted unconditionally so each test can assert only what it cares about.
func sendHistogram(t *testing.T, mf *dto.MetricFamily) *mocksender.MockSender {
	t.Helper()

	c, s := newTestCheck(t, nil, nil, nil)
	s.On("HistogramBucket", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	c.sendMetricFamilies([]*dto.MetricFamily{mf}, nil, nil, s)

	return s
}

func TestSendHistogramDeCumulatesBuckets(t *testing.T) {
	// 100 observations at or below 1, 50 more in (1,10], and 10 above 10.
	s := sendHistogram(t, histogramMetricFamily("test__histogram", 160, 12.5,
		histogramBucket{upperBound: 1, cumulative: 100},
		histogramBucket{upperBound: 10, cumulative: 150},
	))

	// Bounds are ranges, not cumulative counts, and the sampler is told they grow monotonically so
	// it computes the per-interval delta itself.
	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 100, 0, 1, true, "", []string{}, false)
	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 50, 1, 10, true, "", []string{}, false)
	// client_golang leaves +Inf out of the dto, so it is synthesized from SampleCount.
	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 10, 10, math.Inf(1), true, "", []string{}, false)
	s.AssertNumberOfCalls(t, "HistogramBucket", 3)
}

func TestSendHistogramUsesAnExplicitInfBucket(t *testing.T) {
	// The text parser behind remote agent telemetry does include the +Inf bucket. It must be used
	// rather than double counted against SampleCount.
	s := sendHistogram(t, histogramMetricFamily("test__histogram", 160, 12.5,
		histogramBucket{upperBound: 1, cumulative: 100},
		histogramBucket{upperBound: math.Inf(1), cumulative: 160},
	))

	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 100, 0, 1, true, "", []string{}, false)
	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 60, 1, math.Inf(1), true, "", []string{}, false)
	s.AssertNumberOfCalls(t, "HistogramBucket", 2)
}

func TestSendHistogramSortsBucketsBeforeDeCumulating(t *testing.T) {
	// De-cumulating an unsorted family without sorting first would yield negative deltas.
	s := sendHistogram(t, histogramMetricFamily("test__histogram", 160, 12.5,
		histogramBucket{upperBound: 10, cumulative: 150},
		histogramBucket{upperBound: 1, cumulative: 100},
	))

	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 100, 0, 1, true, "", []string{}, false)
	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 50, 1, 10, true, "", []string{}, false)
	s.AssertNumberOfCalls(t, "HistogramBucket", 3)
}

func TestSendHistogramReportsNoSumOrCount(t *testing.T) {
	// The sketch carries the count exactly and the average, so the suffixed counts the check used
	// to emit instead of buckets are gone.
	s := sendHistogram(t, histogramMetricFamily("test__histogram", 160, 12.5,
		histogramBucket{upperBound: 1, cumulative: 100},
	))

	s.AssertNumberOfCalls(t, "MonotonicCountWithFlushFirstValue", 0)
	s.AssertNumberOfCalls(t, "Gauge", 0)
}

func TestSendHistogramWithoutBuckets(t *testing.T) {
	// A histogram with no explicit buckets is still worth its overflow bucket.
	s := sendHistogram(t, histogramMetricFamily("test__histogram", 7, 12.5))

	s.AssertHistogramBucket(t, "HistogramBucket", histogramMetric, 7, 0, math.Inf(1), true, "", []string{}, false)
	s.AssertNumberOfCalls(t, "HistogramBucket", 1)
}

func TestSendMetricFamiliesSkipsUnsupportedTypes(t *testing.T) {
	c, s := newTestCheck(t, nil, nil, nil)

	for _, metricType := range []dto.MetricType{dto.MetricType_SUMMARY, dto.MetricType_UNTYPED} {
		c.sendMetricFamilies([]*dto.MetricFamily{typedMetricFamily("test__unsupported", metricType)}, nil, nil, s)
	}

	s.AssertNumberOfCalls(t, "Gauge", 0)
	s.AssertNumberOfCalls(t, "MonotonicCountWithFlushFirstValue", 0)
}

func TestSendMetricFamiliesContinuesPastFilteredAndUnsupportedFamilies(t *testing.T) {
	// A family that a filter rejects, or that has an unsupported type, must not stop the families
	// after it from being reported.
	mfs := []*dto.MetricFamily{
		counterMetricFamily("test__filtered", 1),
		typedMetricFamily("test__unsupported", dto.MetricType_SUMMARY),
		counterMetricFamily("test__reported", 2),
	}

	c, s := newTestCheck(t, nil, nil, nil)
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.reported", 2.0, "", []string{}, true).Return().Times(1)

	c.sendMetricFamilies(mfs, func(mf *dto.MetricFamily) bool {
		return mf.GetName() != "test__filtered"
	}, nil, s)

	s.AssertExpectations(t)
}

// TestRunFollowsRuntimeConfigChanges pins the runtime-setting behavior: both settings are
// resolved on every run, so flipping them takes effect on the next check run without the check
// being reconfigured or recreated.
func TestRunFollowsRuntimeConfigChanges(t *testing.T) {
	c, s := newTestCheck(t, nil, defaultRegistryFixture(), regularRegistryFixture())

	// This test runs the check several times, so expectations are not bounded with Times().
	s.On("Commit").Return()
	s.On("Gauge", "datadog.agent.test.gauge", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return()
	s.On("MonotonicCountWithFlushFirstValue", "datadog.agent.test.counter", 4.0, "", []string{}, true).Return()
	s.On("Gauge", "datadog.agent.point.sent", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return()
	curated := "datadog.agent.logs.decoded"
	everything := "datadog.agent.some.internal_only_metric"
	s.On("MonotonicCountWithFlushFirstValue", curated, 7.0, "", []string{}, true).Return()
	s.On("Gauge", "datadog.agent.scheduler.queue_size", 3.0, "", []string{"interval:15", "shadow:false"}).Return()
	s.On("MonotonicCountWithFlushFirstValue", everything, 99.0, "", []string{}, true).Return()

	// Off: the default batch only.
	require.NoError(t, c.Run())
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", curated, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	// Turning it on mid-life takes effect on the next run, with no reconfiguration.
	c.config.Set(internalTelemetryEnabledSetting, true, model.SourceCLI)
	require.NoError(t, c.Run())
	s.AssertCalled(t, "MonotonicCountWithFlushFirstValue", curated, 7.0, "", []string{}, true)
	s.AssertNotCalled(t, "MonotonicCountWithFlushFirstValue", everything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	// Same for widening to advanced.
	c.config.Set(internalTelemetryAdvancedSetting, true, model.SourceCLI)
	require.NoError(t, c.Run())
	s.AssertCalled(t, "MonotonicCountWithFlushFirstValue", everything, 99.0, "", []string{}, true)

	// And for turning it back off.
	before := len(s.Calls)
	c.config.Set(internalTelemetryEnabledSetting, false, model.SourceCLI)
	c.config.Set(internalTelemetryAdvancedSetting, false, model.SourceCLI)
	require.NoError(t, c.Run())
	for _, call := range s.Calls[before:] {
		if len(call.Arguments) > 0 {
			assert.NotEqual(t, curated, call.Arguments[0], "internal telemetry must stop when the setting is turned off")
			assert.NotEqual(t, everything, call.Arguments[0], "advanced telemetry must stop when the setting is turned off")
		}
	}
}
