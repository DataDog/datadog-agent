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

package metrics

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	"github.com/DataDog/datadog-agent/pkg/util/quantile/summary"
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

func newTranslatorWithStatsChannel(t *testing.T, logger *zap.Logger, ch chan []byte) *defaultTranslator {
	options := []TranslatorOption{
		WithFallbackSourceProvider(testProvider(fallbackHostname)),
		WithHistogramMode(HistogramModeDistributions),
		WithNumberMode(NumberModeCumulativeToDelta),
		WithHistogramAggregations(),
		WithStatsOut(ch),
	}

	set := componenttest.NewNopTelemetrySettings()
	set.Logger = logger

	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	tr, err := NewDefaultTranslator(
		set,
		attributesTranslator,
		options...,
	)

	require.NoError(t, err)
	return tr.(*defaultTranslator)
}

func newTranslator(t *testing.T, logger *zap.Logger) *defaultTranslator {
	return newTranslatorWithStatsChannel(t, logger, nil)
}

type metric struct {
	name      string
	typ       DataType
	timestamp uint64
	interval  int64
	value     float64
	tags      []string
	host      string
}

type sketch struct {
	name      string
	basic     summary.Summary
	timestamp uint64
	interval  int64
	tags      []string
	host      string
}

var _ Consumer = (*mockTimeSeriesConsumer)(nil)

type mockTimeSeriesConsumer struct {
	metrics []metric
}

func (m *mockTimeSeriesConsumer) ConsumeTimeSeries(
	_ context.Context,
	dimensions *Dimensions,
	typ DataType,
	ts uint64,
	interval int64,
	val float64,
) {
	m.metrics = append(m.metrics,
		metric{
			name:      dimensions.Name(),
			typ:       typ,
			timestamp: ts,
			interval:  interval,
			value:     val,
			tags:      dimensions.Tags(),
			host:      dimensions.Host(),
		},
	)
}

func (m *mockTimeSeriesConsumer) ConsumeSketch(
	_ context.Context,
	_ *Dimensions,
	_ uint64,
	_ int64,
	_ *quantile.Sketch,
) {
	panic("unexpected method call to `ConsumeSketch` on mock consumer")
}

func (m *mockTimeSeriesConsumer) ConsumeExplicitBoundHistogram(
	_ context.Context,
	_ *Dimensions,
	_ pmetric.HistogramDataPointSlice,
) {
	panic("unexpected method call to `ConsumeExplicitBoundHistogram` on mock consumer")
}

func (m *mockTimeSeriesConsumer) ConsumeExponentialHistogram(
	_ context.Context,
	_ *Dimensions,
	_ pmetric.ExponentialHistogramDataPointSlice,

) {
	panic("unexpected method call to `ConsumeExponentialHistogram` on mock consumer")
}

func newDims(name string) *Dimensions {
	return &Dimensions{name: name, tags: []string{}}
}

func newGauge(dims *Dimensions, ts uint64, val float64) metric {
	return newGaugeWithHost(dims, ts, val, "")
}

func newGaugeWithHost(dims *Dimensions, ts uint64, val float64, host string) metric {
	return metric{name: dims.name, typ: Gauge, timestamp: ts, value: val, tags: dims.tags, host: host}
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
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), 17)},
	)

	consumer = &mockTimeSeriesConsumer{}
	dims = newDims("int64.delta.test")
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Count, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newCount(dims, uint64(ts), 17)},
	)

	// With attribute tags
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "int64.test", tags: []string{"attribute_tag:attribute_value"}}
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Gauge, slice)
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
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), math.Pi)},
	)

	consumer = &mockTimeSeriesConsumer{}
	dims = newDims("float64.delta.test")
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Count, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newCount(dims, uint64(ts), math.Pi)},
	)

	// With attribute tags
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "float64.test", tags: []string{"attribute_tag:attribute_value"}}
	tr.getMapper().MapNumberMetrics(ctx, consumer, dims, Gauge, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{newGauge(dims, uint64(ts), math.Pi)},
	)
}

func seconds(i int) pcommon.Timestamp {
	return pcommon.NewTimestampFromTime(time.Unix(int64(i), 0))
}

var exampleDims = newDims("metric.example")
var rateAsGaugeDims = newDims("kafka.net.bytes_out.rate")

func buildMonotonicIntPoints(deltas []int64) (slice pmetric.NumberDataPointSlice) {
	cumulative := make([]int64, len(deltas)+1)
	cumulative[0] = 0
	for i := 1; i < len(cumulative); i++ {
		cumulative[i] = cumulative[i-1] + deltas[i-1]
	}

	slice = pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetIntValue(val)
		point.SetTimestamp(seconds(i * 10))
	}

	return
}

func TestMapIntMonotonicMetrics(t *testing.T) {
	deltas := []int64{1, 2, 200, 3, 7, 0}

	t.Run("diff", func(t *testing.T) {
		slice := buildMonotonicIntPoints(deltas)

		expected := make([]metric, len(deltas))
		for i, val := range deltas {
			expected[i] = newCount(exampleDims, uint64(seconds((i+1)*10)), float64(val))
		}

		ctx := context.Background()
		consumer := &mockTimeSeriesConsumer{}
		tr := newTranslator(t, zap.NewNop())
		tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)

		assert.ElementsMatch(t, expected, consumer.metrics)
	})

	t.Run("rate", func(t *testing.T) {
		slice := buildMonotonicIntPoints(deltas)

		expected := make([]metric, len(deltas))
		for i, val := range deltas {
			// divide val by submission interval (10s)
			expected[i] = newGauge(rateAsGaugeDims, uint64(seconds((i+1)*10)), float64(val)/10.0)
		}

		ctx := context.Background()
		consumer := &mockTimeSeriesConsumer{}
		tr := newTranslator(t, zap.NewNop())
		tr.mapNumberMonotonicMetrics(ctx, consumer, rateAsGaugeDims, slice)

		assert.ElementsMatch(t, expected, consumer.metrics)
	})
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

