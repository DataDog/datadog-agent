// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import "math"

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// sampleVariance returns the unbiased variance of values, of mean m.
func sampleVariance(values []float64, m float64) float64 {
	var sum float64
	for _, v := range values {
		sum += (v - m) * (v - m)
	}
	return sum / float64(len(values)-1)
}

// welchTTest returns the two-sided p-value of Welch's t-test of the hypothesis that the
// samples a and b, of at least 2 values each, come from distributions of equal means.
func welchTTest(a, b []float64) float64 {
	meanA, meanB := mean(a), mean(b)
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

// meanOfMeansTTest returns the two-sided p-value of the t-test of the hypothesis that
// (mean(x) + mean(y)) / 2 = 0, x and y being independent samples of at least 2 values each.
// Its variance being (var(x)/nx + var(y)/ny) / 4, the statistic and the Welch–Satterthwaite
// degrees of freedom are those of Welch's t-test comparing -x to y.
func meanOfMeansTTest(x, y []float64) float64 {
	negated := make([]float64, len(x))
	for i, v := range x {
		negated[i] = -v
	}
	return welchTTest(negated, y)
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
