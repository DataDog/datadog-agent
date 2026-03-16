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

// ScanWelchDetector detects changepoints by scanning all possible split points
// with Welch's t-test for candidate selection, then verifies each candidate
// with a Mann-Whitney p-value filter and MAD-based deviation check.
//
// This hybrid uses parametric detection (t-test finds mean shifts efficiently)
// combined with nonparametric verification (MW p-value for selectivity).
//
// Implements SeriesDetector (batch) — the seriesDetectorAdapter handles streaming.
type ScanWelchDetector struct {
	// MinSegment is the minimum number of points in each segment.
	MinSegment int

	// MinPoints is the minimum total points before detection runs.
	MinPoints int

	// MinTStatistic is the minimum |t| for the candidate selection phase.
	MinTStatistic float64

	// SignificanceThreshold is the maximum MW p-value for reporting.
	SignificanceThreshold float64

	// MinEffectSize is the minimum |rank-biserial correlation|.
	MinEffectSize float64

	// MinDeviationMAD is the minimum |post_median - pre_median| / MAD.
	MinDeviationMAD float64

	// fired tracks which series have been reported.
	fired map[string]bool
}

// NewScanWelchDetector creates a ScanWelch detector with default settings.
func NewScanWelchDetector() *ScanWelchDetector {
	return &ScanWelchDetector{
		MinSegment:            12,
		MinPoints:             30,
		MinTStatistic:         8.0,
		SignificanceThreshold: 1e-8,
		MinEffectSize:         0.85,
		MinDeviationMAD:       3.0,
		fired:                 make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *ScanWelchDetector) Name() string {
	return "scanwelch"
}

// Reset clears internal state.
func (d *ScanWelchDetector) Reset() {
	d.fired = make(map[string]bool)
}

// Detect implements SeriesDetector.
func (d *ScanWelchDetector) Detect(series observer.Series) observer.DetectionResult {
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

	// Phase 1: Scan using Welch's t-statistic (fast, O(n) with cumulative sums).
	cumSum := make([]float64, n+1)
	cumSumSq := make([]float64, n+1)
	for i, v := range values {
		cumSum[i+1] = cumSum[i] + v
		cumSumSq[i+1] = cumSumSq[i] + v*v
	}

	bestTAbs := 0.0
	bestK := -1

	minSeg := d.MinSegment
	for k := minSeg; k <= n-minSeg; k++ {
		fk := float64(k)
		fnk := float64(n - k)

		leftMean := cumSum[k] / fk
		rightMean := (cumSum[n] - cumSum[k]) / fnk

		leftVar := cumSumSq[k]/fk - leftMean*leftMean
		rightVar := (cumSumSq[n]-cumSumSq[k])/fnk - rightMean*rightMean

		if leftVar < 1e-12 {
			leftVar = 1e-12
		}
		if rightVar < 1e-12 {
			rightVar = 1e-12
		}

		se := math.Sqrt(leftVar/fk + rightVar/fnk)
		if se < 1e-15 {
			continue
		}
		t := math.Abs(leftMean-rightMean) / se

		if t > bestTAbs {
			bestTAbs = t
			bestK = k
		}
	}

	if bestK < 0 || bestTAbs < d.MinTStatistic {
		return observer.DetectionResult{}
	}

	// Phase 2: Verify using Mann-Whitney at the best split point.
	ranks, tieCorrection := assignRanks(values)
	var R1 float64
	for i := 0; i < bestK; i++ {
		R1 += ranks[i]
	}

	fk := float64(bestK)
	fnk := float64(n - bestK)
	fN := float64(n)

	U1 := R1 - fk*(fk+1)/2
	U := math.Min(U1, fk*fnk-U1)

	meanU := fk * fnk / 2
	varU := (fk * fnk / 12) * (fN + 1 - tieCorrection/(fN*(fN-1)))
	if varU <= 0 {
		return observer.DetectionResult{}
	}
	stdU := math.Sqrt(varU)

	z := (math.Abs(U-meanU) - 0.5) / stdU
	if z < 0 {
		z = 0
	}

	pValue := 2 * normalCDFUpper(z)
	if pValue > 1.0 {
		pValue = 1.0
	}
	if pValue >= d.SignificanceThreshold {
		return observer.DetectionResult{}
	}

	// Effect size check
	effectSize := rankBiserialCorrelation(U, bestK, n-bestK)
	if math.Abs(effectSize) < d.MinEffectSize {
		return observer.DetectionResult{}
	}

	// Phase 3: Robust deviation check
	preVals := values[:bestK]
	postVals := values[bestK:]
	preMedian := detectorMedian(preVals)
	postMedian := detectorMedian(postVals)
	preMAD := detectorMAD(preVals, preMedian, false)

	denom := preMAD
	if denom < 1e-10 {
		denom = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}
	deviation := math.Abs(postMedian-preMedian) / denom
	if deviation < d.MinDeviationMAD {
		return observer.DetectionResult{}
	}

	d.fired[fireKey] = true

	changePtTime := series.Points[bestK].Timestamp
	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	score := -math.Log10(pValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	return observer.DetectionResult{
		Anomalies: []observer.Anomaly{
			{
				Title: fmt.Sprintf("ScanWelch changepoint: %s", series.Name),
				Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, t=%.2f, p=%.2e, effect=%.2f, %.1f MADs)",
					series.Name, direction, preMedian, postMedian, bestTAbs, pValue, effectSize, deviation),
				Tags:      series.Tags,
				Timestamp: changePtTime,
				Score:     &score,
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineMedian: preMedian,
					BaselineMAD:    preMAD,
					CurrentValue:   postMedian,
					DeviationSigma: deviation,
				},
			},
		},
	}
}
