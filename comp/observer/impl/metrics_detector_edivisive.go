// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// EDivisiveDetector detects changepoints using the E-Divisive energy statistic.
// For 1D data, it scans all possible split points and finds the one that maximizes
// the energy distance between the two resulting segments. This is a nonparametric
// method that makes no distributional assumptions.
//
// Implements SeriesDetector (batch) — the seriesDetectorAdapter handles streaming.
//
// Reference: Matteson & James (2014), "A Nonparametric Approach for Multiple
// Change Point Analysis of Multivariate Data."
type EDivisiveDetector struct {
	// MinSegment is the minimum number of points in each segment after splitting.
	// Default: 15
	MinSegment int

	// MinPoints is the minimum total points before detection runs.
	// Default: 30
	MinPoints int

	// PenaltyFactor scales the penalty term. Higher = fewer changepoints.
	// The penalty is PenaltyFactor * log(n).
	// Default: 8.0
	PenaltyFactor float64

	// MinRelativeChange is the minimum |post_median - pre_median| / MAD for reporting.
	// Default: 2.0
	MinRelativeChange float64

	// fired tracks which series have been reported (keyed by series name+tags).
	fired map[string]bool
}

// NewEDivisiveDetector creates an E-Divisive detector with default settings.
func NewEDivisiveDetector() *EDivisiveDetector {
	return &EDivisiveDetector{
		MinSegment:        15,
		MinPoints:         30,
		PenaltyFactor:     12.0,
		MinRelativeChange: 4.0,
		fired:             make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *EDivisiveDetector) Name() string {
	return "edivisive"
}

// Reset clears internal state.
func (d *EDivisiveDetector) Reset() {
	d.fired = make(map[string]bool)
}

// Detect implements SeriesDetector. It scans the series for the single best
// changepoint using the energy statistic.
func (d *EDivisiveDetector) Detect(series observer.Series) observer.DetectionResult {
	fireKey := series.Name + "|" + strings.Join(series.Tags, ",")
	if d.fired[fireKey] {
		return observer.DetectionResult{}
	}

	n := len(series.Points)
	if n < d.MinPoints {
		return observer.DetectionResult{}
	}

	values := make([]float64, n)
	for i, p := range series.Points {
		values[i] = p.Value
	}

	// Scan all valid split points and find the one that maximizes
	// the penalized Gaussian log-likelihood gain (equivalent to PELT L2 cost).
	// Gain(k) = n*log(var_total) - k*log(var_left) - (n-k)*log(var_right)
	// This captures both mean shifts and variance changes.

	// Compute cumulative sums for O(n) computation.
	cumSum := make([]float64, n+1)
	cumSumSq := make([]float64, n+1)
	for i, v := range values {
		cumSum[i+1] = cumSum[i] + v
		cumSumSq[i+1] = cumSumSq[i] + v*v
	}

	// Total variance
	totalMean := cumSum[n] / float64(n)
	totalVar := cumSumSq[n]/float64(n) - totalMean*totalMean
	if totalVar < 1e-12 {
		return observer.DetectionResult{} // constant series
	}
	totalCost := float64(n) * math.Log(totalVar)

	penalty := d.PenaltyFactor * math.Log(float64(n))
	bestGain := 0.0
	bestK := -1

	minSeg := d.MinSegment
	for k := minSeg; k <= n-minSeg; k++ {
		fk := float64(k)
		fn_k := float64(n - k)

		leftMean := cumSum[k] / fk
		leftVar := cumSumSq[k]/fk - leftMean*leftMean
		if leftVar < 1e-12 {
			leftVar = 1e-12
		}

		rightMean := (cumSum[n] - cumSum[k]) / fn_k
		rightVar := (cumSumSq[n]-cumSumSq[k])/fn_k - rightMean*rightMean
		if rightVar < 1e-12 {
			rightVar = 1e-12
		}

		splitCost := fk*math.Log(leftVar) + fn_k*math.Log(rightVar)
		gain := totalCost - splitCost

		if gain > bestGain {
			bestGain = gain
			bestK = k
		}
	}

	if bestK < 0 || bestGain < penalty {
		return observer.DetectionResult{}
	}

	// Compute pre/post statistics for the anomaly description.
	preVals := values[:bestK]
	postVals := values[bestK:]
	preMedian := detectorMedian(preVals)
	postMedian := detectorMedian(postVals)
	preMAD := detectorMAD(preVals, preMedian, false)

	// Check minimum relative change
	denom := preMAD
	if denom < 1e-10 {
		denom = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}
	relChange := math.Abs(postMedian-preMedian) / denom
	if relChange < d.MinRelativeChange {
		return observer.DetectionResult{}
	}

	d.fired[fireKey] = true

	changePtTime := series.Points[bestK].Timestamp
	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	score := bestGain
	return observer.DetectionResult{
		Anomalies: []observer.Anomaly{
			{
				Title: fmt.Sprintf("E-Divisive changepoint: %s", series.Name),
				Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, gain=%.2f, relΔ=%.1f MADs)",
					series.Name, direction, preMedian, postMedian, bestGain, relChange),
				Tags:      series.Tags,
				Timestamp: changePtTime,
				Score:     &score,
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineMedian: preMedian,
					BaselineMAD:    preMAD,
					CurrentValue:   postMedian,
					DeviationSigma: relChange,
				},
			},
		},
	}
}
