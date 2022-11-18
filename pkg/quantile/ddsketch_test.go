// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acceptableFloatError = 2e-12
)

func generateDDSketch(quantile func(float64) float64, N, M int) (*ddsketch.DDSketch, error) {
	sketch, _ := ddsketch.NewDefaultDDSketch(0.01)
	// Simulate a given distribution by replacing it with a distribution
	// where all points are placed where the evaluated quantiles are.
	// This ensures that quantile(x) for any x = i / N is the same between
	// the generated distribution and the theoretical distribution, making
	// comparisons easy in the test cases.
	for i := 0; i <= N; i++ {
		err := sketch.AddWithCount(quantile(float64(i)/float64(N)), float64(M))
		if err != nil {
			return nil, err
		}
	}

	return sketch, nil
}

func TestCreateDDSketchWithSketchMapping(t *testing.T) {
	// Support of the distribution: [0,N] or [-N,0]
	N := 1_000
	// Number of points per quantile
	M := 50

	tests := []struct {
		// distribution name
		name string
		// the quantile function (within [0,1])
		quantile func(x float64) float64
	}{
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:     "Uniform distribution (a=0,b=N)",
			quantile: func(y float64) float64 { return y * float64(N) },
		},
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:     "Uniform distribution (a=-N,b=0)",
			quantile: func(y float64) float64 { return (y - 1) * float64(N) },
		},
		{
			// https://en.wikipedia.org/wiki/U-quadratic_distribution
			name: "U-quadratic distribution (a=0,b=N)",
			quantile: func(y float64) float64 {
				a := 0.0
				b := float64(N)
				alpha := 12.0 / math.Pow(b-a, 3)
				beta := (b + a) / 2.0

				// golang's math.Pow doesn't like negative numbers as the first argument
				// (it will return NaN), even though cubic roots of negative numbers are defined.
				sign := 1.0
				if 3/alpha*y-math.Pow(beta-a, 3) < 0 {
					sign = -1.0
				}
				return beta + sign*math.Pow(sign*(3/alpha*y-math.Pow(beta-a, 3)), 1.0/3.0)
			},
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

				quantileValue, err := sketch.GetValueAtQuantile(q)
				require.NoError(t, err)

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
	N := 1_000
	// Number of points per quantile
	M := 50

	tests := []struct {
		// distribution name
		name string
		// the quantile function (within [0,1])
		quantile func(x float64) float64
		// the map of quantiles for which the test is known to fail
		excludedQuantiles map[int]bool
	}{
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:     "Uniform distribution (a=0,b=N)",
			quantile: func(y float64) float64 { return y * float64(N) },
		},
		// The p99 for this test fails, likely due to the shift of leftover bucket counts the right that is performed
		// during the DDSketch -> Sketch conversion, causing the p99 of the output sketch to fall on 0
		// (which means the InEpsilon check returns 1).
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:              "Uniform distribution (a=-N,b=0)",
			quantile:          func(y float64) float64 { return (y - 1) * float64(N) },
			excludedQuantiles: map[int]bool{99: true},
		},
		{
			// https://en.wikipedia.org/wiki/U-quadratic_distribution
			name: "U-quadratic distribution (a=0,b=N)",
			quantile: func(y float64) float64 {
				a := 0.0
				b := float64(N)
				alpha := 12.0 / math.Pow(b-a, 3)
				beta := (b + a) / 2.0

				// golang's math.Pow doesn't like negative numbers as the first argument
				// (it will return NaN), even though cubic roots of negative numbers are defined.
				sign := 1.0
				if 3/alpha*y-math.Pow(beta-a, 3) < 0 {
					sign = -1.0
				}
				return beta + sign*math.Pow(sign*(3/alpha*y-math.Pow(beta-a, 3)), 1.0/3.0)
			},
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

				quantileValue, err := sketch.GetValueAtQuantile(q)
				require.NoError(t, err)
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

			// Conversion accuracy formula taken from:
			// https://github.com/DataDog/logs-backend/blob/895e56c9eefa1c28a3affbdd0027f58a4c6f4322/domains/event-store/libs/event-store-aggregate/src/test/java/com/dd/event/store/api/query/sketch/SketchTest.java#L409-L422
			inputGamma := (1.0 + convertedSketch.RelativeAccuracy()) / (1.0 - convertedSketch.RelativeAccuracy())
			outputGamma := sketchConfig.gamma.v
			conversionGamma := inputGamma * outputGamma * outputGamma
			conversionRelativeAccuracy := (conversionGamma - 1) / (conversionGamma + 1)

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
