// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package serializerexporter

import (
	"context"
	"testing"
	"unsafe"

	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendHistograms(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)

	edp := pmetric.NewExponentialHistogramDataPoint()
	edp.SetScale(4)
	edp.SetZeroCount(5)
	edp.Positive().SetOffset(0)
	edp.Positive().BucketCounts().FromRaw([]uint64{10, 20, 30})
	edp.SetCount(65)
	edp.SetSum(100.0)

	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
		sketches: metrics.SketchSeriesList{
			{
				Name:     "test.histogram",
				Tags:     tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:     "testhost",
				Interval: 10,
				Points: []metrics.SketchPoint{{
					Ts:     0,
					Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp},
				}},
				Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
			{
				Name:     "test.exp.histogram",
				Tags:     tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:     "testhost",
				Interval: 10,
				Points: []metrics.SketchPoint{{
					Ts:     0,
					Sketch: &metrics.ExponentialHistogramPoint{Point: edp},
				}},
				Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	var explicit, exponential []*metrics.SketchSeries
	for _, s := range mock.sketches {
		if len(s.Points) == 0 {
			continue
		}
		switch s.Points[0].Sketch.(type) {
		case metrics.ExplicitBoundProvider:
			explicit = append(explicit, s)
		case metrics.ExponentialProvider:
			exponential = append(exponential, s)
		}
	}

	require.Len(t, explicit, 1)
	assert.Equal(t, "test.histogram", explicit[0].Name)
	assert.Equal(t, "testhost", explicit[0].Host)
	assert.Equal(t, int64(10), explicit[0].Interval)
	require.Len(t, explicit[0].Points, 1)
	ep := explicit[0].Points[0].Sketch.(metrics.ExplicitBoundProvider)
	assert.Equal(t, uint64(11), ep.Count())
	assert.Equal(t, 42.0, ep.Sum())

	require.Len(t, exponential, 1)
	assert.Equal(t, "test.exp.histogram", exponential[0].Name)
	assert.Equal(t, "testhost", exponential[0].Host)
	require.Len(t, exponential[0].Points, 1)
	xp := exponential[0].Points[0].Sketch.(metrics.ExponentialProvider)
	assert.Equal(t, uint64(65), xp.Count())
	assert.Equal(t, int32(4), xp.Scale())
}

func TestSendHistograms_Multiple(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	edp := pmetric.NewExponentialHistogramDataPoint()

	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
		sketches: metrics.SketchSeriesList{
			{Name: "hist1", Host: "host1", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "hist2", Host: "host2", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "hist3", Host: "host3", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "exp1", Host: "host1", Points: []metrics.SketchPoint{{Sketch: &metrics.ExponentialHistogramPoint{Point: edp}}}},
			{Name: "exp2", Host: "host2", Points: []metrics.SketchPoint{{Sketch: &metrics.ExponentialHistogramPoint{Point: edp}}}},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	var explicit, exponential []*metrics.SketchSeries
	for _, s := range mock.sketches {
		if len(s.Points) == 0 {
			continue
		}
		switch s.Points[0].Sketch.(type) {
		case metrics.ExplicitBoundProvider:
			explicit = append(explicit, s)
		case metrics.ExponentialProvider:
			exponential = append(exponential, s)
		}
	}

	require.Len(t, explicit, 3)
	assert.Equal(t, "hist1", explicit[0].Name)
	assert.Equal(t, "hist2", explicit[1].Name)
	assert.Equal(t, "hist3", explicit[2].Name)

	require.Len(t, exponential, 2)
	assert.Equal(t, "exp1", exponential[0].Name)
	assert.Equal(t, "exp2", exponential[1].Name)
}

// dimensionsMirror mirrors the memory layout of otlpmetrics.Dimensions.
type dimensionsMirror struct {
	name                string
	tags                []string
	host                string
	originID            string
	originProduct       otlpmetrics.OriginProduct
	originSubProduct    otlpmetrics.OriginSubProduct
	originProductDetail otlpmetrics.OriginProductDetail
}

func makeTestDimensions(name, host string, tags []string) *otlpmetrics.Dimensions {
	m := dimensionsMirror{name: name, tags: tags, host: host}
	return (*otlpmetrics.Dimensions)(unsafe.Pointer(&m))
}

func TestConsumeExplicitBoundHistogram(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)

	dims := makeTestDimensions("http.request.duration", "web-1", []string{"env:prod", "service:api"})

	sc := serializerConsumer{
		extraTags: []string{"extra:tag"},
	}
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 5_000_000_000, 10, dp, true)

	require.Len(t, sc.sketches, 1)
	s := sc.sketches[0]
	assert.Equal(t, "http.request.duration", s.Name)
	assert.Equal(t, "web-1", s.Host)
	assert.Equal(t, int64(10), s.Interval)
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, s.Source)

	var tags []string
	s.Tags.ForEach(func(tag string) { tags = append(tags, tag) })
	assert.Contains(t, tags, "extra:tag")
	assert.Contains(t, tags, "env:prod")
	assert.Contains(t, tags, "service:api")

	require.Len(t, s.Points, 1)
	assert.Equal(t, int64(5), s.Points[0].Ts)

	ep, ok := s.Points[0].Sketch.(*metrics.ExplicitBoundHistogramPoint)
	require.True(t, ok)
	assert.Equal(t, uint64(11), ep.Count())
	assert.Equal(t, 42.0, ep.Sum())
	assert.Equal(t, []float64{1, 5, 10}, ep.ExplicitBounds())
	assert.Equal(t, []uint64{1, 3, 5, 2}, ep.BucketCounts())
}

