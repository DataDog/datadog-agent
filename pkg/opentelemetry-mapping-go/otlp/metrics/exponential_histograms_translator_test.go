// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// newExpHistogramMetrics builds a single-datapoint delta exponential histogram with
// the given scale and positive bucket layout.
func newExpHistogramMetrics(scale int32, offset int32, counts []uint64) pmetric.Metrics {
	md := pmetric.NewMetrics()
	m := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("expohist.test")
	eh := m.SetEmptyExponentialHistogram()
	eh.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	dp := eh.DataPoints().AppendEmpty()
	dp.SetTimestamp(seconds(0))
	dp.SetScale(scale)

	var total uint64
	pos := dp.Positive()
	pos.SetOffset(offset)
	for _, c := range counts {
		pos.BucketCounts().Append(c)
		total += c
	}
	dp.SetCount(total)
	dp.SetSum(0.627158)
	dp.SetMin(0.143741)
	dp.SetMax(0.327218)
	return md
}

// TestExponentialHistogramOutOfRangeBoundsNoPanic covers
// https://github.com/DataDog/datadog-agent/issues/55140: bucket boundaries that
// overflow float64 used to reach DDSketch.ChangeMapping as +Inf or NaN, which
// converts them to math.MinInt64 on amd64 and panics inside DenseStore.
//
// The overflowing points must now be dropped with an error, and the representable
// ones must still produce a sketch.
func TestExponentialHistogramOutOfRangeBoundsNoPanic(t *testing.T) {
	tests := []struct {
		name string
		// scale and offset of the datapoint. Three populated buckets are used, so the
		// highest boundary evaluated is 2^((offset+3) * 2^-scale).
		scale  int32
		offset int32
		// expectError is whether the boundaries are out of float64 range.
		expectError bool
		// expectSketch is whether MapMetrics still emits a sketch. A point can pass the
		// bounds check and yet be dropped further down by the Agent sketch's own index
		// limit (pkg/util/quantile maxIndex), which is pre-existing behaviour.
		expectSketch bool
	}{
		// Scales reported in the issue: gamma = 2^(2^-scale) overflows at scale <= -10,
		// and the boundary of bucket 3 already overflows at scale -9.
		{name: "scale -10 gamma overflows", scale: -10, offset: 0, expectError: true},
		{name: "scale -9 boundary overflows", scale: -9, offset: 0, expectError: true},
		// In-spec scales are equally affected: the bucket offset alone pushes the
		// boundary out of range. offset is a sint32 in OTLP, so this is well-formed input.
		{name: "scale 0 offset in range", scale: 0, offset: 1000, expectError: false, expectSketch: false},
		{name: "scale 0 offset overflows", scale: 0, offset: 1023, expectError: true},
		{name: "scale 3 offset overflows", scale: 3, offset: 8192, expectError: true},
		{name: "scale 20 offset overflows", scale: 20, offset: 2000000000, expectError: true},
		// Representable points must keep working, including negative offsets whose
		// boundaries round down towards zero rather than overflowing.
		{name: "scale 0 offset zero", scale: 0, offset: 0, expectError: false, expectSketch: true},
		{name: "scale 4 offset zero", scale: 4, offset: 0, expectError: false, expectSketch: true},
		{name: "scale 0 negative offset", scale: 0, offset: -1024, expectError: false, expectSketch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := newExpHistogramMetrics(tt.scale, tt.offset, []uint64{1, 1, 1})
			dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
				ExponentialHistogram().DataPoints().At(0)

			err := checkExponentialHistogramBounds(dp, tt.scale, 1)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// The whole translation must not panic either way.
			tr := newTranslator(t, zap.NewNop())
			consumer := &mockFullConsumer{}
			require.NotPanics(t, func() {
				_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
				require.NoError(t, err)
			})

			if tt.expectSketch {
				assert.NotEmpty(t, consumer.sketches, "representable point should still produce a sketch")
			} else {
				assert.Empty(t, consumer.sketches, "point should be dropped")
			}

			// CreateDDSketchFromExponentialHistogramOfDuration shares the same
			// unbounded boundary computation and must not panic either. It scales every
			// boundary to nanoseconds, so it rejects at least as much as the check above.
			require.NotPanics(t, func() {
				sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, tt.scale, "s")
				if tt.expectError {
					assert.Error(t, err)
					assert.Nil(t, sketch)
				}
			})
		})
	}
}

