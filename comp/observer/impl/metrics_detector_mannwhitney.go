// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// MannWhitneyDetector uses the Mann-Whitney U test as a sliding-window
// changepoint detector. For each candidate split point it compares the
// "before" and "after" windows using the rank-based U statistic and
// picks the split with the lowest p-value.
//
// Reference: Mann & Whitney (1947). Non-parametric two-sample test.
//
// Key properties:
//   - Non-parametric: no Gaussian assumption
//   - Rank-based, robust to outliers
//   - Distribution-free under H0
//   - Score output: -log10(p_value) as confidence
//
// Precision-focused: uses multiple layered filters (statistical significance,
// effect size, deviation sigma, and relative change) to ensure only
// practically meaningful changepoints are reported.
type MannWhitneyDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 50
	MinPoints int

	// WindowSize is the number of points in each half-window (before/after).
	// Larger windows require more sustained shifts, reducing false positives.
	// Default: 60
	WindowSize int

	// SignificanceThreshold is the p-value below which a changepoint is flagged.
	// Default: 1e-12
	SignificanceThreshold float64

	// MinEffectSize is the minimum |rank-biserial correlation| (effect size)
	// required to report. Filters out statistically significant but tiny shifts.
	// Default: 0.95
	MinEffectSize float64

	// MinDeviationSigma is the minimum |median_after - median_before| / MAD_before.
	// Uses robust statistics (median/MAD) for outlier resistance.
	// Default: 3.0 (lowered from 5.0 to detect TP metrics with 3-4σ shifts)
	MinDeviationSigma float64

	// MinRelativeChange is the minimum |mean_after - mean_before| / max(|mean_before|, 1e-6).
	// Ensures the shift represents a meaningful fraction of the signal level.
	// Default: 0.20 (20% change)
	MinRelativeChange float64

	// StepSize controls how many points to skip between candidate splits.
	// 1 = check every point. Higher = faster but coarser.
	// Default: 3
	StepSize int
}

// NewMannWhitneyDetector creates a MannWhitneyDetector with default settings.
func NewMannWhitneyDetector() *MannWhitneyDetector {
	return &MannWhitneyDetector{
		MinPoints:             50,
		WindowSize:            60,
		SignificanceThreshold: 1e-12,
		MinEffectSize:         0.95,
		MinDeviationSigma:     3.0,
		MinRelativeChange:     0.20,
		StepSize:              3,
	}
}

// Name returns the detector name.
func (m *MannWhitneyDetector) Name() string {
	return "mannwhitney_detector"
}

// Detect runs the Mann-Whitney sliding window changepoint detection on the series.
// It reports at most one changepoint: the split with the lowest p-value.
// Multiple layered filters ensure only practically significant shifts are reported.
func (m *MannWhitneyDetector) Detect(series observer.Series) observer.MetricsDetectionResult {
	minPoints := m.MinPoints
	if minPoints <= 0 {
		minPoints = 50
	}
	windowSize := m.WindowSize
	if windowSize <= 0 {
		windowSize = 60
	}
	sigThreshold := m.SignificanceThreshold
	if sigThreshold <= 0 {
		sigThreshold = 1e-12
	}
	minEffect := m.MinEffectSize
	if minEffect <= 0 {
		minEffect = 0.95
	}
	minDevSigma := m.MinDeviationSigma
	if minDevSigma <= 0 {
		minDevSigma = 3.0
	}
	minRelChange := m.MinRelativeChange
	if minRelChange < 0 {
		minRelChange = 0.20
	}
	stepSize := m.StepSize
	if stepSize <= 0 {
		stepSize = 3
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.MetricsDetectionResult{}
	}

	// Adaptive window: use min of configured window and what fits
	maxWindow := (n - 1) / 2
	if windowSize > maxWindow {
		windowSize = maxWindow
	}
	if windowSize < 10 {
		return observer.MetricsDetectionResult{}
	}

	bestPValue := 1.0
	bestSplit := -1
	bestU := 0.0
	bestEffect := 0.0

	// Slide split point from windowSize to n-windowSize
	for t := windowSize; t <= n-windowSize; t += stepSize {
		before := extractValues(series.Points[t-windowSize : t])
		after := extractValues(series.Points[t : t+windowSize])

		u, pValue := mannWhitneyU(before, after)

		if pValue < bestPValue {
			effectSize := rankBiserialCorrelation(u, len(before), len(after))
			bestPValue = pValue
			bestSplit = t
			bestU = u
			bestEffect = effectSize
		}
	}

	// Filter 1: statistical significance
	if bestSplit < 0 || bestPValue >= sigThreshold {
		return observer.MetricsDetectionResult{}
	}

	// Filter 2: effect size (rank-biserial correlation)
	if math.Abs(bestEffect) < minEffect {
		return observer.MetricsDetectionResult{}
	}

	// Compute baseline and after stats using robust statistics
	beforeStart := bestSplit - windowSize
	if beforeStart < 0 {
		beforeStart = 0
	}
	beforeVals := extractValues(series.Points[beforeStart:bestSplit])
	afterVals := extractValues(series.Points[bestSplit : bestSplit+windowSize])

	beforeMedian := detectorMedian(beforeVals)
	beforeMAD := detectorMAD(beforeVals, beforeMedian, true) // scaled for σ-deviation comparison
	afterMedian := detectorMedian(afterVals)

	// Also compute means for relative change check
	baselineMean := mean(series.Points[beforeStart:bestSplit])
	baselineStddev := sampleStddev(series.Points[beforeStart:bestSplit], baselineMean)
	afterMean := detectorMeanValues(afterVals)

	// Filter 3: robust deviation check using median/MAD
	deviation := 0.0
	if beforeMAD > 1e-10 {
		deviation = (afterMedian - beforeMedian) / beforeMAD
	} else if math.Abs(beforeMedian) > 1e-10 {
		// For constant baselines, use relative change scaled to sigma-like units
		deviation = (afterMedian - beforeMedian) / (math.Abs(beforeMedian) * 0.05)
	}

	if math.Abs(deviation) < minDevSigma {
		return observer.MetricsDetectionResult{}
	}

	// Filter 4: minimum relative change
	absBaseline := math.Abs(baselineMean)
	if absBaseline < 1e-6 {
		absBaseline = 1e-6
	}
	relChange := math.Abs(afterMean-baselineMean) / absBaseline
	if relChange < minRelChange {
		return observer.MetricsDetectionResult{}
	}

	score := -math.Log10(bestPValue)
	if math.IsInf(score, 1) {
		score = 300.0 // cap for extremely small p-values
	}

	direction := "increased"
	if afterMean < baselineMean {
		direction = "decreased"
	}

	anomaly := observer.Anomaly{
		Source: observer.MetricName(series.Name),
		Title:  fmt.Sprintf("Mann-Whitney changepoint: %s", series.Name),
		Description: fmt.Sprintf("%s %s from %.2f to %.2f (p=%.2e, U=%.0f, effect=%.2f, %.1fσ, relΔ=%.1f%%)",
			series.Name, direction, baselineMean, afterMean, bestPValue, bestU, bestEffect, deviation, relChange*100),
		Tags:      series.Tags,
		Timestamp: series.Points[bestSplit].Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineStart:  series.Points[beforeStart].Timestamp,
			BaselineEnd:    series.Points[bestSplit-1].Timestamp,
			BaselineMean:   baselineMean,
			BaselineStddev: baselineStddev,
			Threshold:      sigThreshold,
			CurrentValue:   afterMean,
			DeviationSigma: deviation,
		},
	}

	return observer.MetricsDetectionResult{Anomalies: []observer.Anomaly{anomaly}}
}

