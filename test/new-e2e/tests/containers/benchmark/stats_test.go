// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStudentTwoSidedPValue(t *testing.T) {
	// Reference values: closed forms for 1 and 2 degrees of freedom, numerical integration
	// of the density of the distribution otherwise.
	tests := []struct {
		t, df, p float64
	}{
		{t: 0, df: 5, p: 1},
		{t: 1, df: 1, p: 0.5},
		{t: -1, df: 1, p: 0.5},
		{t: 2, df: 2, p: 1 - 2/math.Sqrt(6)},
		{t: 2.228, df: 10, p: 0.050012},
		{t: 2.042, df: 30, p: 0.050029},
		{t: 0.5, df: 4.5, p: 0.640550},
		{t: 3.5, df: 7.3, p: 0.009343},
		{t: 50, df: 20, p: 0},
	}
	for _, tt := range tests {
		assert.InDeltaf(t, tt.p, studentTwoSidedPValue(tt.t, tt.df), 1e-6, "t=%g df=%g", tt.t, tt.df)
	}
}

func TestWelchTTest(t *testing.T) {
	tests := []struct {
		name string
		a, b []float64
		p    float64
	}{
		{
			// Examples of https://en.wikipedia.org/wiki/Welch%27s_t-test
			name: "equal sizes",
			a:    []float64{27.5, 21.0, 19.0, 23.6, 17.0, 17.9, 16.9, 20.1, 21.9, 22.6, 23.1, 19.6, 19.0, 21.7, 21.4},
			b:    []float64{27.1, 22.0, 20.8, 23.4, 23.4, 23.5, 25.8, 22.0, 24.8, 20.2, 21.9, 22.1, 22.9, 20.5, 24.4},
			p:    0.021378,
		},
		{
			name: "unequal sizes and variances",
			a:    []float64{17.2, 20.9, 22.6, 18.1, 21.7, 21.4, 23.5, 24.2, 14.7, 21.8},
			b:    []float64{21.5, 22.8, 21.0, 23.0, 21.6, 23.6, 22.5, 20.7, 23.4, 21.8, 20.7, 21.7, 21.5, 22.5, 23.6, 21.5, 22.5, 23.5, 21.5, 21.8},
			p:    0.148842,
		},
		{
			name: "identical constant samples",
			a:    []float64{3, 3, 3},
			b:    []float64{3, 3, 3},
			p:    1,
		},
		{
			name: "different constant samples",
			a:    []float64{3, 3, 3},
			b:    []float64{4, 4, 4},
			p:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.p, welchTTest(tt.a, tt.b), 1e-6)
			assert.InDelta(t, tt.p, welchTTest(tt.b, tt.a), 1e-6, "the test must be symmetric")

			// (mean(-a) + mean(b)) / 2 = 0 is the hypothesis of Welch's t-test of a and b.
			negated := make([]float64, len(tt.a))
			for i, v := range tt.a {
				negated[i] = -v
			}
			assert.InDelta(t, tt.p, meanOfMeansTTest(negated, tt.b), 1e-6)
		})
	}
}