func buildMonotonicIntRebootPoints() (slice pmetric.NumberDataPointSlice) {
	values := []int64{0, 30, 0, 20}
	slice = pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(i * 10))
		point.SetIntValue(val)
	}

	return
}

// This test checks that in the case of a reboot within a NumberDataPointSlice,
// we cache the value but we do NOT compute first value for the value at reset.
func TestMapIntMonotonicWithRebootWithinSlice(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		slice := buildMonotonicIntRebootPoints()
		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockTimeSeriesConsumer{}
		tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCount(exampleDims, uint64(seconds(10)), 30),
				newCount(exampleDims, uint64(seconds(30)), 20),
			},
		)
	})

	t.Run("rate", func(t *testing.T) {
		slice := buildMonotonicIntRebootPoints()
		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockTimeSeriesConsumer{}
		tr.mapNumberMonotonicMetrics(ctx, consumer, rateAsGaugeDims, slice)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGauge(rateAsGaugeDims, uint64(seconds(10)), 3),
				newGauge(rateAsGaugeDims, uint64(seconds(30)), 2),
			},
		)
	})
}

// This test checks that in the case of a reboot within a NumberDataPointSlice,
// we cache the value but we do NOT compute first value for the value at reset.
func TestMapIntMonotonicWithNoRecordedValueWithinSlice(t *testing.T) {

	buildMonotonicWithNoRecorded := func() (slice pmetric.NumberDataPointSlice) {
		values := []int64{0, 30, 0, 40}
		slice = pmetric.NewNumberDataPointSlice()
		slice.EnsureCapacity(len(values))

		for i, val := range values {
			point := slice.AppendEmpty()
			point.SetTimestamp(seconds(i * 10))
			point.SetIntValue(val)
		}

		var flags pmetric.DataPointFlags
		slice.At(2).SetFlags(flags.WithNoRecordedValue(true))
		return
	}

	slice := buildMonotonicWithNoRecorded()
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockTimeSeriesConsumer{}
	tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleDims, uint64(seconds(10)), 30),
			newCount(exampleDims, uint64(seconds(30)), 10),
		},
	)
}

// This test checks that in the case of a reboot at the first point in a NumberDataPointSlice:
// - diff: we cache the value AND compute first value
// - rate: we cache the value AND don't compute first value
func TestMapIntMonotonicWithRebootBeginningOfSlice(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// point is smaller than previous point. This is a reset. Cache this point and submit as new value.
		dpInt2.SetTimestamp(seconds(startTs + 3))
		dpInt2.SetIntValue(5)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(30)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 25, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("rate", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicRate(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// point is smaller than previous point. This is a reset. Cache this point and don't submit as new value.
		dpInt2.SetTimestamp(seconds(startTs + 3))
		dpInt2.SetIntValue(5)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(30)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 25, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})
}

// This test validates that a point (within a NumberDataPointSlice) with a timestamp older or equal
// to the timestamp of previous point received is dropped.
func TestMapIntMonotonicDropPointPointWithinSlice(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(exampleDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + 2))
		dpInt.SetIntValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("equal-rate", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(rateAsGaugeDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		// initial value, not computed for rate
		dpInt.SetTimestamp(seconds(startTs + 2))
		dpInt.SetIntValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(exampleDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + 3))
		dpInt.SetIntValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(25)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 10, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+5)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older-rate", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(rateAsGaugeDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		// initial value, not computed for rate
		dpInt.SetTimestamp(seconds(startTs + 3))
		dpInt.SetIntValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(25)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+5)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})
}

// Regression Test: This test validates that a point (the first point in a NumberDataPointSlice) with a timestamp older or equal
// to the timestamp of previous point received is dropped and not computed as a first val.
func TestMapIntMonotonicDropPointPointBeginningOfSlice(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("equal-rate", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+3)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+5)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older-rate", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+3)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+5)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

}

func TestMapIntMonotonicReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func TestMapIntMonotonicRateDontReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, rateAsGaugeDims), consumer, nil)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func TestMapIntMonotonicNotReportFirstValueIfStartTSMatchTS(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(true, exampleDims), consumer, nil)
	assert.Empty(t, consumer.metrics)
	assert.Empty(t, rmt.Languages)
}

func TestMapIntMonotonicRateNotReportFirstValueIfStartTSMatchTS(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(true, rateAsGaugeDims), consumer, nil)
	assert.Empty(t, consumer.metrics)
	assert.Empty(t, rmt.Languages)
}

func TestMapIntMonotonicReportDiffForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the diff (9) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 9, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func TestMapIntMonotonicReportRateForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the rate (10-1)/(startTs+2-startTs+1) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	rmt, _ := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, rateAsGaugeDims), consumer, nil)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+2)), 9, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func secondsAfterStart(i int) pcommon.Timestamp {
	return seconds(int(getProcessStartTime()) + 1 + i)
}

