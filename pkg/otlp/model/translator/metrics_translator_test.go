// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package translator

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/source"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/quantile/summary"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func TestIsCumulativeMonotonic(t *testing.T) {
	// Some of these examples are from the hostmetrics receiver
	// and reflect the semantic meaning of the metrics there.
	//
	// If the receiver changes these examples should be added here too

	{ // Sum: Cumulative but not monotonic
		metric := pmetric.NewMetric()
		metric.SetName("system.filesystem.usage")
		metric.SetDescription("Filesystem bytes used.")
		metric.SetUnit("bytes")
		metric.SetEmptySum()
		sum := metric.Sum()
		sum.SetIsMonotonic(false)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

		assert.False(t, isCumulativeMonotonic(metric))
	}

	{ // Sum: Cumulative and monotonic
		metric := pmetric.NewMetric()
		metric.SetName("system.network.packets")
		metric.SetDescription("The number of packets transferred.")
		metric.SetUnit("1")
		metric.SetEmptySum()
		sum := metric.Sum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

		assert.True(t, isCumulativeMonotonic(metric))
	}

	{ // DoubleSumL Cumulative and monotonic
		metric := pmetric.NewMetric()
		metric.SetName("metric.example")
		metric.SetEmptySum()
		sum := metric.Sum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

		assert.True(t, isCumulativeMonotonic(metric))
	}

	{ // Not IntSum
		metric := pmetric.NewMetric()
		metric.SetName("system.cpu.load_average.1m")
		metric.SetDescription("Average CPU Load over 1 minute.")
		metric.SetUnit("1")
		metric.SetEmptyGauge()

		assert.False(t, isCumulativeMonotonic(metric))
	}
}

var _ source.Provider = (*testProvider)(nil)

type testProvider string

func (t testProvider) Source(context.Context) (source.Source, error) {
	return source.Source{
		Kind:       source.HostnameKind,
		Identifier: string(t),
	}, nil
}

func newTranslator(t *testing.T, logger *zap.Logger, opts ...Option) *Translator {
	options := append([]Option{
		WithFallbackSourceProvider(testProvider(fallbackHostname)),
		WithHistogramMode(HistogramModeDistributions),
		WithNumberMode(NumberModeCumulativeToDelta),
	}, opts...)

	tr, err := New(
		logger,
		options...,
	)

	require.NoError(t, err)
	return tr
}

type metric struct {
	name      string
	typ       MetricDataType
	timestamp uint64
	value     float64
	tags      []string
	host      string
}

type sketch struct {
	name      string
	basic     summary.Summary
	timestamp uint64
	tags      []string
	host      string
}

var _ TimeSeriesConsumer = (*mockTimeSeriesConsumer)(nil)

type mockTimeSeriesConsumer struct {
	metrics []metric
}

func (m *mockTimeSeriesConsumer) ConsumeTimeSeries(
	_ context.Context,
	dimensions *Dimensions,
	typ MetricDataType,
	ts uint64,
	val float64,
) {
	m.metrics = append(m.metrics,
		metric{
			name:      dimensions.Name(),
			typ:       typ,
			timestamp: ts,
			value:     val,
			tags:      dimensions.Tags(),
			host:      dimensions.Host(),
		},
	)
}

func newDims(name string) *Dimensions {
	return &Dimensions{name: name, tags: []string{}}
}

func newGauge(dims *Dimensions, ts uint64, val float64) metric {
	return metric{name: dims.name, typ: Gauge, timestamp: ts, value: val, tags: dims.tags}
}

func newCount(dims *Dimensions, ts uint64, val float64) metric {
	return newCountWithHost(dims, ts, val, "")
}

func newCountWithHost(dims *Dimensions, ts uint64, val float64, host string) metric {
	return metric{name: dims.name, typ: Count, timestamp: ts, value: val, tags: dims.tags, host: host}
}

func newSketch(dims *Dimensions, ts uint64, s summary.Summary) sketch {
	return sketch{name: dims.name, basic: s, timestamp: ts, tags: dims.tags}
}

func TestMapIntMetrics(t *testing.T) {
	ts := pcommon.NewTimestampFromTime(time.Now())
	slice := pmetric.NewNumberDataPointSlice()
	point := slice.AppendEmpty()
	point.SetIntValue(17)
	point.SetTimestamp(ts)
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	consumer := &mockTimeSeriesConsumer{}
	dims := newDims("int64.test")
	tr.mapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), 17)},
	)

	consumer = &mockTimeSeriesConsumer{}
	dims = newDims("int64.delta.test")
	tr.mapNumberMetrics(ctx, consumer, dims, Count, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newCount(dims, uint64(ts), 17)},
	)

	// With attribute tags
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "int64.test", tags: []string{"attribute_tag:attribute_value"}}
	tr.mapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), 17)},
	)
}

