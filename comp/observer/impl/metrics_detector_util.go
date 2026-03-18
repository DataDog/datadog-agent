// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"
	"strings"
)

// detectorMedian computes the median of a float64 slice without modifying the input.
func detectorMedian(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// detectorMAD computes the Median Absolute Deviation from a given median.
// MAD = median(|x_i - median|).
// When scaleToSigma is true, the result is scaled by 1.4826 to estimate the
// standard deviation for normally distributed data. Use scaleToSigma=true when
// comparing against sigma-based thresholds (e.g. Mann-Whitney's deviation check),
// and false when using raw MAD as a denominator for relative change scores (e.g. TopK).
func detectorMAD(vals []float64, median float64, scaleToSigma bool) float64 {
	if len(vals) == 0 {
		return 0
	}
	absDevs := make([]float64, len(vals))
	for i, v := range vals {
		absDevs[i] = math.Abs(v - median)
	}
	sort.Float64s(absDevs)
	n := len(absDevs)
	var mad float64
	if n%2 == 0 {
		mad = (absDevs[n/2-1] + absDevs[n/2]) / 2
	} else {
		mad = absDevs[n/2]
	}
	if scaleToSigma {
		mad *= 1.4826
	}
	return mad
}

// detectorMeanValues computes the arithmetic mean of a float64 slice.
func detectorMeanValues(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// detectorSampleStddev computes the sample standard deviation of a float64 slice.
func detectorSampleStddev(vals []float64, mean float64) float64 {
	n := len(vals)
	if n < 2 {
		return 0
	}
	var sumSq float64
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n-1))
}

// detectorHasServiceTag checks whether any of the tags is a service: tag.
func detectorHasServiceTag(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "service:") {
			return true
		}
	}
	return false
}

// rankBiserialCorrelation computes the rank-biserial correlation from a Mann-Whitney U statistic.
// Used by ScanMW and ScanWelch for effect size verification.
func rankBiserialCorrelation(u float64, n1, n2 int) float64 {
	fn1 := float64(n1)
	fn2 := float64(n2)
	product := fn1 * fn2
	if product == 0 {
		return 0
	}
	return 1 - 2*u/product
}

// normalCDFUpper computes P(Z > z) for z >= 0 using the Abramowitz & Stegun approximation.
// Used by ScanMW and ScanWelch for p-value computation.
func normalCDFUpper(z float64) float64 {
	if z < 0 {
		return 1 - normalCDFUpper(-z)
	}
	// Rational approximation (Abramowitz & Stegun 26.2.17)
	const (
		p  = 0.2316419
		b1 = 0.319381530
		b2 = -0.356563782
		b3 = 1.781477937
		b4 = -1.821255978
		b5 = 1.330274429
	)
	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t
	phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
	return phi * (b1*t + b2*t2 + b3*t3 + b4*t4 + b5*t5)
}