func buildIntPoints(startTs int, deltas []int64) pmetric.NumberDataPointSlice {
	slice := pmetric.NewNumberDataPointSlice()
	val := int64(0)
	for i, delta := range deltas {
		val += delta
		point := slice.AppendEmpty()
		point.SetStartTimestamp(secondsAfterStart(startTs))
		point.SetTimestamp(secondsAfterStart(i + 1))
		point.SetIntValue(val)
	}
	return slice
}

// Regression Test: Check initial point drop behavior based on the value of
// InitialCumulMonoValueMode and whether the metric series started before or after the Agent.
// Notably, we want to make sure that the "auto" value drops the initial point iff the series
// started before the metrics translator.
func TestInitialCumulMonoValueMode(t *testing.T) {
	ctx := context.Background()

	deltas := []int64{1, 2, 3}

	agentRestartInput := buildIntPoints(-20, deltas)
	appRestartInput := buildIntPoints(0, deltas)

	var keepOutput []metric
	for i, delta := range deltas {
		keepOutput = append(keepOutput, newCount(exampleDims, uint64(secondsAfterStart(i+1)), float64(delta)))
	}
	dropOutput := keepOutput[1:]

	type testCase struct {
		name   string
		mode   InitialCumulMonoValueMode
		input  pmetric.NumberDataPointSlice
		output []metric
	}
	testCases := []testCase{
		{"auto/agent-restart", InitialCumulMonoValueModeAuto, agentRestartInput, dropOutput},
		{"auto/app-restart", InitialCumulMonoValueModeAuto, appRestartInput, keepOutput},
		{"drop/agent-restart", InitialCumulMonoValueModeDrop, agentRestartInput, dropOutput},
		{"drop/app-restart", InitialCumulMonoValueModeDrop, appRestartInput, dropOutput},
		{"keep/agent-restart", InitialCumulMonoValueModeKeep, agentRestartInput, keepOutput},
		{"keep/app-restart", InitialCumulMonoValueModeKeep, appRestartInput, keepOutput},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTranslator(t, zap.NewNop())
			tr.cfg.InitialCumulMonoValueMode = tc.mode
			consumer := mockFullConsumer{}
			tr.mapNumberMonotonicMetrics(ctx, &consumer, exampleDims, tc.input)
			assert.Equal(t, tc.output, consumer.metrics)
		})
	}
}

func TestMapRuntimeMetricsHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	exampleDims = newDims("process.runtime.go.goroutines")
	mappedDims := newDims("runtime.go.num_goroutine")
	rmt, err := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
			newCountWithHost(mappedDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
			newCountWithHost(mappedDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(mappedDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"go"}, rmt.Languages)
}

func TestMapRuntimeMetricsHasMappingCollector(t *testing.T) {
	ctx := context.Background()
	tr := NewTestTranslator(t, WithRemapping())
	consumer := &mockFullConsumer{}
	exampleDims = newDims("process.runtime.go.goroutines")
	exampleOtelDims := newDims("otel.process.runtime.go.goroutines")
	mappedDims := newDims("runtime.go.num_goroutine")
	rmt, err := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(exampleOtelDims, uint64(seconds(startTs+2)), 10),
			newCount(exampleOtelDims, uint64(seconds(startTs+3)), 5),
			newCount(exampleOtelDims, uint64(seconds(startTs+4)), 5),
			newCount(mappedDims, uint64(seconds(startTs+2)), 10),
			newCount(mappedDims, uint64(seconds(startTs+3)), 5),
			newCount(mappedDims, uint64(seconds(startTs+4)), 5),
		},
	)
	assert.Equal(t, []string{"go"}, rmt.Languages)
}

func TestMapSystemMetricsRenamedWithOTelPrefix(t *testing.T) {
	ctx := context.Background()
	// WithOTelPrefix() is used to rename the system metrics, this overrides WithRemapping.
	tr := NewTestTranslator(t, WithOTelPrefix())
	consumer := &mockFullConsumer{}
	systemDims := newDims("system.cpu.utilization")
	processDims := newDims("process.runtime.go.goroutines")
	jvmDims := newDims("jvm.memory.used")
	for _, dims := range []*Dimensions{systemDims, processDims, jvmDims} {
		_, err := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, dims), consumer, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	startTs := int(getProcessStartTime()) + 1
	// Ensure datadog metrics are not created from system.cpu.utilization (ex: system.cpu.idle, system.cpu.system, etc)
	// Ensure otel.* prefix is added to system and process metrics
	// Ensure otel.* prefix is not added to jvm or go runtime metrics
	expectedSystemDims := newDims("otel.system.cpu.utilization")
	expectedProcessDims := newDims("otel.process.runtime.go.goroutines")
	expectedRuntimeDims := newDims("runtime.go.num_goroutine")
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(expectedSystemDims, uint64(seconds(startTs+2)), 10),
			newCount(expectedSystemDims, uint64(seconds(startTs+3)), 5),
			newCount(expectedSystemDims, uint64(seconds(startTs+4)), 5),
			newCount(expectedProcessDims, uint64(seconds(startTs+2)), 10),
			newCount(expectedProcessDims, uint64(seconds(startTs+3)), 5),
			newCount(expectedProcessDims, uint64(seconds(startTs+4)), 5),
			newCount(expectedRuntimeDims, uint64(seconds(startTs+2)), 10),
			newCount(expectedRuntimeDims, uint64(seconds(startTs+3)), 5),
			newCount(expectedRuntimeDims, uint64(seconds(startTs+4)), 5),
			newCount(jvmDims, uint64(seconds(startTs+2)), 10),
			newCount(jvmDims, uint64(seconds(startTs+3)), 5),
			newCount(jvmDims, uint64(seconds(startTs+4)), 5),
		},
	)
}

func TestMapSumRuntimeMetricWithAttributesHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "generation",
		values: []string{"gen0"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.dotnet.gc.collections.count", pmetric.MetricTypeSum, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(newDims("process.runtime.dotnet.gc.collections.count").AddTags("generation:gen0"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newCountWithHost(newDims("runtime.dotnet.gc.count.gen0"), uint64(seconds(startTs+1)), 10, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"dotnet"}, rmt.Languages)
}

func TestMapSumRuntimeMetricWithAttributesHasMappingCollector(t *testing.T) {
	ctx := context.Background()
	tr := NewTestTranslator(t, WithRemapping())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "generation",
		values: []string{"gen0"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.dotnet.gc.collections.count", pmetric.MetricTypeSum, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCount(newDims("otel.process.runtime.dotnet.gc.collections.count").AddTags("generation:gen0"), uint64(seconds(startTs+1)), 10),
			newCount(newDims("runtime.dotnet.gc.count.gen0"), uint64(seconds(startTs+1)), 10),
		},
	)
	assert.Equal(t, []string{"dotnet"}, rmt.Languages)
}

func TestMapGaugeRuntimeMetricWithAttributesHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "generation",
		values: []string{"gen1"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.dotnet.gc.heap.size", pmetric.MetricTypeGauge, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(newDims("process.runtime.dotnet.gc.heap.size").AddTags("generation:gen1"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("runtime.dotnet.gc.size.gen1"), uint64(seconds(startTs+1)), 10, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"dotnet"}, rmt.Languages)
}

func TestMapHistogramRuntimeMetricHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}

	rmt, err := tr.MapMetrics(ctx, createTestHistogramMetric("process.runtime.jvm.threads.count"), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(newDims("process.runtime.jvm.threads.count.count"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("process.runtime.jvm.threads.count.sum"), uint64(seconds(startTs+1)), 0, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.jvm.threads.count.min"), uint64(seconds(startTs+1)), -100, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.jvm.threads.count.max"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("jvm.thread_count.count"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("jvm.thread_count.sum"), uint64(seconds(startTs+1)), 0, fallbackHostname),
			newGaugeWithHost(newDims("jvm.thread_count.min"), uint64(seconds(startTs+1)), -100, fallbackHostname),
			newGaugeWithHost(newDims("jvm.thread_count.max"), uint64(seconds(startTs+1)), 100, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"jvm"}, rmt.Languages)
}

func TestMapHistogramRuntimeMetricWithAttributesHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "generation",
		values: []string{"gen1"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.dotnet.gc.heap.size", pmetric.MetricTypeHistogram, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(newDims("process.runtime.dotnet.gc.heap.size.count").AddTags("generation:gen1"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("process.runtime.dotnet.gc.heap.size.sum").AddTags("generation:gen1"), uint64(seconds(startTs+1)), 0, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.dotnet.gc.heap.size.min").AddTags("generation:gen1"), uint64(seconds(startTs+1)), -100, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.dotnet.gc.heap.size.max").AddTags("generation:gen1"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("runtime.dotnet.gc.size.gen1.count"), uint64(seconds(startTs+1)), 100, fallbackHostname),
			newCountWithHost(newDims("runtime.dotnet.gc.size.gen1.sum"), uint64(seconds(startTs+1)), 0, fallbackHostname),
			newGaugeWithHost(newDims("runtime.dotnet.gc.size.gen1.min"), uint64(seconds(startTs+1)), -100, fallbackHostname),
			newGaugeWithHost(newDims("runtime.dotnet.gc.size.gen1.max"), uint64(seconds(startTs+1)), 100, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"dotnet"}, rmt.Languages)
}

func TestMapRuntimeMetricWithTwoAttributesHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "pool",
		values: []string{"G1 Old Gen"},
	}, {
		key:    "type",
		values: []string{"heap"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.jvm.memory.usage", pmetric.MetricTypeGauge, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(newDims("process.runtime.jvm.memory.usage").AddTags("pool:G1 Old Gen", "type:heap"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("jvm.heap_memory").AddTags("pool:G1 Old Gen"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("jvm.gc.old_gen_size"), uint64(seconds(startTs+1)), 10, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"jvm"}, rmt.Languages)
}

func TestMapRuntimeMetricWithTwoAttributesMultipleDataPointsHasMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "pool",
		values: []string{"G1 Old Gen", "G1 Survivor Space", "G1 Eden Space"},
	}, {
		key:    "type",
		values: []string{"heap", "heap", "heap"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.jvm.memory.usage", pmetric.MetricTypeGauge, attributes, 3), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(newDims("process.runtime.jvm.memory.usage").AddTags("pool:G1 Old Gen", "type:heap"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.jvm.memory.usage").AddTags("pool:G1 Survivor Space", "type:heap"), uint64(seconds(startTs+2)), 20, fallbackHostname),
			newGaugeWithHost(newDims("process.runtime.jvm.memory.usage").AddTags("pool:G1 Eden Space", "type:heap"), uint64(seconds(startTs+3)), 30, fallbackHostname),
			newGaugeWithHost(newDims("jvm.heap_memory").AddTags("pool:G1 Old Gen"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("jvm.heap_memory").AddTags("pool:G1 Survivor Space"), uint64(seconds(startTs+2)), 20, fallbackHostname),
			newGaugeWithHost(newDims("jvm.heap_memory").AddTags("pool:G1 Eden Space"), uint64(seconds(startTs+3)), 30, fallbackHostname),
			newGaugeWithHost(newDims("jvm.gc.old_gen_size"), uint64(seconds(startTs+1)), 10, fallbackHostname),
			newGaugeWithHost(newDims("jvm.gc.survivor_size"), uint64(seconds(startTs+2)), 20, fallbackHostname),
			newGaugeWithHost(newDims("jvm.gc.eden_size"), uint64(seconds(startTs+3)), 30, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"jvm"}, rmt.Languages)
}

