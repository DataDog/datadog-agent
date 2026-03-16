// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ScanMWDetector detects changepoints by scanning all possible split points
// with the Mann-Whitney U test. It picks the split that gives the most
// significant test result (smallest p-value), making it a non-parametric
// changepoint detector that's robust to distribution shape.
//
// Uses an efficient O(n log n) implementation: ranks are assigned once via
// sorting, then the rank sum is updated incrementally as the split point moves.
//
// Implements SeriesDetector (batch) — the seriesDetectorAdapter handles streaming.
type ScanMWDetector struct {
	// MinSegment is the minimum number of points in each segment.
	// Default: 12
	MinSegment int

	// MinPoints is the minimum total points before detection runs.
	// Default: 30
	MinPoints int

	// SignificanceThreshold is the maximum p-value for the best split to be
	// considered a changepoint. Default: 1e-6
	SignificanceThreshold float64

	// MinEffectSize is the minimum |rank-biserial correlation| for reporting.
	// Default: 0.8
	MinEffectSize float64

	// MinDeviationMAD is the minimum |post_median - pre_median| / MAD.
	// Default: 2.5
	MinDeviationMAD float64

	// fired tracks which series have been reported.
	fired map[string]bool
}

// NewScanMWDetector creates a ScanMW detector with default settings.
func NewScanMWDetector() *ScanMWDetector {
	return &ScanMWDetector{
		MinSegment:            12,
		MinPoints:             30,
		SignificanceThreshold: 1e-8,
		MinEffectSize:         0.85,
		MinDeviationMAD:       3.0,
		fired:                 make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *ScanMWDetector) Name() string {
	return "scanmw"
}

// Reset clears internal state.
func (d *ScanMWDetector) Reset() {
	d.fired = make(map[string]bool)
}

// Detect implements SeriesDetector.
func (d *ScanMWDetector) Detect(series observer.Series) observer.DetectionResult {
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

	// Efficient O(n log n) scan: assign ranks once, then slide the split point.
	// ranks[i] = rank of values[i] in the combined sorted order (tie-averaged).
	ranks, tieCorrection := assignRanks(values)

	// Initialize R1 = sum of ranks for values[0..minSeg-1] (the "before" group).
	minSeg := d.MinSegment
	var R1 float64
	for i := 0; i < minSeg; i++ {
		R1 += ranks[i]
	}

	bestZAbs := 0.0
	bestK := -1

	fN := float64(n)

	for k := minSeg; k <= n-minSeg; k++ {
		if k > minSeg {
			// Move split: value at index k-1 moves from "after" to "before".
			R1 += ranks[k-1]
		}

		fk := float64(k)
		fn_k := float64(n - k)

		U1 := R1 - fk*(fk+1)/2
		U := math.Min(U1, fk*fn_k-U1)

		meanU := fk * fn_k / 2
		varU := (fk * fn_k / 12) * (fN + 1 - tieCorrection/(fN*(fN-1)))
		if varU <= 0 {
			continue
		}
		stdU := math.Sqrt(varU)

		z := (math.Abs(U-meanU) - 0.5) / stdU
		if z < 0 {
			z = 0
		}

		if z > bestZAbs {
			bestZAbs = z
			bestK = k
		}
	}

	if bestK < 0 {
		return observer.DetectionResult{}
	}

	// Convert best z to p-value.
	bestPValue := 2 * normalCDFUpper(bestZAbs)
	if bestPValue > 1.0 {
		bestPValue = 1.0
	}

	if bestPValue >= d.SignificanceThreshold {
		return observer.DetectionResult{}
	}

	// Recompute U at bestK for effect size.
	var bestR1 float64
	for i := 0; i < bestK; i++ {
		bestR1 += ranks[i]
	}
	bestU1 := bestR1 - float64(bestK)*float64(bestK+1)/2
	bestU := math.Min(bestU1, float64(bestK)*float64(n-bestK)-bestU1)

	effectSize := rankBiserialCorrelation(bestU, bestK, n-bestK)
	if math.Abs(effectSize) < d.MinEffectSize {
		return observer.DetectionResult{}
	}

	// Check robust deviation at best split.
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

	score := -math.Log10(bestPValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	return observer.DetectionResult{
		Anomalies: []observer.Anomaly{
			{
				Title: fmt.Sprintf("ScanMW changepoint: %s", series.Name),
				Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, p=%.2e, effect=%.2f, %.1f MADs)",
					series.Name, direction, preMedian, postMedian, bestPValue, effectSize, deviation),
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

// assignRanks computes the average rank of each value in its original position.
// Returns (ranks, tieCorrection) where tieCorrection = sum(t^3 - t) for tie groups.
func assignRanks(values []float64) ([]float64, float64) {
	n := len(values)

	type indexedValue struct {
		value float64
		index int
	}

	indexed := make([]indexedValue, n)
	for i, v := range values {
		indexed[i] = indexedValue{value: v, index: i}
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].value < indexed[j].value
	})

	ranks := make([]float64, n)
	tieCorrection := 0.0

	i := 0
	for i < n {
		j := i
		for j < n && indexed[j].value == indexed[i].value {
			j++
		}
		avgRank := float64(i+1+j) / 2.0
		tieSize := float64(j - i)
		for k := i; k < j; k++ {
			ranks[indexed[k].index] = avgRank
		}
		tieCorrection += tieSize*tieSize*tieSize - tieSize
		i = j
	}

	return ranks, tieCorrection
}
