// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"math"
	"slices"
	"testing"

	"github.com/samber/lo"
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

// welchTTest returns the two-sided p-value of Welch's t-test of the hypothesis that the
// samples a and b, of at least 2 values each, come from distributions of equal means. It is the
// test the benchmark computed before it supported more than 2 phases, as welchTTest(-x, y) for
// the block log ratios x and y of its phases, kept as a reference.
func welchTTest(a, b []float64) float64 {
	meanA, meanB := lo.Mean(a), lo.Mean(b)
	// Squared standard errors of the means
	seA := sampleVariance(a, meanA) / float64(len(a))
	seB := sampleVariance(b, meanB) / float64(len(b))
	se := seA + seB
	if se == 0 {
		if meanA == meanB {
			return 1
		}
		return 0
	}

	t := (meanB - meanA) / math.Sqrt(se)
	// Welch–Satterthwaite degrees of freedom
	df := se * se / (seA*seA/float64(len(a)-1) + seB*seB/float64(len(b)-1))
	return studentTwoSidedPValue(t, df)
}

// negated returns the opposites of values.
func negated(values []float64) []float64 {
	return lo.Map(values, func(v float64, _ int) float64 { return -v })
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

			// (mean(-a) + mean(b)) / 2 = 0 is the hypothesis of Welch's t-test of a and b: with 2
			// samples, the p-value is the same to the bit.
			assert.Equal(t, welchTTest(tt.a, tt.b), meanOfMeansTTest(negated(tt.a), tt.b))
			assert.Equal(t, welchTTest(tt.b, tt.a), meanOfMeansTTest(negated(tt.b), tt.a))
		})
	}
}

func TestMeanOfMeansTTest(t *testing.T) {
	// Reference values: numerical integration of the density of Student's t-distribution.
	tests := []struct {
		name    string
		samples [][]float64
		p       float64
	}{
		{
			// Means 2, -1, 2 and -1, variances 1, 1, 4 and 4: the squared standard errors of the
			// means vₖ are 1/3, 1/3, 4/3 and 4/3, so t = 2 / √(10/3) and the Welch–Satterthwaite
			// degrees of freedom (10/3)² / (2 × (1/3)²/2 + 2 × (4/3)²/2) = 100/17 ≈ 5.88. With
			// Σ (nₖ - 1) = 8 degrees of freedom instead, the p-value would be 0.305201.
			name:    "4 samples of unequal variances",
			samples: [][]float64{{1, 2, 3}, {-2, -1, 0}, {0, 2, 4}, {-3, -1, 1}},
			p:       0.316133,
		},
		{
			// Equal variances and sizes: t = 2 / √(4/3), and Σ (nₖ - 1) = 8 degrees of freedom.
			name:    "4 samples of equal variances",
			samples: [][]float64{{1, 2, 3}, {-2, -1, 0}, {1, 2, 3}, {-2, -1, 0}},
			p:       0.121503,
		},
		{
			name:    "4 constant samples of means summing to 0",
			samples: [][]float64{{1, 1, 1}, {-1, -1, -1}, {2, 2, 2}, {-2, -2, -2}},
			p:       1,
		},
		{
			name:    "4 constant samples of means not summing to 0",
			samples: [][]float64{{1, 1, 1}, {-1, -1, -1}, {2, 2, 2}, {-1, -1, -1}},
			p:       0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.p, meanOfMeansTTest(tt.samples...), 1e-6)
			// The order of the samples doesn't matter, nor the sign of all the values.
			reversed := slices.Clone(tt.samples)
			slices.Reverse(reversed)
			assert.InDelta(t, tt.p, meanOfMeansTTest(reversed...), 1e-6)
			assert.InDelta(t, tt.p, meanOfMeansTTest(lo.Map(tt.samples, func(sample []float64, _ int) []float64 { return negated(sample) })...), 1e-6)
		})
	}
}