func TestMapRuntimeMetricsMultipleLanguageTags(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	exampleDims = newDims("process.runtime.go.goroutines")
	md1 := createTestIntCumulativeMonotonicMetrics(false, exampleDims)
	rmt, err := tr.MapMetrics(ctx, md1, consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{"go"}, rmt.Languages)

	exampleDims = newDims("process.runtime.go.lookups")
	md2 := createTestIntCumulativeMonotonicMetrics(false, exampleDims)
	md1.ResourceMetrics().MoveAndAppendTo(md2.ResourceMetrics())
	rmt, err = tr.MapMetrics(ctx, md2, consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{"go"}, rmt.Languages)

	exampleDims = newDims("process.runtime.dotnet.exceptions.count")
	md3 := createTestIntCumulativeMonotonicMetrics(false, exampleDims)
	md2.ResourceMetrics().MoveAndAppendTo(md3.ResourceMetrics())
	rmt, err = tr.MapMetrics(ctx, md3, consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []string{"go", "dotnet"}, rmt.Languages)

	exampleDims = newDims("process.runtime.jvm.classes.current_loaded")
	md4 := createTestIntCumulativeMonotonicMetrics(false, exampleDims)
	md3.ResourceMetrics().MoveAndAppendTo(md4.ResourceMetrics())
	rmt, err = tr.MapMetrics(ctx, md4, consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, []string{"go", "dotnet", "jvm"}, rmt.Languages)
}

func TestMapGaugeRuntimeMetricWithInvalidAttributes(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	attributes := []runtimeMetricAttribute{{
		key:    "type",
		values: []string{"heap2"},
	}}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("process.runtime.jvm.memory.usage", pmetric.MetricTypeGauge, attributes, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(newDims("process.runtime.jvm.memory.usage").AddTags("type:heap2"), uint64(seconds(startTs+1)), 10, fallbackHostname),
		},
	)
	assert.Equal(t, []string{"jvm"}, rmt.Languages)
}

func TestMapRuntimeMetricsNoMapping(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	exampleDims = newDims("runtime.go.mem.live_objects")
	rmt, err := tr.MapMetrics(ctx, createTestIntCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func TestWithRuntimeMetricMappings(t *testing.T) {
	tests := []struct {
		name         string
		mappedName   string
		withMappings bool
		expectedLang string
	}{
		{
			name:         "process.runtime.go.goroutines",
			withMappings: true,
			mappedName:   "runtime.go.num_goroutine",
			expectedLang: "go",
		},
		{
			name: "process.runtime.go.goroutines",
		},
		{
			name:         "process.runtime.dotnet.exceptions.count",
			withMappings: true,
			mappedName:   "runtime.dotnet.exceptions.count",
			expectedLang: "dotnet",
		},
		{
			name: "process.runtime.dotnet.exceptions.count",
		},
		{
			name:         "jvm.thread.count",
			withMappings: true,
			mappedName:   "jvm.thread_count",
			expectedLang: "jvm",
		},
		{
			name: "jvm.thread.count",
		},
		{
			name:         "process.runtime.jvm.threads.count",
			withMappings: true,
			mappedName:   "jvm.thread_count",
			expectedLang: "jvm",
		},
		{
			name: "process.runtime.jvm.threads.count",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%v", tt.name, tt.withMappings), func(t *testing.T) {
			var opts []TranslatorOption
			if !tt.withMappings {
				opts = append(opts, WithoutRuntimeMetricMappings())
			}
			tr := NewTestTranslator(t, opts...)
			consumer := &mockTimeSeriesConsumer{}
			metric := createTestMetricWithAttributes(tt.name, pmetric.MetricTypeGauge, nil, 1)

			rmt, err := tr.MapMetrics(t.Context(), metric, consumer, nil)
			require.NoError(t, err)

			if tt.withMappings {
				require.Len(t, consumer.metrics, 2)
				assert.Equal(t, tt.name, consumer.metrics[0].name)
				assert.Equal(t, tt.mappedName, consumer.metrics[1].name)
				assert.Equal(t, []string{tt.expectedLang}, rmt.Languages)
			} else {
				require.Len(t, consumer.metrics, 1)
				assert.Equal(t, tt.name, consumer.metrics[0].name)
				assert.Empty(t, rmt.Languages)
			}
		})
	}
}

func TestMapSystemMetrics(t *testing.T) {
	ctx := context.Background()
	tr := NewTestTranslator(t, WithRemapping())
	consumer := &mockFullConsumer{}
	rmt, err := tr.MapMetrics(ctx, createTestMetricWithAttributes("system.filesystem.utilization", pmetric.MetricTypeGauge, nil, 1), consumer, nil)
	if err != nil {
		t.Fatal(err)
	}
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(newDims("otel.system.filesystem.utilization"), uint64(seconds(startTs+1)), 10, ""),
			newGaugeWithHost(newDims("system.disk.in_use"), uint64(seconds(startTs+1)), 10, ""),
		},
	)
	assert.Empty(t, rmt.Languages)
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