func TestCheckExponentialHistogramBoundsZeroCountBuckets(t *testing.T) {
	// Trailing zero-count buckets are discarded by DenseStore.AddWithCount, so they
	// must not make an otherwise representable point fail the check.
	md := newExpHistogramMetrics(0, 1000, []uint64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		ExponentialHistogram().DataPoints().At(0)
	require.NoError(t, checkExponentialHistogramBounds(dp, 0, 1))

	// The negative buckets are checked as well.
	dp.Negative().SetOffset(2000)
	dp.Negative().BucketCounts().Append(1)
	require.Error(t, checkExponentialHistogramBounds(dp, 0, 1))
}

func TestMaxPopulatedIndex(t *testing.T) {
	tests := []struct {
		offset  int32
		counts  []uint64
		wantIdx int
		wantAny bool
	}{
		{offset: 0, counts: nil, wantIdx: 0, wantAny: false},
		{offset: 0, counts: []uint64{0, 0}, wantIdx: 0, wantAny: false},
		{offset: 0, counts: []uint64{1, 1, 1}, wantIdx: 2, wantAny: true},
		{offset: 5, counts: []uint64{1, 0, 0}, wantIdx: 5, wantAny: true},
		{offset: -10, counts: []uint64{0, 3}, wantIdx: -9, wantAny: true},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case%d", i), func(t *testing.T) {
			b := pmetric.NewExponentialHistogramDataPointBuckets()
			b.SetOffset(tt.offset)
			for _, c := range tt.counts {
				b.BucketCounts().Append(c)
			}
			idx, ok := maxPopulatedIndex(b)
			assert.Equal(t, tt.wantAny, ok)
			if tt.wantAny {
				assert.Equal(t, tt.wantIdx, idx)
			}
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationUnitOverflow covers the
// boundary overflow that only the duration path can hit: a bucket boundary that is
// representable on its own but overflows once scaled to nanoseconds.
func TestCreateDDSketchFromExponentialHistogramOfDurationUnitOverflow(t *testing.T) {
	// 2^1000 is finite, but 2^1000 * 1e9 (seconds to nanoseconds) is not.
	md := newExpHistogramMetrics(0, 1000, []uint64{1, 1, 1})
	dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		ExponentialHistogram().DataPoints().At(0)

	require.NoError(t, checkExponentialHistogramBounds(dp, 0, 1))
	require.Error(t, checkExponentialHistogramBounds(dp, 0, float64(time.Second)))

	require.NotPanics(t, func() {
		sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, 0, "s")
		assert.Error(t, err)
		assert.Nil(t, sketch)
	})

	// A nil datapoint keeps returning an empty sketch.
	sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(nil, 0, "s")
	require.NoError(t, err)
	require.NotNil(t, sketch)
}

// TestCreateDDSketchFromExponentialHistogramOfDurationUnderflowToZeroBin checks that
// boundaries too small for the mapping to index are counted in the zero bin instead
// of being passed to LogarithmicMapping.Index, where Log(0) = -Inf used to panic in
// DenseStore.
func TestCreateDDSketchFromExponentialHistogramOfDurationUnderflowToZeroBin(t *testing.T) {
	for _, half := range []string{"positive", "negative"} {
		t.Run(half, func(t *testing.T) {
			// base^math.MinInt32 underflows to 0 at any scale.
			md := newExpHistogramMetrics(0, 0, nil)
			dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
				ExponentialHistogram().DataPoints().At(0)
			buckets := dp.Positive()
			if half == "negative" {
				buckets = dp.Negative()
			}
			buckets.SetOffset(math.MinInt32)
			buckets.BucketCounts().Append(1)
			buckets.BucketCounts().Append(1)

			// The bounds check allows underflow: it is only the overflow side that
			// produces a non-finite boundary.
			require.NoError(t, checkExponentialHistogramBounds(dp, 0, float64(time.Second)))

			var sketch *ddsketch.DDSketch
			var err error
			require.NotPanics(t, func() {
				sketch, err = CreateDDSketchFromExponentialHistogramOfDuration(&dp, 0, "s")
			})
			require.NoError(t, err)
			require.NotNil(t, sketch)
			// Both observations are preserved, in the zero bin.
			assert.Equal(t, 2.0, sketch.GetCount())
			assert.Equal(t, 2.0, sketch.GetZeroCount())
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationDegenerateMapping checks the
// case where the clamped gamma sits so close to 1 that the mapping's multiplier
// explodes and its indexable range inverts. mapping.Index then returns an index far
// too large for DenseStore to allocate, which used to panic with
// "growslice: len out of range".
func TestCreateDDSketchFromExponentialHistogramOfDurationDegenerateMapping(t *testing.T) {
	// 2^(2^-52) is 1+1.5e-16, below the 1.01/0.99 clamp, so the clamp does not apply.
	// Scale 53 rounds gamma to exactly 1, which NewLogarithmicMappingWithGamma rejects.
	for _, scale := range []int32{40, 52} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			md := newExpHistogramMetrics(scale, 0, []uint64{1, 1})
			dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
				ExponentialHistogram().DataPoints().At(0)

			// The boundaries themselves are perfectly representable.
			require.NoError(t, checkExponentialHistogramBounds(dp, scale, float64(time.Second)))

			require.NotPanics(t, func() {
				sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, scale, "s")
				assert.Error(t, err)
				assert.Nil(t, sketch)
			})
		})
	}

	// In-spec scales are unaffected: their indexable range still covers nanosecond
	// magnitudes.
	for _, scale := range []int32{-4, 0, 10, 20} {
		t.Run(fmt.Sprintf("inspec_scale%d", scale), func(t *testing.T) {
			md := newExpHistogramMetrics(scale, 0, []uint64{1, 1})
			dp := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
				ExponentialHistogram().DataPoints().At(0)
			sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, scale, "s")
			require.NoError(t, err)
			require.NotNil(t, sketch)
			assert.Equal(t, 2.0, sketch.GetCount())
		})
	}
}
