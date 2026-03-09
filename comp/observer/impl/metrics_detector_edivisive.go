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

// EDivisiveDetector uses the E-Divisive algorithm (Matteson & James, 2014) to
// detect distributional changepoints in metric series. Unlike mean-based detectors,
// E-Divisive can catch variance shifts, shape changes, and other distributional
// changes using energy statistics.
//
// The detector computes the energy statistic for candidate split points across
// the series and reports the single most significant changepoint (highest energy).
//
// Precision-focused: uses multiple layered filters (energy significance, effect size,
// deviation sigma, and relative change) to ensure only practically meaningful
// changepoints are reported.
type EDivisiveDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 50
	MinPoints int

	// MinSegmentFraction is the minimum fraction of total points in each segment.
	// Default: 0.15 (at least 15% on each side)
	MinSegmentFraction float64

	// SignificanceThreshold is the minimum normalized energy statistic to report.
	// The energy is normalized by series stddev to make it scale-invariant.
	// Default: 40.0 (strict — energy must be 40x stddev to report)
	SignificanceThreshold float64

	// MinEffectSize is the minimum absolute mean shift in MAD units to report.
	// Even if energy is high, we suppress detections where the practical effect is tiny.
	// Default: 15.0 (very strict — 15 MADs required)
	MinEffectSize float64

	// MinDeviationSigma is the minimum |median_after - median_before| / MAD_before.
	// Uses robust statistics (median/MAD) for outlier resistance.
	// Default: 10.0 (strict — 10 MADs robust deviation required)
	MinDeviationSigma float64

	// MinRelativeChange is the minimum |mean_after - mean_before| / max(|mean_before|, 1e-6).
	// Ensures the shift represents a meaningful fraction of the signal level.
	// Default: 0.50 (50% change required)
	MinRelativeChange float64

	// SubsampleStep controls how many candidate split points to skip.
	// Default: 5
	SubsampleStep int

	// MinVarianceThreshold: skip series with very low coefficient of variation.
	// Default: 1e-6
	MinVarianceThreshold float64
}

// NewEDivisiveDetector creates an EDivisiveDetector with default settings.
func NewEDivisiveDetector() *EDivisiveDetector {
	return &EDivisiveDetector{
		MinPoints:             50,
		MinSegmentFraction:    0.15,
		SignificanceThreshold: 40.0,
		MinEffectSize:         15.0,
		MinDeviationSigma:     10.0,
		MinRelativeChange:     0.50,
		SubsampleStep:         5,
		MinVarianceThreshold:  1e-6,
	}
}

// Name returns the detector name.
func (d *EDivisiveDetector) Name() string {
	return "edivisive_detector"
}

