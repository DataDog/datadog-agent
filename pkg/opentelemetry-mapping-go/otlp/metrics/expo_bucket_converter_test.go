// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"fmt"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateDDSketchFromExponentialHistogramOfDurationOutOfRangeBounds covers
// the duration conversion, which feeds every boundary to
// LogarithmicMapping.Index directly. Index does not validate its argument, so
// an out-of-range boundary panics in the dense store on every architecture.
func TestCreateDDSketchFromExponentialHistogramOfDurationOutOfRangeBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scale   int32
		offset  int32
		unit    string
		wantErr bool
	}{
		// gamma overflows here, so base is +Inf and every index above zero lands on
		// +Inf. Index zero does not: base^0 is one whatever base is, leaving the
		// unit itself, and the mapping is usable because gamma gets clamped.
		{name: "minimum scale", scale: -10, offset: 1, unit: "s", wantErr: true},
		{name: "minimum scale at index zero", scale: -10, offset: 0, unit: "s", wantErr: false},
		{name: "index 32 at scale -5", scale: -5, offset: 32, unit: "s", wantErr: true},
		// The mapping can index up to ~1.26e308 ns, and rebasing to nanoseconds
		// spends log2(1e9) ~= 29.9 exponent bits, so at scale 0 the last index that
		// survives is 993. 2^994 is a perfectly good float64; 2^994 * 1e9 is not.
		{name: "unit scaling pushes the boundary out of range", scale: 0, offset: 994, unit: "s", wantErr: true},
		{name: "same index without the scaling", scale: 0, offset: 994, unit: "ns", wantErr: false},
		{name: "last index that survives the scaling", scale: 0, offset: 993, unit: "s", wantErr: false},
		{name: "representable", scale: 0, offset: 0, unit: "s", wantErr: false},
		{name: "representable at a fine scale", scale: 6, offset: 0, unit: "ms", wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dp := expoHistDataPoint(tc.scale, false, tc.offset, []uint64{1}, 0)

			var err error
			require.NotPanics(t, func() {
				_, err = CreateDDSketchFromExponentialHistogramOfDuration(&dp, tc.scale, tc.unit)
			})

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationUnderflowToZeroBin
// asserts that boundaries too small for the mapping to tell apart from zero are
// kept in the sketch's zero bin rather than dropped or fed to Index, where
// log(0) would become an extreme index. Both halves are covered.
func TestCreateDDSketchFromExponentialHistogramOfDurationUnderflowToZeroBin(t *testing.T) {
	for _, negative := range []bool{false, true} {
		t.Run(fmt.Sprintf("negative=%v", negative), func(t *testing.T) {
			// 2^-1052 * 1e9 is about 2.1e-308, below the 4.5e-308 the mapping can
			// index at this scale, yet still a non-zero float64.
			dp := expoHistDataPoint(0, negative, -1052, []uint64{3}, 2)

			var (
				sketch *ddsketch.DDSketch
				err    error
			)
			require.NotPanics(t, func() {
				sketch, err = CreateDDSketchFromExponentialHistogramOfDuration(&dp, 0, "s")
			})
			require.NoError(t, err)
			require.NotNil(t, sketch)
			// The 3 underflowing observations join the 2 that were already at zero.
			assert.Equal(t, 5.0, sketch.GetCount())
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationMixedHalves asserts that
// an out-of-range boundary in one half rejects the data point even when the
// other half converts cleanly, and that the error says which half it was.
func TestCreateDDSketchFromExponentialHistogramOfDurationMixedHalves(t *testing.T) {
	dp := expoHistDataPoint(0, false, 0, []uint64{1, 1}, 0)
	dp.Negative().SetOffset(995)
	dp.Negative().BucketCounts().Append(1)
	dp.SetCount(3)

	var err error
	require.NotPanics(t, func() {
		_, err = CreateDDSketchFromExponentialHistogramOfDuration(&dp, 0, "s")
	})
	assert.ErrorContains(t, err, "negative buckets")
}

// TestCreateDDSketchFromExponentialHistogramOfDurationDegenerateMapping covers
// scales fine enough that the mapping's indexable range inverts, leaving no
// value it can represent. Folding every observation into the zero bin would be
// silently wrong, so the point is rejected.
func TestCreateDDSketchFromExponentialHistogramOfDurationDegenerateMapping(t *testing.T) {
	// The indexable range inverts between scale 30 and 31 for a nanosecond offset.
	for _, scale := range []int32{31, 34} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			dp := expoHistDataPoint(scale, false, 0, []uint64{1, 1}, 0)

			var err error
			require.NotPanics(t, func() {
				_, err = CreateDDSketchFromExponentialHistogramOfDuration(&dp, scale, "s")
			})
			assert.Error(t, err)
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationInSpecScales asserts the
// new checks leave well-formed input alone: every scale go-expohisto can
// produce still converts and keeps its observations.
func TestCreateDDSketchFromExponentialHistogramOfDurationInSpecScales(t *testing.T) {
	for _, scale := range []int32{-9, -4, 0, 6, 15, 20} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			dp := expoHistDataPoint(scale, false, 0, []uint64{1, 1}, 0)

			sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, scale, "s")
			require.NoError(t, err)
			require.NotNil(t, sketch)
			assert.Equal(t, 2.0, sketch.GetCount())
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationZeroCountOnly asserts a
// data point with no populated bucket converts at any scale, including scales
// that would be rejected as soon as a bucket needed the mapping.
func TestCreateDDSketchFromExponentialHistogramOfDurationZeroCountOnly(t *testing.T) {
	for _, scale := range []int32{-10, 0, 20, 34} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			dp := expoHistDataPoint(scale, false, 0, nil, 4)

			sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, scale, "s")
			require.NoError(t, err)
			require.NotNil(t, sketch)
			assert.Equal(t, 4.0, sketch.GetZeroCount())
		})
	}
}

// TestCreateDDSketchFromExponentialHistogramOfDurationEmptyBuckets asserts that
// buckets with a zero count are never handed to the mapping: they carry no
// observation, so an unrepresentable boundary among them is harmless.
func TestCreateDDSketchFromExponentialHistogramOfDurationEmptyBuckets(t *testing.T) {
	dp := expoHistDataPoint(0, false, 1025, []uint64{0, 0, 0, 0}, 1)

	sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, 0, "ns")
	require.NoError(t, err)
	require.NotNil(t, sketch)
	assert.Equal(t, 1.0, sketch.GetCount())
}
