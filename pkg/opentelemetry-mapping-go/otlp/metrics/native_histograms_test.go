// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// nativeHistConsumer captures all metric consumer calls for gate ON/OFF assertion.
var _ Consumer = (*nativeHistConsumer)(nil)

type capturedExplicitHistogram struct {
	name    string
	tags    []string
	host    string
	ts      uint64
	delta   bool
	count   uint64
	sum     float64
	bounds  []float64
	buckets []uint64
}

type capturedExpHistogram struct {
	name      string
	tags      []string
	host      string
	ts        uint64
	scale     int32
	zeroCount uint64
	count     uint64
	sum       float64
}

type nativeHistConsumer struct {
	timeseries            []metric
	sketchCount           int
	explicitHistograms    []capturedExplicitHistogram
	exponentialHistograms []capturedExpHistogram
}

func (c *nativeHistConsumer) ConsumeTimeSeries(
	_ context.Context, dims *Dimensions, typ DataType,
	ts uint64, interval int64, val float64,
) {
	c.timeseries = append(c.timeseries, metric{
		name: dims.Name(), typ: typ, timestamp: ts,
		interval: interval, value: val,
		tags: dims.Tags(), host: dims.Host(),
	})
}

func (c *nativeHistConsumer) ConsumeSketch(
	_ context.Context, _ *Dimensions, _ uint64, _ int64, _ *quantile.Sketch,
) {
	c.sketchCount++
}

func (c *nativeHistConsumer) ConsumeExplicitBoundHistogram(
	_ context.Context, dims *Dimensions, ts uint64, _ int64,
	point pmetric.HistogramDataPoint, delta bool,
) {
	c.explicitHistograms = append(c.explicitHistograms, capturedExplicitHistogram{
		name: dims.Name(), tags: dims.Tags(), host: dims.Host(),
		ts: ts, delta: delta,
		count: point.Count(), sum: point.Sum(),
		bounds: point.ExplicitBounds().AsRaw(), buckets: point.BucketCounts().AsRaw(),
	})
}

func (c *nativeHistConsumer) ConsumeExponentialHistogram(
	_ context.Context, dims *Dimensions, ts uint64, _ int64,
	point pmetric.ExponentialHistogramDataPoint,
) {
	c.exponentialHistograms = append(c.exponentialHistograms, capturedExpHistogram{
		name: dims.Name(), tags: dims.Tags(), host: dims.Host(),
		ts: ts, scale: point.Scale(), zeroCount: point.ZeroCount(),
		count: point.Count(), sum: point.Sum(),
	})
}

func (c *nativeHistConsumer) timeseriesNames() []string {
	names := make([]string, len(c.timeseries))
	for i, ts := range c.timeseries {
		names[i] = ts.name
	}
	return names
}

func makeExplicitBoundHistogramMetric(name string, delta bool, bounds []float64, counts []uint64) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	h := m.SetEmptyHistogram()
	if delta {
		h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	} else {
		h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	}

	dp := h.DataPoints().AppendEmpty()
	dp.ExplicitBounds().FromRaw(bounds)
	dp.BucketCounts().FromRaw(counts)

	var total uint64
	for _, c := range counts {
		total += c
	}
	dp.SetCount(total)
	dp.SetSum(42.0)
	dp.SetMin(0.5)
	dp.SetMax(12.0)

	now := pcommon.NewTimestampFromTime(time.Now())
	start := pcommon.NewTimestampFromTime(time.Now().Add(-10 * time.Second))
	dp.SetTimestamp(now)
	dp.SetStartTimestamp(start)
	return md
}

func makeExponentialHistogramMetric(name string, delta bool, scale int32, zeroCount uint64, posCounts []uint64) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	eh := m.SetEmptyExponentialHistogram()
	if delta {
		eh.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	} else {
		eh.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	}

	dp := eh.DataPoints().AppendEmpty()
	dp.SetScale(scale)
	dp.SetZeroCount(zeroCount)
	dp.Positive().SetOffset(0)
	dp.Positive().BucketCounts().FromRaw(posCounts)

	var total uint64 = zeroCount
	for _, c := range posCounts {
		total += c
	}
	dp.SetCount(total)
	dp.SetSum(100.0)
	dp.SetMin(0.0)
	dp.SetMax(50.0)

	now := pcommon.NewTimestampFromTime(time.Now())
	start := pcommon.NewTimestampFromTime(time.Now().Add(-10 * time.Second))
	dp.SetTimestamp(now)
	dp.SetStartTimestamp(start)
	return md
}

