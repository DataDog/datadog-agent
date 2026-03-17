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

// WinCompDetector detects changepoints by comparing adjacent sliding windows
// using the Welch t-test, verified by robust MAD-based deviation.
//
// Algorithm:
//  1. Slide a split point through the series with two adjacent windows
//     (baseline window of size W before the split, recent window of size W after)
//  2. At each split, compute Welch's t-statistic
//  3. Find the split with the maximum |t|
//  4. Verify with MAD-based deviation filter
//  5. After detection, advance past the changepoint for next detection
//
// This is a lightweight scan approach that uses fixed-size windows instead of
// growing segments, making it O(n) per scan instead of O(n^2). The fixed window
// size means it's naturally suited for detecting level shifts at any position,
// not biased toward the middle of the series.
//
// Implements SeriesDetector (batch) — the seriesDetectorAdapter handles streaming.
type WinCompDetector struct {
	// WindowSize is the number of points in each comparison window.
	// Default: 30
	WindowSize int

	// MinTStat is the minimum |Welch t-statistic| for reporting.
	// Default: 5.0
	MinTStat float64

	// MinDeviationMAD is the minimum |post_median - pre_median| / MAD.
	// Default: 3.0
	MinDeviationMAD float64

	// MinPoints is the minimum total points before detection runs.
	// Default: 30
	MinPoints int

	// MaxFires is the maximum changepoints per series.
	// Default: 3
	MaxFires int

	// MinEffectSize is the minimum rank-biserial correlation for reporting.
	// Default: 0.7
	MinEffectSize float64

	// fired tracks fires per series.
	fired map[string]int
}

// NewWinCompDetector creates a WinComp detector with default settings.
func NewWinCompDetector() *WinCompDetector {
	return &WinCompDetector{
		WindowSize:      30,
		MinTStat:        8.0,
		MinDeviationMAD: 5.0,
		MinPoints:       30,
		MaxFires:        2,
		MinEffectSize:   0.85,
		fired:           make(map[string]int),
	}
}

// Name returns the detector name.
func (d *WinCompDetector) Name() string {
	return "wincomp"
}

// Reset clears internal state.
func (d *WinCompDetector) Reset() {
	d.fired = make(map[string]int)
}

// Detect implements SeriesDetector.
func (d *WinCompDetector) Detect(series observer.Series) observer.DetectionResult {
	if d.fired == nil {
		d.fired = make(map[string]int)
	}

	fireKey := series.Name + "|" + strings.Join(series.Tags, ",")
	maxFires := d.MaxFires
	if maxFires <= 0 {
		maxFires = 3
	}
	if d.fired[fireKey] >= maxFires {
		return observer.DetectionResult{}
	}

	n := len(series.Points)
	minPts := d.MinPoints
	if minPts <= 0 {
		minPts = 30
	}
	if n < minPts {
		return observer.DetectionResult{}
	}

	w := d.WindowSize
	if w <= 0 {
		w = 30
	}
	minTStat := d.MinTStat
	if minTStat <= 0 {
		minTStat = 5.0
	}
	minDevMAD := d.MinDeviationMAD
	if minDevMAD <= 0 {
		minDevMAD = 3.0
	}
	minEffect := d.MinEffectSize
	if minEffect <= 0 {
		minEffect = 0.7
	}

	values := make([]float64, n)
	for i, p := range series.Points {
		values[i] = p.Value
	}

	// Use cumulative sums for O(1) mean/variance at each window position.
	cumSum := make([]float64, n+1)
	cumSumSq := make([]float64, n+1)
	for i, v := range values {
		cumSum[i+1] = cumSum[i] + v
		cumSumSq[i+1] = cumSumSq[i] + v*v
	}

	var anomalies []observer.Anomaly
	segStart := 0

	for d.fired[fireKey] < maxFires {
		// Need at least 2*w points from segStart.
		if segStart+2*w > n {
			break
		}

		bestTAbs := 0.0
		bestK := -1

		// Slide the split point.
		for k := segStart + w; k <= n-w; k++ {
			// Left window: [k-w, k), Right window: [k, k+w)
			leftSum := cumSum[k] - cumSum[k-w]
			leftSumSq := cumSumSq[k] - cumSumSq[k-w]
			rightSum := cumSum[k+w] - cumSum[k]
			rightSumSq := cumSumSq[k+w] - cumSumSq[k]

			fw := float64(w)
			leftMean := leftSum / fw
			rightMean := rightSum / fw
			leftVar := leftSumSq/fw - leftMean*leftMean
			rightVar := rightSumSq/fw - rightMean*rightMean

			if leftVar < 0 {
				leftVar = 0
			}
			if rightVar < 0 {
				rightVar = 0
			}

			denom := math.Sqrt(leftVar/fw + rightVar/fw)
			if denom < 1e-12 {
				continue
			}

			tStat := math.Abs(leftMean-rightMean) / denom
			if tStat > bestTAbs {
				bestTAbs = tStat
				bestK = k
			}
		}

		if bestK < 0 || bestTAbs < minTStat {
			break
		}

		// Verify with robust stats.
		preStart := bestK - w
		if preStart < segStart {
			preStart = segStart
		}
		preVals := values[preStart:bestK]
		postEnd := bestK + w
		if postEnd > n {
			postEnd = n
		}
		postVals := values[bestK:postEnd]

		preMedian := detectorMedian(preVals)
		postMedian := detectorMedian(postVals)
		preMAD := detectorMAD(preVals, preMedian, false)
		denom := preMAD
		if denom < 1e-10 {
			denom = math.Max(math.Abs(preMedian)*0.01, 1e-6)
		}
		deviation := math.Abs(postMedian-preMedian) / denom

		if deviation < minDevMAD {
			break
		}

		// Check effect size using rank-biserial correlation.
		// Combine pre+post into a single array and compute ranks.
		combined := make([]float64, 0, len(preVals)+len(postVals))
		combined = append(combined, preVals...)
		combined = append(combined, postVals...)
		ranks, _ := assignRanks(combined)

		var R1 float64
		n1 := len(preVals)
		n2 := len(postVals)
		for i := 0; i < n1; i++ {
			R1 += ranks[i]
		}
		U1 := R1 - float64(n1)*float64(n1+1)/2
		U := math.Min(U1, float64(n1)*float64(n2)-U1)
		effectSize := rankBiserialCorrelation(U, n1, n2)

		if math.Abs(effectSize) < minEffect {
			break
		}

		d.fired[fireKey]++

		direction := "increased"
		if postMedian < preMedian {
			direction = "decreased"
		}

		score := bestTAbs * deviation / 100
		anomalies = append(anomalies, observer.Anomaly{
			Title: fmt.Sprintf("WinComp changepoint: %s", series.Name),
			Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, t=%.2f, effect=%.2f, %.1f MADs)",
				series.Name, direction, preMedian, postMedian, bestTAbs, effectSize, deviation),
			Tags:      series.Tags,
			Timestamp: series.Points[bestK].Timestamp,
			Score:     &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineMedian: preMedian,
				BaselineMAD:    preMAD,
				CurrentValue:   postMedian,
				DeviationSigma: deviation,
			},
		})

		// Advance past the changepoint.
		segStart = bestK + w/2
	}

	return observer.DetectionResult{Anomalies: anomalies}
}
