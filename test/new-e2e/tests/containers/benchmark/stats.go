// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"math"

	"github.com/samber/lo"
)

// sampleVariance returns the unbiased variance of values, of mean m.
func sampleVariance(values []float64, m float64) float64 {
	return lo.SumBy(values, func(v float64) float64 { return (v - m) * (v - m) }) / float64(len(values)-1)
}

// meanOfMeansTTest returns the two-sided p-value of the t-test of the hypothesis that the
// mean e of the means mₖ of K independent samples, of at least 2 values each, is 0.
//
// The variance of e is Σ vₖ / K², vₖ = sₖ² / nₖ being the squared standard error of mₖ, so
// t = e / SE(e) = Σ mₖ / √(Σ vₖ), with the Welch–Satterthwaite degrees of freedom
// (Σ vₖ)² / Σ (vₖ² / (nₖ − 1)). For two samples x and y, these are the statistic and the degrees
// of freedom of Welch's t-test comparing -x to y.
func meanOfMeansTTest(samples ...[]float64) float64 {
	// Σ mₖ, Σ vₖ and Σ (vₖ² / (nₖ − 1))
	var sum, sumV, dfDenominator float64
	for _, sample := range samples {
		mean := lo.Mean(sample)
		v := sampleVariance(sample, mean) / float64(len(sample))
		sum += mean
		sumV += v
		dfDenominator += v * v / float64(len(sample)-1)
	}
	if sumV == 0 {
		if sum == 0 {
			return 1
		}
		return 0
	}
	return studentTwoSidedPValue(sum/math.Sqrt(sumV), sumV*sumV/dfDenominator)
}

// studentTwoSidedPValue returns P(|T| >= |t|) for T following Student's t-distribution with
// df degrees of freedom.
func studentTwoSidedPValue(t, df float64) float64 {
	return regularizedIncompleteBeta(df/(df+t*t), df/2, 0.5)
}

// regularizedIncompleteBeta returns I_x(a, b), evaluated with its continued fraction
// (Numerical Recipes, 6.4), which converges quickly for x < (a+1)/(a+b+2) and is used
// through the symmetry I_x(a, b) = 1 - I_{1-x}(b, a) otherwise.
func regularizedIncompleteBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	lgammaA, _ := math.Lgamma(a)
	lgammaB, _ := math.Lgamma(b)
	lgammaAB, _ := math.Lgamma(a + b)
	front := math.Exp(a*math.Log(x) + b*math.Log1p(-x) + lgammaAB - lgammaA - lgammaB)
	if x < (a+1)/(a+b+2) {
		return front * betaContinuedFraction(x, a, b) / a
	}
	return 1 - front*betaContinuedFraction(1-x, b, a)/b
}

// betaContinuedFraction evaluates the continued fraction of the incomplete beta function
// with the modified Lentz's method.
func betaContinuedFraction(x, a, b float64) float64 {
	const (
		maxIterations = 300
		epsilon       = 1e-15
		tiny          = 1e-300
	)
	// Lentz's method divides by the convergents, which must not be zero.
	nonZero := func(v float64) float64 {
		if math.Abs(v) < tiny {
			return tiny
		}
		return v
	}

	c := 1.0
	d := 1 / nonZero(1-(a+b)*x/(a+1))
	h := d
	for m := 1.0; m <= maxIterations; m++ {
		// Even step
		numerator := m * (b - m) * x / ((a + 2*m - 1) * (a + 2*m))
		d = 1 / nonZero(1+numerator*d)
		c = nonZero(1 + numerator/c)
		h *= d * c
		// Odd step
		numerator = -(a + m) * (a + b + m) * x / ((a + 2*m) * (a + 2*m + 1))
		d = 1 / nonZero(1+numerator*d)
		c = nonZero(1 + numerator/c)
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < epsilon {
			break
		}
	}
	return h
}