// Detect runs the E-Divisive algorithm on the series and returns an anomaly at
// the most significant distributional changepoint, if one exceeds the threshold.
func (d *EDivisiveDetector) Detect(series observer.Series) observer.MetricsDetectionResult {
	minPoints := d.MinPoints
	if minPoints <= 0 {
		minPoints = 50
	}
	minSegFrac := d.MinSegmentFraction
	if minSegFrac <= 0 {
		minSegFrac = 0.15
	}
	sigThreshold := d.SignificanceThreshold
	if sigThreshold <= 0 {
		sigThreshold = 40.0
	}
	minEffect := d.MinEffectSize
	if minEffect <= 0 {
		minEffect = 15.0
	}
	minDevSigma := d.MinDeviationSigma
	if minDevSigma <= 0 {
		minDevSigma = 10.0
	}
	minRelChange := d.MinRelativeChange
	if minRelChange < 0 {
		minRelChange = 0.50
	}
	subsampleStep := d.SubsampleStep
	if subsampleStep <= 0 {
		subsampleStep = 5
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.MetricsDetectionResult{}
	}

	// Extract values for fast access
	values := make([]float64, n)
	for i, p := range series.Points {
		values[i] = p.Value
	}

	// Compute overall series statistics for normalization
	seriesMean := edMeanValues(values)
	seriesStddev := edStddevValues(values, seriesMean)

	// Early termination: skip constant or near-constant series
	if seriesStddev < d.MinVarianceThreshold {
		return observer.MetricsDetectionResult{}
	}

	// Compute MAD for robust effect size estimation
	seriesMedian := medianValues(values)
	seriesMAD := madValues(values, seriesMedian)
	if seriesMAD < 1e-10 {
		// If MAD is zero, use stddev-based threshold
		seriesMAD = seriesStddev * 0.6745 // approximate MAD from stddev for normal
	}
	if seriesMAD < 1e-10 {
		return observer.MetricsDetectionResult{}
	}

	// Minimum segment size
	minSegSize := int(float64(n) * minSegFrac)
	if minSegSize < 10 {
		minSegSize = 10
	}

	// Find the split point that maximizes the energy statistic
	bestSplit, bestEnergy := d.findBestSplit(values, minSegSize, subsampleStep)

	if bestSplit < 0 {
		return observer.MetricsDetectionResult{}
	}

	// Filter 1: Normalize energy by series stddev to make it scale-invariant
	normalizedEnergy := bestEnergy / seriesStddev

	if normalizedEnergy < sigThreshold {
		return observer.MetricsDetectionResult{}
	}

	// Compute segment statistics
	baselineVals := values[:bestSplit]
	postVals := values[bestSplit:]
	baselineMean := edMeanValues(baselineVals)
	postMean := edMeanValues(postVals)
	baselineStddev := edStddevValues(baselineVals, baselineMean)
	baselineMedian := medianValues(baselineVals)
	baselineMAD := madValues(baselineVals, baselineMedian)
	postMedian := medianValues(postVals)

	// Filter 2: effect size (MAD-normalized mean shift)
	effectSize := math.Abs(postMean-baselineMean) / seriesMAD

	// Also check variance ratio for variance-shift detection
	postStddev := edStddevValues(postVals, postMean)
	varianceRatio := 1.0
	if baselineStddev > 1e-10 {
		varianceRatio = postStddev / baselineStddev
	}

	// Require either a sufficient mean shift OR a sufficient variance change
	meanShiftOK := effectSize >= minEffect
	varianceShiftOK := varianceRatio > 3.0 || varianceRatio < 1.0/3.0

	if !meanShiftOK && !varianceShiftOK {
		return observer.MetricsDetectionResult{}
	}

	// Filter 3: robust deviation check using median/MAD (same as MW v3)
	deviation := 0.0
	if baselineMAD > 1e-10 {
		deviation = (postMedian - baselineMedian) / baselineMAD
	} else if math.Abs(baselineMedian) > 1e-10 {
		// For constant baselines, use relative change scaled to sigma-like units
		deviation = (postMedian - baselineMedian) / (math.Abs(baselineMedian) * 0.05)
	}

	if math.Abs(deviation) < minDevSigma {
		return observer.MetricsDetectionResult{}
	}

	// Filter 4: minimum relative change (same as MW v3)
	absBaseline := math.Abs(baselineMean)
	if absBaseline < 1e-6 {
		absBaseline = 1e-6
	}
	relChange := math.Abs(postMean-baselineMean) / absBaseline
	if relChange < minRelChange {
		return observer.MetricsDetectionResult{}
	}

	// Score: normalized energy
	score := normalizedEnergy

	// Compute stddev-based deviation for debug info
	stddevDeviation := 0.0
	if baselineStddev > 1e-10 {
		stddevDeviation = (postMean - baselineMean) / baselineStddev
	}

	debugInfo := &observer.AnomalyDebugInfo{
		BaselineStart:  series.Points[0].Timestamp,
		BaselineEnd:    series.Points[bestSplit-1].Timestamp,
		BaselineMean:   baselineMean,
		BaselineMedian: baselineMedian,
		BaselineStddev: baselineStddev,
		BaselineMAD:    baselineMAD,
		Threshold:      sigThreshold,
		CurrentValue:   postMean,
		DeviationSigma: stddevDeviation,
	}

	changeType := "distributional"
	if meanShiftOK && math.Abs(stddevDeviation) > 2.0 {
		changeType = "mean shift"
	} else if varianceShiftOK {
		changeType = "variance shift"
	}

	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.MetricName(series.Name),
		DetectorName: d.Name(),
		Title:        fmt.Sprintf("E-Divisive %s detected: %s", changeType, series.Name),
		Description: fmt.Sprintf("%s: %s at t=%d (energy=%.4f, effect=%.1f MAD, %.1fσ, relΔ=%.1f%%)",
			series.Name, changeType, series.Points[bestSplit].Timestamp,
			normalizedEnergy, effectSize, deviation, relChange*100),
		Tags:      series.Tags,
		Timestamp: series.Points[bestSplit].Timestamp,
		Score:     &score,
		DebugInfo: debugInfo,
	}

	return observer.MetricsDetectionResult{Anomalies: []observer.Anomaly{anomaly}}
}

