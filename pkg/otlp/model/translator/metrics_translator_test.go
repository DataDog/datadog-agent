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
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/quantile/summary"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/model/pdata"
	conventions "go.opentelemetry.io/collector/model/semconv/v1.5.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes"
)

func TestIsCumulativeMonotonic(t *testing.T) {
	// Some of these examples are from the hostmetrics receiver
	// and reflect the semantic meaning of the metrics there.
	//
	// If the receiver changes these examples should be added here too

	{ // Sum: Cumulative but not monotonic
		metric := pdata.NewMetric()
		metric.SetName("system.filesystem.usage")
		metric.SetDescription("Filesystem bytes used.")
		metric.SetUnit("bytes")
		metric.SetDataType(pdata.MetricDataTypeSum)
		sum := metric.Sum()
		sum.SetIsMonotonic(false)
		sum.SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)

		assert.False(t, isCumulativeMonotonic(metric))
	}

	{ // Sum: Cumulative and monotonic
		metric := pdata.NewMetric()
		metric.SetName("system.network.packets")
		metric.SetDescription("The number of packets transferred.")
		metric.SetUnit("1")
		metric.SetDataType(pdata.MetricDataTypeSum)
		sum := metric.Sum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)

		assert.True(t, isCumulativeMonotonic(metric))
	}

	{ // DoubleSumL Cumulative and monotonic
		metric := pdata.NewMetric()
		metric.SetName("metric.example")
		metric.SetDataType(pdata.MetricDataTypeSum)
		sum := metric.Sum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)

		assert.True(t, isCumulativeMonotonic(metric))
	}

	{ // Not IntSum
		metric := pdata.NewMetric()
		metric.SetName("system.cpu.load_average.1m")
		metric.SetDescription("Average CPU Load over 1 minute.")
		metric.SetUnit("1")
		metric.SetDataType(pdata.MetricDataTypeGauge)

		assert.False(t, isCumulativeMonotonic(metric))
	}
}

type testProvider string

func (t testProvider) Hostname(context.Context) (string, error) {
	return string(t), nil
}