func TestNativeHistograms_ExplicitBound(t *testing.T) {
	bounds := []float64{1, 5, 10}
	counts := []uint64{1, 3, 5, 2}

	t.Run("DeltaGateOn", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
			WithHistogramMode(HistogramModeDistributions),
		)
		consumer := &nativeHistConsumer{}
		md := makeExplicitBoundHistogramMetric("test.histogram", true, bounds, counts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		require.Len(t, consumer.explicitHistograms, 1, "ConsumeExplicitBoundHistogram should be called once")
		h := consumer.explicitHistograms[0]
		assert.Equal(t, "test.histogram", h.name)
		assert.True(t, h.delta)
		assert.Equal(t, bounds, h.bounds)
		assert.Equal(t, counts, h.buckets)
		assert.Equal(t, uint64(11), h.count)
		assert.Equal(t, 42.0, h.sum)

		assert.Equal(t, 0, consumer.sketchCount, "ConsumeSketch must not be called when gate is ON")
		assert.Empty(t, consumer.timeseries, "no aggregation timeseries without WithHistogramAggregations")
	})

	t.Run("DeltaGateOnWithAggregations", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
			WithHistogramMode(HistogramModeDistributions),
			WithHistogramAggregations(),
		)
		consumer := &nativeHistConsumer{}
		md := makeExplicitBoundHistogramMetric("test.histogram", true, bounds, counts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		require.Len(t, consumer.explicitHistograms, 1)
		assert.Equal(t, 0, consumer.sketchCount)

		names := consumer.timeseriesNames()
		assert.Contains(t, names, "test.histogram.count")
		assert.Contains(t, names, "test.histogram.sum")
		assert.Contains(t, names, "test.histogram.min")
		assert.Contains(t, names, "test.histogram.max")
	})

	t.Run("CumulativeGateOnSkipped", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
			WithHistogramMode(HistogramModeDistributions),
		)
		consumer := &nativeHistConsumer{}
		md := makeExplicitBoundHistogramMetric("test.histogram", false, bounds, counts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		assert.Empty(t, consumer.explicitHistograms, "cumulative explicit-bound histograms should be skipped when gate is ON")
		assert.Equal(t, 0, consumer.sketchCount)
	})

	t.Run("DeltaGateOffUsesSketch", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithHistogramMode(HistogramModeDistributions),
		)
		consumer := &nativeHistConsumer{}
		md := makeExplicitBoundHistogramMetric("test.histogram", true, bounds, counts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		assert.Greater(t, consumer.sketchCount, 0, "ConsumeSketch should be called when gate is OFF")
		assert.Empty(t, consumer.explicitHistograms, "ConsumeExplicitBoundHistogram should not be called when gate is OFF")
	})
}

func TestNativeHistograms_Exponential(t *testing.T) {
	scale := int32(4)
	zeroCount := uint64(5)
	posCounts := []uint64{10, 20, 30}

	t.Run("DeltaGateOn", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
		)
		consumer := &nativeHistConsumer{}
		md := makeExponentialHistogramMetric("test.exp.histogram", true, scale, zeroCount, posCounts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		require.Len(t, consumer.exponentialHistograms, 1, "ConsumeExponentialHistogram should be called once")
		h := consumer.exponentialHistograms[0]
		assert.Equal(t, "test.exp.histogram", h.name)
		assert.Equal(t, scale, h.scale)
		assert.Equal(t, zeroCount, h.zeroCount)
		assert.Equal(t, uint64(65), h.count)
		assert.Equal(t, 100.0, h.sum)

		assert.Equal(t, 0, consumer.sketchCount, "ConsumeSketch must not be called when gate is ON")
	})

	t.Run("DeltaGateOnWithAggregations", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
			WithHistogramAggregations(),
		)
		consumer := &nativeHistConsumer{}
		md := makeExponentialHistogramMetric("test.exp.histogram", true, scale, zeroCount, posCounts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		require.Len(t, consumer.exponentialHistograms, 1)
		assert.Equal(t, 0, consumer.sketchCount)

		names := consumer.timeseriesNames()
		assert.Contains(t, names, "test.exp.histogram.count")
		assert.Contains(t, names, "test.exp.histogram.sum")
	})

	t.Run("CumulativeGateOnSkipped", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
			WithNativeHistograms(),
		)
		consumer := &nativeHistConsumer{}
		md := makeExponentialHistogramMetric("test.exp.histogram", false, scale, zeroCount, posCounts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		assert.Empty(t, consumer.exponentialHistograms, "cumulative exp histograms should be skipped when gate is ON")
		assert.Equal(t, 0, consumer.sketchCount)
	})

	t.Run("DeltaGateOffUsesSketch", func(t *testing.T) {
		tr := NewTestTranslator(t,
			WithOriginProduct(OriginProductDatadogAgent),
		)
		consumer := &nativeHistConsumer{}
		md := makeExponentialHistogramMetric("test.exp.histogram", true, scale, zeroCount, posCounts)
		_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
		require.NoError(t, err)

		assert.Greater(t, consumer.sketchCount, 0, "ConsumeSketch should be called when gate is OFF")
		assert.Empty(t, consumer.exponentialHistograms, "ConsumeExponentialHistogram should not be called when gate is OFF")
	})
}

func TestNativeHistograms_MultiplePoints(t *testing.T) {
	tr := NewTestTranslator(t,
		WithOriginProduct(OriginProductDatadogAgent),
		WithNativeHistograms(),
		WithHistogramMode(HistogramModeDistributions),
	)

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("multi.point.histogram")
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	now := time.Now()
	for i := 0; i < 3; i++ {
		dp := h.DataPoints().AppendEmpty()
		dp.ExplicitBounds().FromRaw([]float64{10})
		dp.BucketCounts().FromRaw([]uint64{uint64(i + 1), uint64(i + 2)})
		dp.SetCount(uint64(2*i + 3))
		dp.SetSum(float64(i) * 10)
		dp.SetMin(0)
		dp.SetMax(float64(i) * 5)
		ts := pcommon.NewTimestampFromTime(now.Add(time.Duration(i) * time.Second))
		dp.SetTimestamp(ts)
		dp.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i-1) * time.Second)))
	}

	consumer := &nativeHistConsumer{}
	_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
	require.NoError(t, err)

	assert.Len(t, consumer.explicitHistograms, 3, "all 3 delta points should produce histogram consumer calls")
	assert.Equal(t, 0, consumer.sketchCount)
}