func buildMonotonicDoublePoints(deltas []float64) (slice pmetric.NumberDataPointSlice) {
	cumulative := make([]float64, len(deltas)+1)
	cumulative[0] = 0
	for i := 1; i < len(cumulative); i++ {
		cumulative[i] = cumulative[i-1] + deltas[i-1]
	}

	slice = pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetDoubleValue(val)
		point.SetTimestamp(seconds(i * 10))
	}

	return
}

func TestMapDoubleMonotonicMetrics(t *testing.T) {
	deltas := []float64{1, 2, 200, 3, 7, 0}
	t.Run("diff", func(t *testing.T) {
		slice := buildMonotonicDoublePoints(deltas)

		expected := make([]metric, len(deltas))
		for i, val := range deltas {
			expected[i] = newCount(exampleDims, uint64(seconds(i+1)*10), val)
		}

		ctx := context.Background()
		consumer := &mockTimeSeriesConsumer{}
		tr := newTranslator(t, zap.NewNop())
		tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)

		assert.ElementsMatch(t, expected, consumer.metrics)
	})

	t.Run("rate", func(t *testing.T) {
		slice := buildMonotonicDoublePoints(deltas)

		expected := make([]metric, len(deltas))
		for i, val := range deltas {
			// divide val by submission interval (10s)
			expected[i] = newGauge(rateAsGaugeDims, uint64(seconds((i+1)*10)), val/10.0)
		}

		ctx := context.Background()
		consumer := &mockTimeSeriesConsumer{}
		tr := newTranslator(t, zap.NewNop())
		tr.mapNumberMonotonicMetrics(ctx, consumer, rateAsGaugeDims, slice)

		assert.ElementsMatch(t, expected, consumer.metrics)
	})

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

func buildMonotonicDoubleRebootPoints() (slice pmetric.NumberDataPointSlice) {
	values := []float64{0, 30, 0, 20}
	slice = pmetric.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(i * 10))
		point.SetDoubleValue(val)
	}

	return
}

// This test checks that in the case of a reboot within a NumberDataPointSlice,
// we cache the value but we do NOT compute first value for the value at reset.
func TestMapDoubleMonotonicWithRebootWithinSlice(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		slice := buildMonotonicDoubleRebootPoints()

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockTimeSeriesConsumer{}
		tr.mapNumberMonotonicMetrics(ctx, consumer, exampleDims, slice)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCount(exampleDims, uint64(seconds(10)), 30),
				newCount(exampleDims, uint64(seconds(30)), 20),
			},
		)
	})

	t.Run("rate", func(t *testing.T) {
		slice := buildMonotonicDoubleRebootPoints()

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockTimeSeriesConsumer{}
		tr.mapNumberMonotonicMetrics(ctx, consumer, rateAsGaugeDims, slice)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGauge(rateAsGaugeDims, uint64(seconds(10)), 3),
				newGauge(rateAsGaugeDims, uint64(seconds(30)), 2),
			},
		)
	})
}

// This test checks that in the case of a reboot at the first point in a NumberDataPointSlice:
// - diff: we cache the value AND compute first value
// - rate: we cache the value AND don't compute first value
func TestMapDoubleMonotonicWithRebootBeginningOfSlice(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10.0)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// point is smaller than previous point. This is a reset. Cache this point and submit as new value.
		dpInt2.SetTimestamp(seconds(startTs + 3))
		dpInt2.SetDoubleValue(5)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetDoubleValue(30)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 25, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("rate", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicRate(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10.0)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// point is smaller than previous point. This is a reset. Cache this point and don't submit as new value.
		dpInt2.SetTimestamp(seconds(startTs + 3))
		dpInt2.SetDoubleValue(5)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetDoubleValue(30)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 25, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

}