func newTranslator(t *testing.T, logger *zap.Logger, opts ...Option) *Translator {
	options := append([]Option{
		WithFallbackHostnameProvider(testProvider("fallbackHostname")),
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
	return metric{name: dims.name, typ: Count, timestamp: ts, value: val, tags: dims.tags}
}

func newSketch(dims *Dimensions, ts uint64, s summary.Summary) sketch {
	return sketch{name: dims.name, basic: s, timestamp: ts, tags: dims.tags}
}

func TestMapIntMetrics(t *testing.T) {
	ts := pdata.NewTimestampFromTime(time.Now())
	slice := pdata.NewNumberDataPointSlice()
	point := slice.AppendEmpty()
	point.SetIntVal(17)
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
	ts := pdata.NewTimestampFromTime(time.Now())
	slice := pdata.NewNumberDataPointSlice()
	point := slice.AppendEmpty()
	point.SetDoubleVal(math.Pi)
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

func seconds(i int) pdata.Timestamp {
	return pdata.NewTimestampFromTime(time.Unix(int64(i), 0))
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
	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetIntVal(val)
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
	slice := pdata.NewNumberDataPointSlice()

	// No tags
	point := slice.AppendEmpty()
	point.SetTimestamp(seconds(0))

	point = slice.AppendEmpty()
	point.SetIntVal(20)
	point.SetTimestamp(seconds(1))

	// One tag: valA
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().InsertString("key1", "valA")

	point = slice.AppendEmpty()
	point.SetIntVal(30)
	point.SetTimestamp(seconds(1))
	point.Attributes().InsertString("key1", "valA")

	// same tag: valB
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().InsertString("key1", "valB")

	point = slice.AppendEmpty()
	point.SetIntVal(40)
	point.SetTimestamp(seconds(1))
	point.Attributes().InsertString("key1", "valB")

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
	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(i))
		point.SetIntVal(val)
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

func TestMapIntMonotonicOutOfOrder(t *testing.T) {
	stamps := []int{1, 0, 2, 3}
	values := []int64{0, 1, 2, 3}

	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(stamps[i]))
		point.SetIntVal(val)
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
	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(cumulative))
	for i, val := range cumulative {
		point := slice.AppendEmpty()
		point.SetDoubleVal(val)
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
	slice := pdata.NewNumberDataPointSlice()

	// No tags
	point := slice.AppendEmpty()
	point.SetTimestamp(seconds(0))

	point = slice.AppendEmpty()
	point.SetDoubleVal(20)
	point.SetTimestamp(seconds(1))

	// One tag: valA
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().InsertString("key1", "valA")

	point = slice.AppendEmpty()
	point.SetDoubleVal(30)
	point.SetTimestamp(seconds(1))
	point.Attributes().InsertString("key1", "valA")

	// one tag: valB
	point = slice.AppendEmpty()
	point.SetTimestamp(seconds(0))
	point.Attributes().InsertString("key1", "valB")

	point = slice.AppendEmpty()
	point.SetDoubleVal(40)
	point.SetTimestamp(seconds(1))
	point.Attributes().InsertString("key1", "valB")

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
	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(2 * i))
		point.SetDoubleVal(val)
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

func TestMapDoubleMonotonicOutOfOrder(t *testing.T) {
	stamps := []int{1, 0, 2, 3}
	values := []float64{0, 1, 2, 3}

	slice := pdata.NewNumberDataPointSlice()
	slice.EnsureCapacity(len(values))

	for i, val := range values {
		point := slice.AppendEmpty()
		point.SetTimestamp(seconds(stamps[i]))
		point.SetDoubleVal(val)
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

func dimsWithBucket(dims *Dimensions, lowerBound string, upperBound string) *Dimensions {
	dims = dims.WithSuffix("bucket")
	return dims.AddTags(
		fmt.Sprintf("lower_bound:%s", lowerBound),
		fmt.Sprintf("upper_bound:%s", upperBound),
	)
}

func TestMapDeltaHistogramMetrics(t *testing.T) {
	ts := pdata.NewTimestampFromTime(time.Now())
	slice := pdata.NewHistogramDataPointSlice()
	point := slice.AppendEmpty()
	point.SetCount(20)
	point.SetSum(math.Pi)
	point.SetBucketCounts([]uint64{2, 18})
	point.SetExplicitBounds([]float64{0})
	point.SetTimestamp(ts)

	dims := newDims("doubleHist.test")
	dimsTags := dims.AddTags("attribute_tag:attribute_value")
	counts := []metric{
		newCount(dims.WithSuffix("count"), uint64(ts), 20),
		newCount(dims.WithSuffix("sum"), uint64(ts), math.Pi),
	}

	countsAttributeTags := []metric{
		newCount(dimsTags.WithSuffix("count"), uint64(ts), 20),
		newCount(dimsTags.WithSuffix("sum"), uint64(ts), math.Pi),
	}

	bucketsCounts := []metric{
		newCount(dimsWithBucket(dims, "-inf", "0"), uint64(ts), 2),
		newCount(dimsWithBucket(dims, "0", "inf"), uint64(ts), 18),
	}

	bucketsCountsAttributeTags := []metric{
		newCount(dimsWithBucket(dimsTags, "-inf", "0"), uint64(ts), 2),
		newCount(dimsWithBucket(dimsTags, "0", "inf"), uint64(ts), 18),
	}

	sketches := []sketch{
		newSketch(dims, uint64(ts), summary.Summary{
			Min: 0,
			Max: 0,
			Sum: 0,
			Avg: 0,
			Cnt: 20,
		}),
	}

	sketchesAttributeTags := []sketch{
		newSketch(dimsTags, uint64(ts), summary.Summary{
			Min: 0,
			Max: 0,
			Sum: 0,
			Avg: 0,
			Cnt: 20,
		}),
	}

	ctx := context.Background()
	delta := true

	tests := []struct {
		name             string
		histogramMode    HistogramMode
		sendCountSum     bool
		tags             []string
		expectedMetrics  []metric
		expectedSketches []sketch
	}{
		{
			name:             "No buckets: send count & sum metrics, no attribute tags",
			histogramMode:    HistogramModeNoBuckets,
			sendCountSum:     true,
			expectedMetrics:  counts,
			expectedSketches: []sketch{},
		},
		{
			name:             "No buckets: send count & sum metrics, attribute tags",
			histogramMode:    HistogramModeNoBuckets,
			sendCountSum:     true,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  countsAttributeTags,
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: do not send count & sum metrics, no tags",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     false,
			tags:             []string{},
			expectedMetrics:  bucketsCounts,
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: do not send count & sum metrics, attribute tags",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     false,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  bucketsCountsAttributeTags,
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: send count & sum metrics, no tags",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     true,
			tags:             []string{},
			expectedMetrics:  append(counts, bucketsCounts...),
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: send count & sum metrics, attribute tags",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     true,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  append(countsAttributeTags, bucketsCountsAttributeTags...),
			expectedSketches: []sketch{},
		},
		{
			name:             "Distributions: do not send count & sum metrics, no tags",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     false,
			tags:             []string{},
			expectedMetrics:  []metric{},
			expectedSketches: sketches,
		},
		{
			name:             "Distributions: do not send count & sum metrics, attribute tags",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     false,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  []metric{},
			expectedSketches: sketchesAttributeTags,
		},
		{
			name:             "Distributions: send count & sum metrics, no tags",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     true,
			tags:             []string{},
			expectedMetrics:  counts,
			expectedSketches: sketches,
		},
		{
			name:             "Distributions: send count & sum metrics, attribute tags",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     true,
			tags:             []string{"attribute_tag:attribute_value"},
			expectedMetrics:  countsAttributeTags,
			expectedSketches: sketchesAttributeTags,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			tr := newTranslator(t, zap.NewNop())
			tr.cfg.HistMode = testInstance.histogramMode
			tr.cfg.SendCountSum = testInstance.sendCountSum
			consumer := &mockFullConsumer{}
			dims := &Dimensions{name: "doubleHist.test", tags: testInstance.tags}
			tr.mapHistogramMetrics(ctx, consumer, dims, slice, delta)
			assert.ElementsMatch(t, consumer.metrics, testInstance.expectedMetrics)
			assert.ElementsMatch(t, consumer.sketches, testInstance.expectedSketches)
		})
	}
}

func TestMapCumulativeHistogramMetrics(t *testing.T) {
	slice := pdata.NewHistogramDataPointSlice()
	point := slice.AppendEmpty()
	point.SetCount(20)
	point.SetSum(math.Pi)
	point.SetBucketCounts([]uint64{2, 18})
	point.SetExplicitBounds([]float64{0})
	point.SetTimestamp(seconds(0))

	point = slice.AppendEmpty()
	point.SetCount(20 + 30)
	point.SetSum(math.Pi + 20)
	point.SetBucketCounts([]uint64{2 + 11, 18 + 19})
	point.SetExplicitBounds([]float64{0})
	point.SetTimestamp(seconds(2))

	dims := newDims("doubleHist.test")
	counts := []metric{
		newCount(dims.WithSuffix("count"), uint64(seconds(2)), 30),
		newCount(dims.WithSuffix("sum"), uint64(seconds(2)), 20),
	}

	bucketsCounts := []metric{
		newCount(dimsWithBucket(dims, "-inf", "0"), uint64(seconds(2)), 11),
		newCount(dimsWithBucket(dims, "0", "inf"), uint64(seconds(2)), 19),
	}

	sketches := []sketch{
		newSketch(dims, uint64(seconds(2)), summary.Summary{
			Min: 0,
			Max: 0,
			Sum: 0,
			Avg: 0,
			Cnt: 30,
		}),
	}

	ctx := context.Background()
	delta := false

	tests := []struct {
		name             string
		histogramMode    HistogramMode
		sendCountSum     bool
		expectedMetrics  []metric
		expectedSketches []sketch
	}{
		{
			name:             "No buckets: send count & sum metrics",
			histogramMode:    HistogramModeNoBuckets,
			sendCountSum:     true,
			expectedMetrics:  counts,
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: do not send count & sum metrics",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     false,
			expectedMetrics:  bucketsCounts,
			expectedSketches: []sketch{},
		},
		{
			name:             "Counters: send count & sum metrics",
			histogramMode:    HistogramModeCounters,
			sendCountSum:     true,
			expectedMetrics:  append(counts, bucketsCounts...),
			expectedSketches: []sketch{},
		},
		{
			name:             "Distributions: do not send count & sum metrics",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     false,
			expectedMetrics:  []metric{},
			expectedSketches: sketches,
		},
		{
			name:             "Distributions: send count & sum metrics",
			histogramMode:    HistogramModeDistributions,
			sendCountSum:     true,
			expectedMetrics:  counts,
			expectedSketches: sketches,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			tr := newTranslator(t, zap.NewNop())
			tr.cfg.HistMode = testInstance.histogramMode
			tr.cfg.SendCountSum = testInstance.sendCountSum
			consumer := &mockFullConsumer{}
			dims := newDims("doubleHist.test")
			tr.mapHistogramMetrics(ctx, consumer, dims, slice, delta)
			assert.ElementsMatch(t, consumer.metrics, testInstance.expectedMetrics)
			assert.ElementsMatch(t, consumer.sketches, testInstance.expectedSketches)
		})
	}
}

func TestLegacyBucketsTags(t *testing.T) {
	// Test that passing the same tags slice doesn't reuse the slice.
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	tags := make([]string, 0, 10)

	pointOne := pdata.NewHistogramDataPoint()
	pointOne.SetBucketCounts([]uint64{2, 18})
	pointOne.SetExplicitBounds([]float64{0})
	pointOne.SetTimestamp(seconds(0))
	consumer := &mockTimeSeriesConsumer{}
	dims := &Dimensions{name: "test.histogram.one", tags: tags}
	tr.getLegacyBuckets(ctx, consumer, dims, pointOne, true)
	seriesOne := consumer.metrics

	pointTwo := pdata.NewHistogramDataPoint()
	pointTwo.SetBucketCounts([]uint64{2, 18})
	pointTwo.SetExplicitBounds([]float64{1})
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

func exampleSummaryDataPointSlice(ts pdata.Timestamp, sum float64, count uint64) pdata.SummaryDataPointSlice {
	slice := pdata.NewSummaryDataPointSlice()
	point := slice.AppendEmpty()
	point.SetCount(count)
	point.SetSum(sum)
	qSlice := point.QuantileValues()

	qMin := qSlice.AppendEmpty()
	qMin.SetQuantile(0.0)
	qMin.SetValue(0)

	qMedian := qSlice.AppendEmpty()
	qMedian.SetQuantile(0.5)
	qMedian.SetValue(100)

	q999 := qSlice.AppendEmpty()
	q999.SetQuantile(0.999)
	q999.SetValue(500)

	qMax := qSlice.AppendEmpty()
	qMax.SetQuantile(1)
	qMax.SetValue(600)
	point.SetTimestamp(ts)
	return slice
}

func TestMapSummaryMetrics(t *testing.T) {
	ts := pdata.NewTimestampFromTime(time.Now())
	slice := exampleSummaryDataPointSlice(ts, 10_001, 101)

	newTranslator := func(tags []string, quantiles bool) *Translator {
		c := newTestCache()
		c.cache.Set((&Dimensions{name: "summary.example.count", tags: tags}).String(), numberCounter{0, 0, 1}, gocache.NoExpiration)
		c.cache.Set((&Dimensions{name: "summary.example.sum", tags: tags}).String(), numberCounter{0, 0, 1}, gocache.NoExpiration)
		options := []Option{WithFallbackHostnameProvider(testProvider("fallbackHostname"))}
		if quantiles {
			options = append(options, WithQuantiles())
		}
		tr, err := New(zap.NewNop(), options...)
		require.NoError(t, err)
		tr.prevPts = c
		return tr
	}

	dims := newDims("summary.example")
	noQuantiles := []metric{
		newCount(dims.WithSuffix("count"), uint64(ts), 100),
		newCount(dims.WithSuffix("sum"), uint64(ts), 10_000),
	}
	qBaseDims := dims.WithSuffix("quantile")
	quantiles := []metric{
		newGauge(qBaseDims.AddTags("quantile:0"), uint64(ts), 0),
		newGauge(qBaseDims.AddTags("quantile:0.5"), uint64(ts), 100),
		newGauge(qBaseDims.AddTags("quantile:0.999"), uint64(ts), 500),
		newGauge(qBaseDims.AddTags("quantile:1.0"), uint64(ts), 600),
	}
	ctx := context.Background()
	tr := newTranslator([]string{}, false)
	consumer := &mockTimeSeriesConsumer{}
	tr.mapSummaryMetrics(ctx, consumer, dims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		noQuantiles,
	)
	tr = newTranslator([]string{}, true)
	consumer = &mockTimeSeriesConsumer{}
	tr.mapSummaryMetrics(ctx, consumer, dims, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		append(noQuantiles, quantiles...),
	)

	dimsTags := dims.AddTags("attribute_tag:attribute_value")
	noQuantilesAttr := []metric{
		newCount(dimsTags.WithSuffix("count"), uint64(ts), 100),
		newCount(dimsTags.WithSuffix("sum"), uint64(ts), 10_000),
	}

	qBaseDimsTags := dimsTags.WithSuffix("quantile")
	quantilesAttr := []metric{
		newGauge(qBaseDimsTags.AddTags("quantile:0"), uint64(ts), 0),
		newGauge(qBaseDimsTags.AddTags("quantile:0.5"), uint64(ts), 100),
		newGauge(qBaseDimsTags.AddTags("quantile:0.999"), uint64(ts), 500),
		newGauge(qBaseDimsTags.AddTags("quantile:1.0"), uint64(ts), 600),
	}
	tr = newTranslator([]string{"attribute_tag:attribute_value"}, false)
	consumer = &mockTimeSeriesConsumer{}
	tr.mapSummaryMetrics(ctx, consumer, dimsTags, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		noQuantilesAttr,
	)
	tr = newTranslator([]string{"attribute_tag:attribute_value"}, true)
	consumer = &mockTimeSeriesConsumer{}
	tr.mapSummaryMetrics(ctx, consumer, dimsTags, slice)
	assert.ElementsMatch(t,
		consumer.metrics,
		append(noQuantilesAttr, quantilesAttr...),
	)
}

const (
	testHostname = "res-hostname"
)

func createTestMetrics(additionalAttributes map[string]string, name, version string) pdata.Metrics {
	md := pdata.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	attrs := rm.Resource().Attributes()
	attrs.InsertString(attributes.AttributeDatadogHostname, testHostname)
	for attr, val := range additionalAttributes {
		attrs.InsertString(attr, val)
	}
	ilms := rm.InstrumentationLibraryMetrics()

	ilm := ilms.AppendEmpty()
	ilm.InstrumentationLibrary().SetName(name)
	ilm.InstrumentationLibrary().SetVersion(version)
	metricsArray := ilm.Metrics()
	metricsArray.AppendEmpty() // first one is TypeNone to test that it's ignored

	// IntGauge
	met := metricsArray.AppendEmpty()
	met.SetName("int.gauge")
	met.SetDataType(pdata.MetricDataTypeGauge)
	dpsInt := met.Gauge().DataPoints()
	dpInt := dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(0))
	dpInt.SetIntVal(1)

	// DoubleGauge
	met = metricsArray.AppendEmpty()
	met.SetName("double.gauge")
	met.SetDataType(pdata.MetricDataTypeGauge)
	dpsDouble := met.Gauge().DataPoints()
	dpDouble := dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.Pi)

	// aggregation unspecified sum
	met = metricsArray.AppendEmpty()
	met.SetName("unspecified.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityUnspecified)

	// Int Sum (delta)
	met = metricsArray.AppendEmpty()
	met.SetName("int.delta.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsInt = met.Sum().DataPoints()
	dpInt = dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(0))
	dpInt.SetIntVal(2)

	// Double Sum (delta)
	met = metricsArray.AppendEmpty()
	met.SetName("double.delta.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDouble = met.Sum().DataPoints()
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.E)

	// Int Sum (delta monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("int.delta.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsInt = met.Sum().DataPoints()
	dpInt = dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(0))
	dpInt.SetIntVal(2)

	// Double Sum (delta monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("double.delta.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDouble = met.Sum().DataPoints()
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.E)

	// aggregation unspecified histogram
	met = metricsArray.AppendEmpty()
	met.SetName("unspecified.histogram")
	met.SetDataType(pdata.MetricDataTypeHistogram)
	met.Histogram().SetAggregationTemporality(pdata.MetricAggregationTemporalityUnspecified)

	// Histogram (delta)
	met = metricsArray.AppendEmpty()
	met.SetName("double.histogram")
	met.SetDataType(pdata.MetricDataTypeHistogram)
	met.Histogram().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDoubleHist := met.Histogram().DataPoints()
	dpDoubleHist := dpsDoubleHist.AppendEmpty()
	dpDoubleHist.SetCount(20)
	dpDoubleHist.SetSum(math.Phi)
	dpDoubleHist.SetBucketCounts([]uint64{2, 18})
	dpDoubleHist.SetExplicitBounds([]float64{0})
	dpDoubleHist.SetTimestamp(seconds(0))

	// Int Sum (cumulative)
	met = metricsArray.AppendEmpty()
	met.SetName("int.cumulative.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	dpsInt = met.Sum().DataPoints()
	dpsInt.EnsureCapacity(2)
	dpInt = dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(0))
	dpInt.SetIntVal(4)

	// Double Sum (cumulative)
	met = metricsArray.AppendEmpty()
	met.SetName("double.cumulative.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	dpsDouble = met.Sum().DataPoints()
	dpsDouble.EnsureCapacity(2)
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(4)

	// Int Sum (cumulative monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("int.cumulative.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	met.Sum().SetIsMonotonic(true)
	dpsInt = met.Sum().DataPoints()
	dpsInt.EnsureCapacity(2)
	dpInt = dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(0))
	dpInt.SetIntVal(4)
	dpInt = dpsInt.AppendEmpty()
	dpInt.SetTimestamp(seconds(2))
	dpInt.SetIntVal(7)

	// Double Sum (cumulative monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("double.cumulative.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	met.Sum().SetIsMonotonic(true)
	dpsDouble = met.Sum().DataPoints()
	dpsDouble.EnsureCapacity(2)
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(4)
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(2))
	dpDouble.SetDoubleVal(4 + math.Pi)

	// Summary
	met = metricsArray.AppendEmpty()
	met.SetName("summary")
	met.SetDataType(pdata.MetricDataTypeSummary)
	slice := exampleSummaryDataPointSlice(seconds(0), 1, 1)
	slice.CopyTo(met.Summary().DataPoints())

	met = metricsArray.AppendEmpty()
	met.SetName("summary")
	met.SetDataType(pdata.MetricDataTypeSummary)
	slice = exampleSummaryDataPointSlice(seconds(2), 10_001, 101)
	slice.CopyTo(met.Summary().DataPoints())
	return md
}

