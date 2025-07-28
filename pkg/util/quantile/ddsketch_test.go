// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package quantile

import (
	"fmt"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest"
)

const (
	acceptableFloatError = 2e-11
)

func generateDDSketch(quantile sketchtest.QuantileFunction, N, M int) (*ddsketch.DDSketch, error) {
	sketch, _ := ddsketch.NewDefaultDDSketch(0.01)
	// Simulate a given distribution by replacing it with a distribution
	// where all points are placed where the evaluated quantiles are.
	// This ensures that quantile(x) for any x = i / N is the same between
	// the generated distribution and the theoretical distribution, making
	// comparisons easy in the test cases.
	for i := 0; i <= N; i++ {
		err := sketch.AddWithCount(quantile(float64(i)/float64(N)), float64(M))
		if err != nil {
			return nil, fmt.Errorf("failed to add quantile %g (%f): %w", float64(i)/float64(N), quantile(float64(i)/float64(N)), err)
		}
	}

	return sketch, nil
}

func TestCreateDDSketchWithSketchMapping(t *testing.T) {
	// Support of the distribution: [0,N] or [-N,0]
	N := 1_000.0
	// Number of points per quantile
	M := 50

	tests := []struct {
		// distribution name
		name string
		// the quantile function (within [0,1])
		quantile sketchtest.QuantileFunction
	}{
		{
			name:     "Uniform distribution (a=0,b=N)",
			quantile: sketchtest.UniformQ(0, N),
		},
		{
			name:     "Uniform distribution (a=-N,b=0)",
			quantile: sketchtest.UniformQ(-N, 0),
		},
		{
			name:     "Uniform distribution (a=-N,b=N)",
			quantile: sketchtest.UniformQ(-N, N),
		},
		{
			name:     "U-quadratic distribution (a=0,b=N)",
			quantile: sketchtest.UQuadraticQ(0, N),
		},
		{
			name:     "U-quadratic distribution (a=-N,b=0)",
			quantile: sketchtest.UQuadraticQ(-N, 0),
		},
		{
			name:     "U-quadratic distribution (a=-N/2,b=N/2)",
			quantile: sketchtest.UQuadraticQ(-N/2, N/2),
		},
		{
			name:     "Truncated Exponential distribution (a=0,b=N,lambda=1/100)",
			quantile: sketchtest.TruncateQ(0, N, sketchtest.ExponentialQ(1.0/100), sketchtest.ExponentialCDF(1.0/100)),
		},
		{
			name:     "Truncated Normal distribution (a=-8,b=8,mu=0, sigma=1e-3)",
			quantile: sketchtest.TruncateQ(-8, 8, sketchtest.NormalQ(0, 1e-3), sketchtest.NormalCDF(0, 1e-3)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sketch, err := generateDDSketch(test.quantile, 100, M)
			require.NoError(t, err)

			// Check the count of the sketch
			assert.Equal(
				t,
				float64(101*M),
				sketch.GetCount(),
			)

			// Check that the quantiles of the input sketch do match
			// the input distribution's quantiles
			for i := 0; i <= 100; i++ {
				q := (float64(i)) / 100.0
				expectedValue := test.quantile(q)

				quantileValue, qerr := sketch.GetValueAtQuantile(q)
				require.NoError(t, qerr)

				// Test that the quantile value returned by the sketch is vithin the relative accuracy
				// of the expected quantile value
				if expectedValue == 0.0 {
					// If the expected value is 0, we can't use InEpsilon, so we directly check that
					// the value is equal (within a small float precision error margin).
					assert.InDelta(
						t,
						expectedValue,
						quantileValue,
						acceptableFloatError,
					)
				} else {
					assert.InEpsilon(t,
						expectedValue,
						quantileValue,
						sketch.RelativeAccuracy(),
						fmt.Sprintf("error too high for p%d", i),
					)
				}
			}

			sketchConfig := Default()
			convertedSketch, err := createDDSketchWithSketchMapping(sketchConfig, sketch)
			require.NoError(t, err)

			// Conversion accuracy formula taken from:
			// https://github.com/DataDog/logs-backend/blob/895e56c9eefa1c28a3affbdd0027f58a4c6f4322/domains/event-store/libs/event-store-aggregate/src/test/java/com/dd/event/store/api/query/sketch/SketchTest.java#L409-L422
			inputGamma := (1.0 + sketch.RelativeAccuracy()) / (1.0 - sketch.RelativeAccuracy())
			outputGamma := (1.0 + convertedSketch.RelativeAccuracy()) / (1.0 - convertedSketch.RelativeAccuracy())
			conversionGamma := inputGamma * outputGamma * outputGamma
			conversionRelativeAccuracy := (conversionGamma - 1) / (conversionGamma + 1)

			// Check the count of the converted sketch
			assert.InDelta(
				t,
				float64(101*M),
				convertedSketch.GetCount(),
				acceptableFloatError,
			)

			// Check that the quantiles of the converted sketch
			// approximately match the input distribution's quantiles
			for i := 0; i <= 100; i++ {
				q := (float64(i)) / 100.0
				expectedValue, err := sketch.GetValueAtQuantile(q)
				require.NoError(t, err)

				quantileValue, err := convertedSketch.GetValueAtQuantile(q)
				require.NoError(t, err)

				// Test that the quantile value returned by the sketch is vithin the relative accuracy
				// of the expected value
				if expectedValue == 0.0 {
					// If the expected value is 0, we can't use InEpsilon, so we directly check that
					// the value is equal (within a small float precision error margin).
					assert.InDelta(
						t,
						expectedValue,
						quantileValue,
						acceptableFloatError,
					)
				} else {
					assert.InEpsilon(t,
						expectedValue,
						quantileValue,
						conversionRelativeAccuracy,
						fmt.Sprintf("error too high for p%d", i),
					)
				}
			}
		})
	}
}