func TestMapDoubleMetrics(t *testing.T) {
	ts := pcommon.NewTimestampFromTime(time.Now())
	slice := pmetric.NewNumberDataPointSlice()
	point := slice.AppendEmpty()
	point.SetDoubleValue(math.Pi)
	point.SetTimestamp(ts)
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	consumer := &mockTimeSeriesConsumer{}
	dims := newDims("float64.test")
	tr.mapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), math.Pi)},
	)

	consumer = &mockTimeSeriesConsumer{}
	dims = newDims("float64.delta.test")
	tr.mapNumberMetrics(ctx, consumer, dims, Count, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newCount(dims, uint64(ts), math.Pi)},
	)

	// With attribute tags
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "float64.test", tags: []string{"attribute_tag:attribute_value"}}
	tr.mapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), math.Pi)},
	)
}

func seconds(i int) pcommon.Timestamp {
	return pcommon.NewTimestampFromTime(time.Unix(int64(i), 0))
}

var exampleDims = newDims("metric.example")

func TestMapIntMonotonicMetrics(t *testing.T) {
	// Create list of values
	deltas := []int64{1, 2, 200, 3, 7, 0}
	cumulative := make([]int64, len(deltas)+1)
	cumulative[0] = 0
	for i := 1; i < len(cumulative); i++ {
		cumulative[i] = cumulative[i-1] + deltas[i-1]
	}

	//Map to OpenTelemetry format
	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetIntValue(val)
		point.SetTimestamp(seconds(i))
	}

	// Map to Datadog format
	expected := make([]metric, len(deltas))
	for i, val := range deltas {
		expected[i] = newCount(exampleDims, uint64(seconds(i+1)), float64(val))
	}

	ctx := context.Background()
	consumer := &mockTimeSeriesConsumer{}
	tr := newTranslator(t, zap.NewNop())
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)

	assert.ElementsMatch(t, expected, consumer.metrics)
}

func TestMapIntMonotonicDifferentDimensions(t *testing.T) {
	slice := pmetric.NewNumberDataPointSlice()

	// No tags
	point := slice.AppendEmpty()
	point.SetTimestamp(seconds(0))

	point = slice.AppendEmpty()
	point.SetIntValue(20)
	point.SetTimestamp(seconds(1))

	// One tag: valA
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().PutStr("key1", "valA")

	point = slice.AppendEmpty()
	point.SetIntValue(30)
	point.SetTimestamp(seconds(1))
	point.Attributes().PutStr("key1", "valA")

	// same tag: valB
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().PutStr("key1", "valB")

	point = slice.AppendEmpty()
	point.SetIntValue(40)
	point.SetTimestamp(seconds(1))
	point.Attributes().PutStr("key1", "valB")

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(1)), 20),
			newCount(exampleDims.AddTags("key1:valA"), uint64(seconds(1)), 30),
			newCount(exampleDims.AddTags("key1:valB"), uint64(seconds(1)), 40),
		},
	)
}

func TestMapIntMonotonicWithReboot(t *testing.T) {
	values := []int64{0, 30, 0, 20}
	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(i))
		point.SetIntValue(val)
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(1)), 30),
			newCount(exampleDims, uint64(seconds(3)), 20),
		},
	)
}

func TestMapIntMonotonicReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(), consumer)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+1)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
		},
	)
}

func TestMapIntMonotonicReportDiffForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the diff (9) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(), consumer)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+1)), 9, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
		},
	)
}

func TestMapIntMonotonicOutOfOrder(t *testing.T) {
	stamps := []int{1, 0, 2, 3}
	values := []int64{0, 1, 2, 3}

	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(stamps[i]))
		point.SetIntValue(val)
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(2)), 2),
			newCount(exampleDims, uint64(seconds(3)), 1),
		},
	)
}

func TestMapDoubleMonotonicMetrics(t *testing.T) {
	deltas := []float64{1, 2, 200, 3, 7, 0}
	cumulative := make([]float64, len(deltas)+1)
	cumulative[0] = 0
	for i := 1; i < len(cumulative); i++ {
		cumulative[i] = cumulative[i-1] + deltas[i-1]
	}

	//Map to OpenTelemetry format
	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetDoubleValue(val)
		point.SetTimestamp(seconds(i))
	}

	// Map to Datadog format
	expected := make([]metric, len(deltas))
	for i, val := range deltas {
		expected[i] = newCount(exampleDims, uint64(seconds(i+1)), val)
	}

	ctx := context.Background()
	consumer := &mockTimeSeriesConsumer{}
	tr := newTranslator(t, zap.NewNop())
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)

	assert.ElementsMatch(t, expected, consumer.metrics)
}