func newGaugeWithHostname(name string, val float64, tags []string) metric {
	dims := newDims(name)
	m := newGauge(dims.AddTags(tags...), 0, val)
	m.host = testHostname
	return m
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

func TestMapMetrics(t *testing.T) {
	attrs := map[string]string{
		conventions.AttributeDeploymentEnvironment: "dev",
		"custom_attribute":                         "custom_value",
	}

	// Attributes defined in internal/attributes get converted to tags.
	// Other tags do not get converted if ResourceAttributesAsTags is false,
	// or are converted into datapoint-level attributes (which are then converted to tags) by
	// the resourcetotelemetry helper if ResourceAttributesAsTags is true
	// (outside of the MapMetrics function's scope).
	attrTags := []string{
		"env:dev",
	}

	ilName := "instrumentation_library"
	ilVersion := "1.0.0"
	ilTags := []string{
		fmt.Sprintf("instrumentation_library:%s", ilName),
		fmt.Sprintf("instrumentation_library_version:%s", ilVersion),
	}

	tests := []struct {
		name                                      string
		resourceAttributesAsTags                  bool
		instrumentationLibraryMetadataAsTags      bool
		expectedMetrics                           []metric
		expectedSketches                          []sketch
		expectedUnknownMetricType                 int
		expectedUnsupportedAggregationTemporality int
	}{
		{
			name:                                 "ResourceAttributesAsTags: false, InstrumentationLibraryMetadataAsTags: false",
			resourceAttributesAsTags:             false,
			instrumentationLibraryMetadataAsTags: false,
			expectedMetrics: []metric{
				newGaugeWithHostname("int.gauge", 1, attrTags),
				newGaugeWithHostname("double.gauge", math.Pi, attrTags),
				newCountWithHostname("int.delta.sum", 2, 0, attrTags),
				newCountWithHostname("double.delta.sum", math.E, 0, attrTags),
				newCountWithHostname("int.delta.monotonic.sum", 2, 0, attrTags),
				newCountWithHostname("double.delta.monotonic.sum", math.E, 0, attrTags),
				newCountWithHostname("summary.sum", 10_000, 2, attrTags),
				newCountWithHostname("summary.count", 100, 2, attrTags),
				newGaugeWithHostname("int.cumulative.sum", 4, attrTags),
				newGaugeWithHostname("double.cumulative.sum", 4, attrTags),
				newCountWithHostname("int.cumulative.monotonic.sum", 3, 2, attrTags),
				newCountWithHostname("double.cumulative.monotonic.sum", math.Pi, 2, attrTags),
			},
			expectedSketches: []sketch{
				newSketchWithHostname("double.histogram", summary.Summary{
					Min: 0,
					Max: 0,
					Sum: 0,
					Avg: 0,
					Cnt: 20,
				}, attrTags),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:                                 "ResourceAttributesAsTags: true, InstrumentationLibraryMetadataAsTags: false",
			resourceAttributesAsTags:             true,
			instrumentationLibraryMetadataAsTags: false,
			expectedMetrics: []metric{
				newGaugeWithHostname("int.gauge", 1, attrTags),
				newGaugeWithHostname("double.gauge", math.Pi, attrTags),
				newCountWithHostname("int.delta.sum", 2, 0, attrTags),
				newCountWithHostname("double.delta.sum", math.E, 0, attrTags),
				newCountWithHostname("int.delta.monotonic.sum", 2, 0, attrTags),
				newCountWithHostname("double.delta.monotonic.sum", math.E, 0, attrTags),
				newCountWithHostname("summary.sum", 10_000, 2, attrTags),
				newCountWithHostname("summary.count", 100, 2, attrTags),
				newGaugeWithHostname("int.cumulative.sum", 4, attrTags),
				newGaugeWithHostname("double.cumulative.sum", 4, attrTags),
				newCountWithHostname("int.cumulative.monotonic.sum", 3, 2, attrTags),
				newCountWithHostname("double.cumulative.monotonic.sum", math.Pi, 2, attrTags),
			},
			expectedSketches: []sketch{
				newSketchWithHostname("double.histogram", summary.Summary{
					Min: 0,
					Max: 0,
					Sum: 0,
					Avg: 0,
					Cnt: 20,
				}, attrTags),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:                                 "ResourceAttributesAsTags: false, InstrumentationLibraryMetadataAsTags: true",
			resourceAttributesAsTags:             false,
			instrumentationLibraryMetadataAsTags: true,
			expectedMetrics: []metric{
				newGaugeWithHostname("int.gauge", 1, append(attrTags, ilTags...)),
				newGaugeWithHostname("double.gauge", math.Pi, append(attrTags, ilTags...)),
				newCountWithHostname("int.delta.sum", 2, 0, append(attrTags, ilTags...)),
				newCountWithHostname("double.delta.sum", math.E, 0, append(attrTags, ilTags...)),
				newCountWithHostname("int.delta.monotonic.sum", 2, 0, append(attrTags, ilTags...)),
				newCountWithHostname("double.delta.monotonic.sum", math.E, 0, append(attrTags, ilTags...)),
				newCountWithHostname("summary.sum", 10_000, 2, append(attrTags, ilTags...)),
				newCountWithHostname("summary.count", 100, 2, append(attrTags, ilTags...)),
				newGaugeWithHostname("int.cumulative.sum", 4, append(attrTags, ilTags...)),
				newGaugeWithHostname("double.cumulative.sum", 4, append(attrTags, ilTags...)),
				newCountWithHostname("int.cumulative.monotonic.sum", 3, 2, append(attrTags, ilTags...)),
				newCountWithHostname("double.cumulative.monotonic.sum", math.Pi, 2, append(attrTags, ilTags...)),
			},
			expectedSketches: []sketch{
				newSketchWithHostname("double.histogram", summary.Summary{
					Min: 0,
					Max: 0,
					Sum: 0,
					Avg: 0,
					Cnt: 20,
				}, append(attrTags, ilTags...)),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:                                 "ResourceAttributesAsTags: true, InstrumentationLibraryMetadataAsTags: true",
			resourceAttributesAsTags:             true,
			instrumentationLibraryMetadataAsTags: true,
			expectedMetrics: []metric{
				newGaugeWithHostname("int.gauge", 1, append(attrTags, ilTags...)),
				newGaugeWithHostname("double.gauge", math.Pi, append(attrTags, ilTags...)),
				newCountWithHostname("int.delta.sum", 2, 0, append(attrTags, ilTags...)),
				newCountWithHostname("double.delta.sum", math.E, 0, append(attrTags, ilTags...)),
				newCountWithHostname("int.delta.monotonic.sum", 2, 0, append(attrTags, ilTags...)),
				newCountWithHostname("double.delta.monotonic.sum", math.E, 0, append(attrTags, ilTags...)),
				newCountWithHostname("summary.sum", 10_000, 2, append(attrTags, ilTags...)),
				newCountWithHostname("summary.count", 100, 2, append(attrTags, ilTags...)),
				newGaugeWithHostname("int.cumulative.sum", 4, append(attrTags, ilTags...)),
				newGaugeWithHostname("double.cumulative.sum", 4, append(attrTags, ilTags...)),
				newCountWithHostname("int.cumulative.monotonic.sum", 3, 2, append(attrTags, ilTags...)),
				newCountWithHostname("double.cumulative.monotonic.sum", math.Pi, 2, append(attrTags, ilTags...)),
			},
			expectedSketches: []sketch{
				newSketchWithHostname("double.histogram", summary.Summary{
					Min: 0,
					Max: 0,
					Sum: 0,
					Avg: 0,
					Cnt: 20,
				}, append(attrTags, ilTags...)),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			md := createTestMetrics(attrs, ilName, ilVersion)

			core, observed := observer.New(zapcore.DebugLevel)
			testLogger := zap.New(core)
			ctx := context.Background()
			consumer := &mockFullConsumer{}

			options := []Option{}
			if testInstance.resourceAttributesAsTags {
				options = append(options, WithResourceAttributesAsTags())
			}
			if testInstance.instrumentationLibraryMetadataAsTags {
				options = append(options, WithInstrumentationLibraryMetadataAsTags())
			}
			tr := newTranslator(t, testLogger, options...)
			err := tr.MapMetrics(ctx, md, consumer)
			require.NoError(t, err)

			assert.ElementsMatch(t, consumer.metrics, testInstance.expectedMetrics)
			assert.ElementsMatch(t, consumer.sketches, testInstance.expectedSketches)
			assert.Equal(t, observed.FilterMessage("Unknown or unsupported metric type").Len(), testInstance.expectedUnknownMetricType)
			assert.Equal(t, observed.FilterMessage("Unknown or unsupported aggregation temporality").Len(), testInstance.expectedUnsupportedAggregationTemporality)
		})
	}
}

func createNaNMetrics() pdata.Metrics {
	md := pdata.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	attrs := rm.Resource().Attributes()
	attrs.InsertString(attributes.AttributeDatadogHostname, testHostname)
	ilms := rm.InstrumentationLibraryMetrics()

	metricsArray := ilms.AppendEmpty().Metrics()

	// DoubleGauge
	met := metricsArray.AppendEmpty()
	met.SetName("nan.gauge")
	met.SetDataType(pdata.MetricDataTypeGauge)
	dpsDouble := met.Gauge().DataPoints()
	dpDouble := dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.NaN())

	// Double Sum (delta)
	met = metricsArray.AppendEmpty()
	met.SetName("nan.delta.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDouble = met.Sum().DataPoints()
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.NaN())

	// Double Sum (delta monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("nan.delta.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDouble = met.Sum().DataPoints()
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.NaN())

	// Histogram
	met = metricsArray.AppendEmpty()
	met.SetName("nan.histogram")
	met.SetDataType(pdata.MetricDataTypeHistogram)
	met.Histogram().SetAggregationTemporality(pdata.MetricAggregationTemporalityDelta)
	dpsDoubleHist := met.Histogram().DataPoints()
	dpDoubleHist := dpsDoubleHist.AppendEmpty()
	dpDoubleHist.SetCount(20)
	dpDoubleHist.SetSum(math.NaN())
	dpDoubleHist.SetBucketCounts([]uint64{2, 18})
	dpDoubleHist.SetExplicitBounds([]float64{0})
	dpDoubleHist.SetTimestamp(seconds(0))

	// Double Sum (cumulative)
	met = metricsArray.AppendEmpty()
	met.SetName("nan.cumulative.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	dpsDouble = met.Sum().DataPoints()
	dpsDouble.EnsureCapacity(2)
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.NaN())

	// Double Sum (cumulative monotonic)
	met = metricsArray.AppendEmpty()
	met.SetName("nan.cumulative.monotonic.sum")
	met.SetDataType(pdata.MetricDataTypeSum)
	met.Sum().SetAggregationTemporality(pdata.MetricAggregationTemporalityCumulative)
	met.Sum().SetIsMonotonic(true)
	dpsDouble = met.Sum().DataPoints()
	dpsDouble.EnsureCapacity(2)
	dpDouble = dpsDouble.AppendEmpty()
	dpDouble.SetTimestamp(seconds(0))
	dpDouble.SetDoubleVal(math.NaN())

	// Summary
	met = metricsArray.AppendEmpty()
	met.SetName("nan.summary")
	met.SetDataType(pdata.MetricDataTypeSummary)
	slice := exampleSummaryDataPointSlice(seconds(0), math.NaN(), 1)
	slice.CopyTo(met.Summary().DataPoints())

	met = metricsArray.AppendEmpty()
	met.SetName("nan.summary")
	met.SetDataType(pdata.MetricDataTypeSummary)
	slice = exampleSummaryDataPointSlice(seconds(2), 10_001, 101)
	slice.CopyTo(met.Summary().DataPoints())
	return md
}

func TestNaNMetrics(t *testing.T) {
	md := createNaNMetrics()

	core, observed := observer.New(zapcore.DebugLevel)
	testLogger := zap.New(core)
	ctx := context.Background()
	tr := newTranslator(t, testLogger)
	consumer := &mockFullConsumer{}
	err := tr.MapMetrics(ctx, md, consumer)
	require.NoError(t, err)

	assert.ElementsMatch(t, consumer.metrics, []metric{
		newCountWithHostname("nan.summary.count", 100, 2, []string{}),
	})

	assert.ElementsMatch(t, consumer.sketches, []sketch{
		newSketchWithHostname("nan.histogram", summary.Summary{
			Min: 0,
			Max: 0,
			Sum: 0,
			Avg: 0,
			Cnt: 20,
		}, []string{}),
	})

	// One metric type was unknown or unsupported
	assert.Equal(t, observed.FilterMessage("Unsupported metric value").Len(), 6)
}