// This test validates that a point (within a NumberDataPointSlice) with a timestamp older or equal
// to the timestamp of previous point received is dropped.
func TestMapDoubleMonotonicDropPointPointWithinSlice(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(exampleDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + 2))
		dpInt.SetDoubleValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetDoubleValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetDoubleValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("equal-rate", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(rateAsGaugeDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		// initial value, not computed for rate
		dpInt.SetTimestamp(seconds(startTs + 2))
		dpInt.SetDoubleValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetDoubleValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetDoubleValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(exampleDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		dpInt.SetTimestamp(seconds(startTs + 3))
		dpInt.SetDoubleValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetDoubleValue(25)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetDoubleValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 10, fallbackHostname),
				newCountWithHost(exampleDims, uint64(seconds(startTs+5)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older-rate", func(t *testing.T) {
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(rateAsGaugeDims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		dpInt := dpsInt.AppendEmpty()
		dpInt.SetStartTimestamp(seconds(startTs))
		// initial value, not computed for rate
		dpInt.SetTimestamp(seconds(startTs + 3))
		dpInt.SetIntValue(10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored.
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetIntValue(25)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetIntValue(40)

		ctx := context.Background()
		tr := newTranslator(t, zap.NewNop())
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+5)), 15, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})
}

// Regression Test: This test validates that a point (the first point in a NumberDataPointSlice) with a timestamp older or equal
// to the timestamp of previous point received is dropped and not computed as a first val.
func TestMapDoubleMonotonicDropPointPointBeginningOfSlice(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+2)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// duplicate timestamp to dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetDoubleValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 4))
		dpInt3.SetDoubleValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})

	t.Run("older", func(t *testing.T) {
		tr := newTranslator(t, zap.NewNop())
		dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
		startTs := int(getProcessStartTime()) + 1
		md := pmetric.NewMetrics()
		met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		met.SetName(dims.name)
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt := met.Sum().DataPoints()
		dpsInt.EnsureCapacity(3)

		// dpInt1
		tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+3)), 10)

		dpInt2 := dpsInt.AppendEmpty()
		dpInt2.SetStartTimestamp(seconds(startTs))
		// lower timestamp than dpInt. This point should be ignored and not used as first val
		dpInt2.SetTimestamp(seconds(startTs + 2))
		dpInt2.SetDoubleValue(20)

		dpInt3 := dpsInt.AppendEmpty()
		dpInt3.SetStartTimestamp(seconds(startTs))
		dpInt3.SetTimestamp(seconds(startTs + 5))
		dpInt3.SetDoubleValue(40)

		ctx := context.Background()
		consumer := &mockFullConsumer{}

		rmt, _ := tr.MapMetrics(ctx, md, consumer, nil)
		assert.ElementsMatch(t,
			consumer.metrics,
			[]metric{
				newCountWithHost(exampleDims, uint64(seconds(startTs+5)), 30, fallbackHostname),
			},
		)
		assert.Empty(t, rmt.Languages)
	})
}

func TestMapDoubleMonotonicReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 10, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
}

func TestMapDoubleMonotonicRateDontReportFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	rmt, _ := tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(false, rateAsGaugeDims), consumer, nil)
	startTs := int(getProcessStartTime()) + 1
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
}

func TestMapDoubleMonotonicNotReportFirstValueIfStartTSMatchTS(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(true, exampleDims), consumer, nil)
	assert.Empty(t, consumer.metrics)
}

func TestMapAPMStatsWithBytes(t *testing.T) {
	consumer := &mockFullConsumer{}
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	ch := make(chan []byte, 10)

	options := []TranslatorOption{
		WithFallbackSourceProvider(testProvider(fallbackHostname)),
		WithHistogramMode(HistogramModeDistributions),
		WithNumberMode(NumberModeCumulativeToDelta),
		WithHistogramAggregations(),
		WithStatsOut(ch),
	}

	set := componenttest.NewNopTelemetrySettings()
	set.Logger = logger

	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	tr, err := NewTranslator(set, attributesTranslator, options...)
	require.NoError(t, err)

	want := &pb.StatsPayload{
		Stats: []*pb.ClientStatsPayload{statsPayloads[0], statsPayloads[1]},
	}
	md, err := tr.StatsToMetrics(want)
	assert.NoError(t, err)

	ctx := context.Background()
	tr.MapMetrics(ctx, md, consumer, nil)
	got := &pb.StatsPayload{}

	payload := <-ch
	err = proto.Unmarshal(payload, got)
	assert.NoError(t, err)
	assert.True(t, proto.Equal(want, got))
}

func TestMapDoubleMonotonicReportDiffForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: exampleDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the diff (9) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(false, exampleDims), consumer, nil)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newCountWithHost(exampleDims, uint64(seconds(startTs+2)), 9, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newCountWithHost(exampleDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
}

func TestMapDoubleMonotonicReportRateForFirstValue(t *testing.T) {
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	consumer := &mockFullConsumer{}
	dims := &Dimensions{name: rateAsGaugeDims.name, host: fallbackHostname}
	startTs := int(getProcessStartTime()) + 1
	// Add an entry to the cache about the timeseries, in this case we send the rate (10-1)/(startTs+2-startTs+1) rather than the first value (10).
	tr.prevPts.MonotonicDiff(dims, uint64(seconds(startTs)), uint64(seconds(startTs+1)), 1)
	rmt, _ := tr.MapMetrics(ctx, createTestDoubleCumulativeMonotonicMetrics(false, rateAsGaugeDims), consumer, nil)
	assert.ElementsMatch(t,
		consumer.metrics,
		[]metric{
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+2)), 9, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+3)), 5, fallbackHostname),
			newGaugeWithHost(rateAsGaugeDims, uint64(seconds(startTs+4)), 5, fallbackHostname),
		},
	)
	assert.Empty(t, rmt.Languages)
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

var _ Consumer = (*mockFullConsumer)(nil)

type mockFullConsumer struct {
	mockTimeSeriesConsumer
	sketches []sketch
}