func TestMapDoubleMonotonicDifferentDimensions(t *testing.T) {
	slice := pmetric.NewNumberDataPointSlice()

	// No tags
	point := slice.AppendEmpty()
	point.SetTimestamp(seconds(0))

	point = slice.AppendEmpty()
	point.SetDoubleValue(20)
	point.SetTimestamp(seconds(1))

	// One tag: valA
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().PutStr("key1", "valA")

	point = slice.AppendEmpty()
	point.SetDoubleValue(30)
	point.SetTimestamp(seconds(1))
	point.Attributes().PutStr("key1", "valA")

	// one tag: valB
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().PutStr("key1", "valB")

	point = slice.AppendEmpty()
	point.SetDoubleValue(40)
	point.SetTimestamp(seconds(1))
	point.Attributes().PutStr("key1", "valB")

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(1)), 20),
			newCount(exampleDims.AddTags("key1:valA"), uint64(seconds(1)), 30),
			newCount(exampleDims.AddTags("key1:valB"), uint64(seconds(1)), 40),
		},
	)
}

func TestMapDoubleMonotonicWithReboot(t *testing.T) {
	values := []float64{0, 30, 0, 20}
	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(2 * i))
		point.SetDoubleValue(val)
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(2)), 30),
			newCount(exampleDims, uint64(seconds(6)), 20),
		},
	)
}

func TestMapDoubleMonotonicReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(), consumer)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+1)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
		},
	)
}

func TestMapAPMStats(t *testing.T) {
	consumer := &mockFullConsumer{}
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	tr := newTranslator(t, logger)
	md := tr.StatsPayloadToMetrics(pb.StatsPayload{
		Stats: []pb.ClientStatsPayload{statsPayloads[0], statsPayloads[1]},
	})

	ctx := context.Background()
	tr.MapMetrics(ctx, md, consumer)
	require.Equal(t, consumer.apmstats, statsPayloads)
}

func TestMapDoubleMonotonicReportDiffForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the diff (9) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(), consumer)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+1)), 9, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
		},
	)
}

func TestMapDoubleMonotonicOutOfOrder(t *testing.T) {
	stamps := []int{1, 0, 2, 3}
	values := []float64{0, 1, 2, 3}

	slice := pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(stamps[i]))
		point.SetDoubleValue(val)
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(2)), 2),
			newCount(exampleDims, uint64(seconds(3)), 1),
		},
	)
}

var _ SketchConsumer = (*mockFullConsumer)(nil)

type mockFullConsumer struct {
	mockTimeSeriesConsumer
	sketches []sketch
	apmstats []pb.ClientStatsPayload
}

func (c *mockFullConsumer) ConsumeAPMStats(p pb.ClientStatsPayload) {
	c.apmstats = append(c.apmstats, p)
}

func (c *mockFullConsumer) ConsumeSketch(_ context.Context, dimensions *Dimensions, ts uint64, sk *quantile.Sketch) {
	c.sketches = append(c.sketches,
		sketch{
			name:      dimensions.Name(),
			basic:     sk.Basic,
			timestamp: ts,
			tags:      dimensions.Tags(),
			host:      dimensions.Host(),
		},
	)
}

