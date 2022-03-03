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
)

func generateDDSketch(cdf func(float64) float64, N int, M float64) *ddsketch.DDSketch {
	sketch, _ := ddsketch.NewDefaultDDSketch(0.01)
	for i := 0; i < N; i++ {
		sketch.AddWithCount(float64(i), (cdf(float64(i+1)) - cdf(float64(i))) * M)
	}

	return sketch
}

func TestGenerateDDSketch(t *testing.T) {
	N := 1_000
	M := 50_000.0

	tests := []struct {
		// distribution name
		name string
		// the cumulative distribution function (within [0,N])
		cdf func(x float64) float64
		// the quantile function (within [0,1])
		quantile func(x float64) float64
	}{
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:     "Uniform distribution (a=0,b=N)",
			cdf:      func(x float64) float64 { return x / float64(N) },
			quantile: func(x float64) float64 { return x * float64(N) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sketch := generateDDSketch(test.cdf, N, M)

			// Check the minimum is 0.0
			min, err := sketch.GetValueAtQuantile(0.0)
			assert.NoError(t, err)
			assert.Equal(t, 0.0, min)

			// Check the quantiles are approximately correct
			for i := 1; i <= 100; i++ {
				q := (float64(i)) / 100.0
				expectedValue := test.quantile(q)

				quantileValue, err := sketch.GetValueAtQuantile(q)
				assert.NoError(t, err)
				assert.InEpsilon(t,
					// test that the quantile value returned by the sketch is vithin the relative accuracy
					// of the expected value
					expectedValue,
					quantileValue,
					sketch.RelativeAccuracy(),
					fmt.Sprintf("error too high for p%d", i),
				)
			}
		})
	}
}