func (c *mockFullConsumer) ConsumeSketch(_ context.Context, dimensions *Dimensions, ts uint64, interval int64, sk *quantile.Sketch) {
	c.sketches = append(c.sketches,
		sketch{
			name:      dimensions.Name(),
			basic:     sk.Basic,
			timestamp: ts,
			interval:  interval,
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
	mapper := tr.getMapper().(*defaultMapper)
	mapper.getLegacyBuckets(ctx, consumer, dims, pointOne, true)
	seriesOne := consumer.metrics

	pointTwo := pmetric.NewHistogramDataPoint()
	pointTwo.BucketCounts().FromRaw([]uint64{2, 18})
	pointTwo.ExplicitBounds().FromRaw([]float64{1})
	pointTwo.SetTimestamp(seconds(0))
	consumer = &mockTimeSeriesConsumer{}
	dims = &Dimensions{name: "test.histogram.two", tags: tags}
	mapper.getLegacyBuckets(ctx, consumer, dims, pointTwo, true)
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

func createTestIntCumulativeMonotonicMetrics(tsmatch bool, dims *Dimensions) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(dims.name)
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
		if tsmatch {
			dpInt.SetTimestamp(seconds(startTs))
		} else {
			dpInt.SetTimestamp(seconds(startTs + i + 2))
		}
		dpInt.SetIntValue(val)
	}
	return md
}

func createTestDoubleCumulativeMonotonicMetrics(tsmatch bool, dims *Dimensions) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(dims.name)
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
		if tsmatch {
			dpInt.SetTimestamp(seconds(startTs))
		} else {
			dpInt.SetTimestamp(seconds(startTs + i + 2))
		}
		dpInt.SetDoubleValue(val)
	}
	return md
}

func createTestHistogramMetric(metricName string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(metricName)
	var hpsCount pmetric.HistogramDataPointSlice
	met.SetEmptyHistogram()
	met.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	hpsCount = met.Histogram().DataPoints()
	hpsCount.EnsureCapacity(1)
	startTs := int(getProcessStartTime()) + 1
	hpCount := hpsCount.AppendEmpty()
	hpCount.SetStartTimestamp(seconds(startTs))
	hpCount.SetTimestamp(seconds(startTs + 1))
	hpCount.ExplicitBounds().FromRaw([]float64{})
	hpCount.BucketCounts().FromRaw([]uint64{100})
	hpCount.SetCount(100)
	hpCount.SetSum(0)
	hpCount.SetMin(-100)
	hpCount.SetMax(100)
	return md
}

func createTestMetricWithAttributes(metricName string, metricType pmetric.MetricType, attributes []runtimeMetricAttribute, dataPoints int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(metricName)
	var dpsInt pmetric.NumberDataPointSlice
	var hpsCount pmetric.HistogramDataPointSlice
	if metricType == pmetric.MetricTypeSum {
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt = met.Sum().DataPoints()
	} else if metricType == pmetric.MetricTypeGauge {
		met.SetEmptyGauge()
		dpsInt = met.Gauge().DataPoints()
	} else if metricType == pmetric.MetricTypeHistogram {
		met.SetEmptyHistogram()
		met.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		hpsCount = met.Histogram().DataPoints()
	}

	if metricType != pmetric.MetricTypeHistogram {
		dpsInt.EnsureCapacity(dataPoints)
		for i := 0; i < dataPoints; i++ {
			startTs := int(getProcessStartTime()) + 1
			dpInt := dpsInt.AppendEmpty()
			for _, attr := range attributes {
				dpInt.Attributes().PutStr(attr.key, attr.values[i])
			}
			dpInt.SetStartTimestamp(seconds(startTs))
			dpInt.SetTimestamp(seconds(startTs + 1 + i))
			dpInt.SetIntValue(int64(10 * (1 + i)))
		}
		return md
	}

	hpsCount.EnsureCapacity(dataPoints)
	for i := 0; i < dataPoints; i++ {
		startTs := int(getProcessStartTime()) + 1
		hpCount := hpsCount.AppendEmpty()
		for _, attr := range attributes {
			hpCount.Attributes().PutStr(attr.key, attr.values[i])
		}
		hpCount.SetStartTimestamp(seconds(startTs))
		hpCount.SetTimestamp(seconds(startTs + 1 + i))
		hpCount.ExplicitBounds().FromRaw([]float64{})
		hpCount.BucketCounts().FromRaw([]uint64{100})
		hpCount.SetCount(100)
		hpCount.SetSum(0)
		hpCount.SetMin(-100)
		hpCount.SetMax(100)
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

var statsPayloads = []*pb.ClientStatsPayload{
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
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    10,
				Duration: 1,
				Stats: []*pb.ClientGroupedStats{
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
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    102,
				Duration: 12,
				Stats: []*pb.ClientGroupedStats{
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

func TestInferInterval(t *testing.T) {
	tests := []struct {
		name        string
		startTs, ts uint64
		expected    int64
	}{
		{
			name:     "exact difference",
			startTs:  1e9,
			ts:       11e9,
			expected: 10,
		},
		{
			name:     "under within tolerance",
			startTs:  1e9,
			ts:       11e9 - 30e6,
			expected: 10,
		},
		{
			name:     "over within tolerance",
			startTs:  1e9,
			ts:       11e9 + 30e6,
			expected: 10,
		},
		{
			name:     "outside tolerance",
			startTs:  1e9,
			ts:       11e9 + 50e7,
			expected: 0,
		},
		{
			name:     "no starttimestamp",
			startTs:  0,
			ts:       11e9,
			expected: 0,
		},
		{
			name:     "malformed data",
			startTs:  710000000,
			ts:       0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferDeltaInterval(tt.startTs, tt.ts)
			assert.Equal(t, tt.expected, got)
		})
	}
}