// extractValues extracts float64 values from a slice of Points.
func extractValues(points []observer.Point) []float64 {
	vals := make([]float64, len(points))
	for i, p := range points {
		vals[i] = p.Value
	}
	return vals
}

// mannWhitneyU computes the Mann-Whitney U statistic and approximate p-value
// using the normal approximation with continuity correction and tie correction.
// Returns (U, p-value) where U is min(U1, U2).
func mannWhitneyU(x, y []float64) (float64, float64) {
	n1 := len(x)
	n2 := len(y)
	if n1 == 0 || n2 == 0 {
		return 0, 1.0
	}

	// Combine and rank
	type rankedValue struct {
		value float64
		group int // 0 = x, 1 = y
	}

	N := n1 + n2
	combined := make([]rankedValue, 0, N)
	for _, v := range x {
		combined = append(combined, rankedValue{value: v, group: 0})
	}
	for _, v := range y {
		combined = append(combined, rankedValue{value: v, group: 1})
	}

	sort.Slice(combined, func(i, j int) bool {
		return combined[i].value < combined[j].value
	})

	// Assign ranks with tie averaging
	ranks := make([]float64, N)
	tieCorrection := 0.0

	i := 0
	for i < N {
		j := i
		for j < N && combined[j].value == combined[i].value {
			j++
		}
		// Positions i..j-1 are tied; average rank = (i+1 + j) / 2
		avgRank := float64(i+1+j) / 2.0
		tieSize := float64(j - i)
		for k := i; k < j; k++ {
			ranks[k] = avgRank
		}
		// Tie correction term: sum of (t^3 - t) for each tie group
		tieCorrection += tieSize*tieSize*tieSize - tieSize
		i = j
	}

	// Sum ranks for group x
	var R1 float64
	for k := 0; k < N; k++ {
		if combined[k].group == 0 {
			R1 += ranks[k]
		}
	}

	// U statistics
	fn1 := float64(n1)
	fn2 := float64(n2)
	U1 := R1 - fn1*(fn1+1)/2
	U2 := fn1*fn2 - U1

	U := math.Min(U1, U2)

	// Normal approximation
	meanU := fn1 * fn2 / 2
	fN := float64(N)
	// Variance with tie correction
	varU := (fn1 * fn2 / 12) * (fN + 1 - tieCorrection/(fN*(fN-1)))
	if varU <= 0 {
		return U, 1.0
	}
	stdU := math.Sqrt(varU)

	// Z-score with continuity correction
	z := (math.Abs(U-meanU) - 0.5) / stdU
	if z < 0 {
		z = 0
	}

	// Two-tailed p-value using normal CDF approximation
	pValue := 2 * normalCDFUpper(z)
	if pValue > 1.0 {
		pValue = 1.0
	}

	return U, pValue
}

// rankBiserialCorrelation computes the rank-biserial correlation coefficient
// as an effect size measure. Range: [-1, 1].
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
