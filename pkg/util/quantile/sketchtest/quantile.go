// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package sketchtest is an internal module with test helpers for generating points from a given distribution.
package sketchtest

import "math"

// QuantileFunction for a distribution. The function MUST be defined in (0,1).
// The function MAY return the minimum at 0 and the maximum at 1, for distributions where this
// makes sense.
type QuantileFunction func(float64) float64

// CDF is the cumulative distribution function of a distribution (inverse of QuantileFunction).
type CDF func(float64) float64

// UniformQ returns the quantile function for an uniform distribution
// with parameters a and b.
// See https://en.wikipedia.org/wiki/Continuous_uniform_distribution
func UniformQ(a, b float64) QuantileFunction {
	return func(q float64) float64 {
		return (b-a)*q + a
	}
}

// UQuadraticQ returns the quantile function for an U-Quadratic distribution
// with parameters a and b.
// See https://en.wikipedia.org/wiki/U-quadratic_distribution
func UQuadraticQ(a, b float64) QuantileFunction {
	return func(q float64) float64 {
		alpha := 12.0 / math.Pow(b-a, 3)
		beta := (b + a) / 2.0

		// golang's math.Pow doesn't like negative numbers as the first argument
		// (it will return NaN), even though cubic roots of negative numbers are defined.
		sign := 1.0
		if 3/alpha*q-math.Pow(beta-a, 3) < 0 {
			sign = -1.0
		}
		return beta + sign*math.Pow(sign*(3/alpha*q-math.Pow(beta-a, 3)), 1.0/3.0)
	}
}

// TruncateQ a quantile function to the interval [a,b] given its CDF.
// See https://en.wikipedia.org/wiki/Truncated_distribution.
// This function assumes but does not check that quantile is the inverse of cdf.
func TruncateQ(a, b float64, quantile QuantileFunction, cdf CDF) QuantileFunction {
	// inverse of x ↦ (x - F(a))/(F(b) - F(a))
	h := func(cdfx float64) float64 { return (cdf(b)-cdf(a))*cdfx + cdf(a) }

	return func(q float64) float64 {
		// handle extrema separately to support cdf not defined at these points.
		switch q {
		case 0:
			// quantile(h(0)) = quantile(cdf(a)) = a
			return a
		case 1:
			// quantile(h(1)) = quantile(cdf(b)) = b
			return b
		}
		return quantile(h(q))
	}
}

// TruncateCDF a CDF to the interval (a,b).
// See https://en.wikipedia.org/wiki/Truncated_distribution.
func TruncateCDF(a, b float64, cdf CDF) CDF {
	return func(x float64) float64 {
		return (cdf(x) - cdf(a)) / (cdf(b) - cdf(a))
	}
}

// ExponentialCDF is the CDF of the Exponential distribution
// with parameter lambda.
// See https://en.wikipedia.org/wiki/Exponential_distribution
func ExponentialCDF(lambda float64) CDF {
	return func(x float64) float64 {
		if x < 0 {
			return 0
		}
		return 1 - math.Exp(-lambda*x)
	}
}

// ExponentialQ is the quantile function of the Exponential distribution
// with parameter lambda
// See https://en.wikipedia.org/wiki/Exponential_distribution
func ExponentialQ(lambda float64) QuantileFunction {
	return func(q float64) float64 {
		return -math.Log(1-q) / lambda
	}
}

// NormalCDF is the CDF of the Normal distribution
// with parameters mu and sigma.
// See https://en.wikipedia.org/wiki/Normal_distribution
func NormalCDF(mu, sigma float64) CDF {
	return func(x float64) float64 {
		return 1.0 / 2.0 * (1 + math.Erf((x-mu)/(sigma*math.Sqrt2)))
	}
}

// NormalQ is the quantile function of the Normal distribution
// with parameters mu and sigma.
// See https://en.wikipedia.org/wiki/Normal_distribution
func NormalQ(mu, sigma float64) QuantileFunction {
	return func(q float64) float64 {
		return mu + sigma*math.Sqrt2*math.Erfinv(2*q-1)
	}
}