// findBestSplit finds the split point that maximizes the energy statistic.
// Uses incremental computation for efficiency.
func (d *EDivisiveDetector) findBestSplit(values []float64, minSegSize, subsampleStep int) (int, float64) {
	n := len(values)
	if n < 2*minSegSize {
		return -1, 0
	}

	bestSplit := -1
	bestEnergy := 0.0

	// Initialize: A = values[:minSegSize], B = values[minSegSize:]
	nA := minSegSize
	nB := n - minSegSize

	// Compute initial sums
	sumAA := pairwiseAbsDiffSum(values[:nA])
	sumBB := pairwiseAbsDiffSum(values[nA:])
	sumAB := crossAbsDiffSum(values[:nA], values[nA:])

	energy := computeEnergy(nA, nB, sumAB, sumAA, sumBB)
	if energy > bestEnergy {
		bestEnergy = energy
		bestSplit = minSegSize
	}

	// Slide split point incrementally
	for t := minSegSize + 1; t <= n-minSegSize; t++ {
		// Move values[t-1] from B to A
		movedVal := values[t-1]

		// Compute sum of |movedVal - a_j| for all j in current A (indices 0..t-2)
		sumMovedToA := 0.0
		for j := 0; j < t-1; j++ {
			sumMovedToA += math.Abs(movedVal - values[j])
		}

		// Compute sum of |movedVal - b_j| for all j in new B (indices t..n-1)
		sumMovedToB := 0.0
		for j := t; j < n; j++ {
			sumMovedToB += math.Abs(movedVal - values[j])
		}

		// Update sums incrementally
		sumAA += 2 * sumMovedToA
		sumBB -= 2 * sumMovedToB
		sumAB = sumAB - sumMovedToA + sumMovedToB

		nA = t
		nB = n - t

		// Only evaluate at subsampled points
		if (t-minSegSize)%subsampleStep != 0 {
			continue
		}

		energy = computeEnergy(nA, nB, sumAB, sumAA, sumBB)
		if energy > bestEnergy {
			bestEnergy = energy
			bestSplit = t
		}
	}

	return bestSplit, bestEnergy
}

// computeEnergy computes the energy statistic E(A, B) given precomputed sums.
// E(A,B) = 2/(|A|*|B|) * sumAB - 1/|A|^2 * sumAA - 1/|B|^2 * sumBB
func computeEnergy(nA, nB int, sumAB, sumAA, sumBB float64) float64 {
	if nA <= 1 || nB <= 1 {
		return 0
	}
	fA := float64(nA)
	fB := float64(nB)
	// Scale by nA*nB/(nA+nB) to get the test statistic (Matteson & James, Eq. 3)
	nTotal := fA + fB
	return (2.0/(fA*fB)*sumAB - 1.0/(fA*fA)*sumAA - 1.0/(fB*fB)*sumBB) * fA * fB / nTotal
}

// pairwiseAbsDiffSum computes sum of |x_i - x_j| for all ordered pairs (i,j), i != j.
func pairwiseAbsDiffSum(vals []float64) float64 {
	n := len(vals)
	if n < 2 {
		return 0
	}
	// Sort a copy: sum_{i<j} |x_i - x_j| = sum_k (2k - n + 1) * x_{(k)} for sorted values
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)

	var sum float64
	for k, v := range sorted {
		sum += float64(2*k-n+1) * v
	}
	// Double for both orderings
	return 2 * sum
}

// crossAbsDiffSum computes sum of |a_i - b_j| for all i, j.
func crossAbsDiffSum(a, b []float64) float64 {
	sortedB := make([]float64, len(b))
	copy(sortedB, b)
	sort.Float64s(sortedB)

	prefixSum := make([]float64, len(sortedB)+1)
	for i, v := range sortedB {
		prefixSum[i+1] = prefixSum[i] + v
	}

	totalB := prefixSum[len(sortedB)]
	nB := len(sortedB)

	var sum float64
	for _, ai := range a {
		k := sort.SearchFloat64s(sortedB, ai+1e-300)
		sum += ai*float64(k) - prefixSum[k] + (totalB - prefixSum[k]) - ai*float64(nB-k)
	}
	return sum
}

// variance computes the population variance.
func variance(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := edMeanValues(vals)
	var s float64
	for _, v := range vals {
		d := v - m
		s += d * d
	}
	return s / float64(len(vals))
}

// edMeanValues computes the mean of a float64 slice.
func edMeanValues(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var s float64
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// edStddevValues computes sample standard deviation.
func edStddevValues(vals []float64, m float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	var s float64
	for _, v := range vals {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(vals)-1))
}

// medianValues computes the median of a float64 slice.
func medianValues(vals []float64) float64 {
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

// madValues computes the median absolute deviation.
func madValues(vals []float64, median float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	devs := make([]float64, len(vals))
	for i, v := range vals {
		devs[i] = math.Abs(v - median)
	}
	return medianValues(devs)
}