func TestConvertDDSketchIntoSketch(t *testing.T) {
	// Support of the distribution: [0,N] or [-N,0]
	N := 1_000.0
	// Number of points per quantile
	M := 50

	tests := []struct {
		// distribution name
		name string
		// the quantile function (within [0,1])
		quantile sketchtest.QuantileFunction
		// the map of quantiles for which the test is known to fail
		excludedQuantiles map[int]bool
	}{
		{
			name:     "Uniform distribution (a=0,b=N)",
			quantile: sketchtest.UniformQ(0, N),
		},
		{
			name:     "Uniform distribution (a=-N,b=0)",
			quantile: sketchtest.UniformQ(-N, 0),
		},
		{
			name:     "Uniform distribution (a=-N,b=N)",
			quantile: sketchtest.UniformQ(-N, N),
		},
		{
			name:     "U-quadratic distribution (a=0,b=N)",
			quantile: sketchtest.UQuadraticQ(0, N),
		},
		{
			name:     "U-quadratic distribution (a=-N/2,b=N/2)",
			quantile: sketchtest.UQuadraticQ(-N/2, N/2),
		},
		{
			name:     "U-quadratic distribution (a=-N,b=0)",
			quantile: sketchtest.UQuadraticQ(-N, 0),
		},
		{
			name:     "Truncated Exponential distribution (a=0,b=N,lambda=1/100)",
			quantile: sketchtest.TruncateQ(0, N, sketchtest.ExponentialQ(1.0/100), sketchtest.ExponentialCDF(1.0/100)),
		},
		{
			name:     "Truncated Normal distribution (a=0,b=8,mu=0, sigma=1e-3)",
			quantile: sketchtest.TruncateQ(0, 8, sketchtest.NormalQ(0, 1e-3), sketchtest.NormalCDF(0, 1e-3)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sketch, err := generateDDSketch(test.quantile, 100, M)
			require.NoError(t, err)

			// Check the count of the sketch
			assert.Equal(
				t,
				float64(101*M),
				sketch.GetCount(),
			)

			// Check that the quantiles of the input sketch do match
			// the input distribution's quantiles
			for i := 1; i <= 100; i++ {
				q := (float64(i)) / 100.0
				expectedValue := test.quantile(q)

				quantileValue, qerr := sketch.GetValueAtQuantile(q)
				require.NoError(t, qerr)
				// Test that the quantile value returned by the sketch is vithin the relative accuracy
				// of the expected quantile value
				if expectedValue == 0.0 {
					// If the expected value is 0, we can't use InEpsilon, so we directly check that
					// the value is equal (within a small float precision error margin).
					assert.InDelta(
						t,
						expectedValue,
						quantileValue,
						acceptableFloatError,
					)
				} else {
					assert.InEpsilon(t,
						expectedValue,
						quantileValue,
						sketch.RelativeAccuracy(),
						fmt.Sprintf("error too high for p%d", i),
					)
				}
			}

			sketchConfig := Default()
			convertedSketch, err := createDDSketchWithSketchMapping(sketchConfig, sketch)
			require.NoError(t, err)

			outputSketch, err := convertDDSketchIntoSketch(sketchConfig, convertedSketch)
			require.NoError(t, err)

			/* We compute the expected bound on the relative error between percentile values before
			 * and after the conversion.
			 *
			 * - The +0.5 in `createDDSketchWithSketchMapping` compensates for a bug in
			 *   `sketchConfig.key` which is not relevant here, creating a systematic bias of half
			 *   a bin.
			 * - The error on accumulated bin counts caused by the rounding process is bounded by 0.5
			 *   by design. In most circumstances, including the scenarios under test, this can
			 *   shift quantiles by one bin at most.
			 * - Finally, a bug in the interpolation in `Sketch.Quantile` means the output can be
			 *   off by another half a bin.
			 * (Unfortunately, other tests, as well as backend code, rely on these bugs, so they are
			 *  not easily fixed.)
			 * In total, the expected worst case relative error:
			 */
			conversionRelativeAccuracy := sketchConfig.gamma.v*sketchConfig.gamma.v - 1

			// Check the count of the output sketch
			assert.InDelta(
				t,
				convertedSketch.GetCount(),
				outputSketch.Basic.Cnt,
				acceptableFloatError,
			)

			// Check the minimum value of the output sketch
			expectedMinValue, err := convertedSketch.GetMinValue()
			require.NoError(t, err)
			assert.InDelta(
				t,
				expectedMinValue,
				outputSketch.Basic.Min,
				acceptableFloatError,
			)

			// Check the maximum value of the output sketch
			expectedMaxValue, err := convertedSketch.GetMaxValue()
			require.NoError(t, err)
			assert.InDelta(
				t,
				expectedMaxValue,
				outputSketch.Basic.Max,
				acceptableFloatError,
			)

			// Check that the quantiles of the output sketch do match
			// the quantiles of the DDSketch it comes from
			for i := 0; i <= 100; i++ {
				// Skip if quantile is excluded
				if test.excludedQuantiles[i] {
					continue
				}

				q := (float64(i)) / 100.0

				expectedValue, err := convertedSketch.GetValueAtQuantile(q)
				require.NoError(t, err)

				quantileValue := outputSketch.Quantile(sketchConfig, q)

				// Test that the quantile value returned by the sketch is vithin an acceptable
				// range of the expected value
				if expectedValue == 0.0 {
					// If the expected value is 0, we can't use InEpsilon, so we directly check that
					// the value is equal (within a small float precision error margin).
					assert.InDelta(
						t,
						expectedValue,
						quantileValue,
						acceptableFloatError,
					)
				} else {
					assert.InEpsilon(t,
						expectedValue,
						quantileValue,
						conversionRelativeAccuracy,
						fmt.Sprintf("error too high for p%d", i),
					)
				}
			}
		})
	}
}

// BenchmarkDDSketchConversion benchmarks the DDSketch to Sketch conversion
func BenchmarkDDSketchConversion(b *testing.B) {
	// Test with different data sizes to see how performance scales
	datasets := []struct {
		name      string
		numValues int
	}{
		{"Small", 1000},
		{"Medium", 10000},
		{"Large", 100000},
	}

	for _, dataset := range datasets {
		b.Run(dataset.name, func(b *testing.B) {
			// Create a DDSketch with the specified number of values
			sketch, err := ddsketch.NewDefaultDDSketch(0.01)
			if err != nil {
				b.Fatalf("Failed to create sketch: %v", err)
			}

			// Add values to the sketch (uniform distribution between 0 and 1000)
			for i := 0; i < dataset.numValues; i++ {
				err := sketch.Add(float64(i % 1000))
				if err != nil {
					b.Fatalf("Failed to add to sketch: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Convert the DDSketch to a Sketch
				_, err := ConvertDDSketchIntoSketch(sketch)
				if err != nil {
					b.Fatalf("Conversion failed: %v", err)
				}
			}
		})
	}
}