func TestConsumeExponentialHistogram(t *testing.T) {
	dp := pmetric.NewExponentialHistogramDataPoint()
	dp.SetScale(4)
	dp.SetZeroCount(5)
	dp.Positive().SetOffset(0)
	dp.Positive().BucketCounts().FromRaw([]uint64{10, 20, 30})
	dp.Negative().SetOffset(1)
	dp.Negative().BucketCounts().FromRaw([]uint64{7, 8})
	dp.SetCount(80)
	dp.SetSum(200.0)

	dims := makeTestDimensions("http.request.latency", "api-2", []string{"region:us-east"})

	sc := serializerConsumer{
		extraTags: []string{"team:backend"},
	}
	sc.ConsumeExponentialHistogram(context.Background(), dims, 10_000_000_000, 30, dp)

	require.Len(t, sc.sketches, 1)
	s := sc.sketches[0]
	assert.Equal(t, "http.request.latency", s.Name)
	assert.Equal(t, "api-2", s.Host)
	assert.Equal(t, int64(30), s.Interval)
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, s.Source)

	var tags []string
	s.Tags.ForEach(func(tag string) { tags = append(tags, tag) })
	assert.Contains(t, tags, "team:backend")
	assert.Contains(t, tags, "region:us-east")

	require.Len(t, s.Points, 1)
	assert.Equal(t, int64(10), s.Points[0].Ts)

	xp, ok := s.Points[0].Sketch.(*metrics.ExponentialHistogramPoint)
	require.True(t, ok)
	assert.Equal(t, int32(4), xp.Scale())
	assert.Equal(t, uint64(5), xp.ZeroCount())
	assert.Equal(t, uint64(80), xp.Count())
	assert.Equal(t, 200.0, xp.Sum())
	assert.Equal(t, []uint64{10, 20, 30}, xp.PositiveBucketCounts())
	assert.Equal(t, []uint64{7, 8}, xp.NegativeBucketCounts())
}

func TestConsumeHistograms_Accumulates(t *testing.T) {
	dp1 := pmetric.NewHistogramDataPoint()
	dp1.SetCount(5)
	dp2 := pmetric.NewExponentialHistogramDataPoint()
	dp2.SetCount(10)
	dp3 := pmetric.NewHistogramDataPoint()
	dp3.SetCount(15)

	dims := makeTestDimensions("metric", "host", nil)

	sc := serializerConsumer{}
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 1_000_000_000, 10, dp1, false)
	sc.ConsumeExponentialHistogram(context.Background(), dims, 2_000_000_000, 10, dp2)
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 3_000_000_000, 10, dp3, false)

	require.Len(t, sc.sketches, 3)
	assert.Equal(t, int64(1), sc.sketches[0].Points[0].Ts)
	assert.Equal(t, int64(2), sc.sketches[1].Points[0].Ts)
	assert.Equal(t, int64(3), sc.sketches[2].Points[0].Ts)
}