func TestLegacyBucketsTags(t *testing.T) {
	// Test that passing the same tags slice doesn't reuse the slice.
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	tags := make([]string, 0, 10)

	pointOne := pmetric.NewHistogramDataPoint()
	pointOne.BucketCounts().FromRaw([]uint64{2, 18})
	pointOne.ExplicitBounds().FromRaw([]float64{0})
	pointOne.SetTimestamp(seconds(0))
	consumer := &mockTimeSeriesConsumer{}
	dims := &Dimensions{name: "test.histogram.one", tags: tags}
	tr.getLegacyBuckets(ctx, consumer, dims, pointOne, true)
	seriesOne := consumer.metrics

	pointTwo := pmetric.NewHistogramDataPoint()
	pointTwo.BucketCounts().FromRaw([]uint64{2, 18})
	pointTwo.ExplicitBounds().FromRaw([]float64{1})
	pointTwo.SetTimestamp(seconds(0))
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "test.histogram.two", tags: tags}
	tr.getLegacyBuckets(ctx, consumer, dims, pointTwo, true)
	seriesTwo := consumer.metrics

	assert.ElementsMatch(t, seriesOne[0].tags, []string{"lower_bound:-inf", "upper_bound:0"})
	assert.ElementsMatch(t, seriesTwo[0].tags, []string{"lower_bound:-inf", "upper_bound:1.0"})
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		f float64
		s string
	}{
		{f: 0, s: "0"},
		{f: 0.001, s: "0.001"},
		{f: 0.9, s: "0.9"},
		{f: 0.95, s: "0.95"},
		{f: 0.99, s: "0.99"},
		{f: 0.999, s: "0.999"},
		{f: 1, s: "1.0"},
		{f: 2, s: "2.0"},
		{f: math.Inf(1), s: "inf"},
		{f: math.Inf(-1), s: "-inf"},
		{f: math.NaN(), s: "nan"},
		{f: 1e-10, s: "1e-10"},
	}

	for _, test := range tests {
		assert.Equal(t, test.s, formatFloat(test.f))
	}
}

const (
	testHostname     = "res-hostname"
	fallbackHostname = "fallbackHostname"
)

func createTestIntCumulativeMonotonicMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(exampleDims.name)
	met.SetEmptySum()
	met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	met.Sum().SetIsMonotonic(true)

	values := []int64{10, 15, 20}
	dpsInt := met.Sum().DataPoints()
	dpsInt.EnsureCapacity(len(values))

	startTs := int(getProcessStartTime()) + 1
	for i, val := range values {
		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + i + 1))
		dpInt.SetIntValue(val)
	}
	return md
}

func createTestDoubleCumulativeMonotonicMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(exampleDims.name)
	met.SetEmptySum()
	met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	met.Sum().SetIsMonotonic(true)

	values := []float64{10, 15, 20}
	dpsInt := met.Sum().DataPoints()
	dpsInt.EnsureCapacity(len(values))

	startTs := int(getProcessStartTime()) + 1
	for i, val := range values {
		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + i + 1))
		dpInt.SetDoubleValue(val)
	}
	return md
}

func newCountWithHostname(name string, val float64, seconds uint64, tags []string) metric {
	dims := newDims(name)
	m := newCount(dims.AddTags(tags...), seconds*1e9, val)
	m.host = testHostname
	return m
}

func newSketchWithHostname(name string, summary summary.Summary, tags []string) sketch {
	dims := newDims(name)
	s := newSketch(dims.AddTags(tags...), 0, summary)
	s.host = testHostname
	return s
}

var statsPayloads = []pb.ClientStatsPayload{
	{
		Hostname:         "host",
		Env:              "prod",
		Version:          "v1.2",
		Lang:             "go",
		TracerVersion:    "v44",
		RuntimeID:        "123jkl",
		Sequence:         2,
		AgentAggregation: "blah",
		Service:          "mysql",
		ContainerID:      "abcdef123456",
		Tags:             []string{"a:b", "c:d"},
		Stats: []pb.ClientStatsBucket{
			{
				Start:    10,
				Duration: 1,
				Stats: []pb.ClientGroupedStats{
					{
						Service:        "kafka",
						Name:           "queue.add",
						Resource:       "append",
						HTTPStatusCode: 220,
						Type:           "queue",
						Hits:           15,
						Errors:         3,
						Duration:       143,
						OkSummary:      testSketchBytes(1, 4, 5),
						ErrorSummary:   testSketchBytes(2, 3, 9),
						TopLevelHits:   5,
					},
				},
			},
		},
	},
	{
		Hostname:         "host2",
		Env:              "prod2",
		Version:          "v1.22",
		Lang:             "go2",
		TracerVersion:    "v442",
		RuntimeID:        "123jkl2",
		Sequence:         22,
		AgentAggregation: "blah2",
		Service:          "mysql2",
		ContainerID:      "abcdef1234562",
		Tags:             []string{"a:b2", "c:d2"},
		Stats: []pb.ClientStatsBucket{
			{
				Start:    102,
				Duration: 12,
				Stats: []pb.ClientGroupedStats{
					{
						Service:        "kafka2",
						Name:           "queue.add2",
						Resource:       "append2",
						HTTPStatusCode: 2202,
						Type:           "queue2",
						Hits:           152,
						Errors:         32,
						Duration:       1432,
						OkSummary:      testSketchBytes(10, 11, 12),
						ErrorSummary:   testSketchBytes(14, 15, 16),
						TopLevelHits:   52,
					},
				},
			},
		},
	},
}
