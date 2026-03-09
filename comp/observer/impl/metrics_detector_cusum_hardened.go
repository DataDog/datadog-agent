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

// HardenedCUSUMDetector is a precision-focused CUSUM detector that combines
// robust statistics (median + MAD) with strict multi-layered filtering to
// emit only high-confidence changepoints.
//
// Design philosophy: Phase 2 showed that SELECTIVITY is the key to high L2 F1.
// Mann-Whitney v2 achieved 0.468 avg L2 F1 by combining 5 strict filters.
// This detector applies the same principle to the CUSUM algorithm:
//
// Filter 1: CUSUM threshold crossing (ThresholdFactor * robustSigma)
// Filter 2: Instantaneous deviation gate (MinDeviationAtCrossing MADs)
// Filter 3: Relative change (|post_median - baseline_median| / |baseline_median|)
// Filter 4: Effect size (rank-biserial correlation between baseline and post windows)
// Filter 5: Mann-Whitney U significance (borrowed from MW v2 for additional validation)
//
// The combination of CUSUM (sequential detection) + MW (distribution comparison)
// creates a very selective detector that only fires on genuine regime changes.
type HardenedCUSUMDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	// Default: 60
	MinPoints int

	// BaselineWindowFraction is the fraction of series length used as the
	// sliding baseline window size. Default: 0.25
	BaselineWindowFraction float64

	// SlackFactor is multiplied by the robust sigma estimate to get k (slack).
	// Default: 1.0 (higher than standard CUSUM to suppress noise accumulation)
	SlackFactor float64

	// ThresholdFactor is multiplied by the robust sigma estimate to get h.
	// Default: 10.0
	ThresholdFactor float64

	// MinDeviationAtCrossing is the minimum |x - median| / scaledMAD at crossing.
	// Very strict: only metrics with instantaneous deviation > 15 MADs pass.
	// Default: 15.0
	MinDeviationAtCrossing float64

	// MinRelativeChange is the minimum |post_median - baseline_median| / |baseline_median|.
	// Requires the metric to at least double (100% change). Default: 1.00
	MinRelativeChange float64

	// MinEffectSize is the minimum |rank-biserial correlation| from the
	// Mann-Whitney U test between pre- and post-crossing windows.
	// Range [0, 1], 1 = perfect rank separation. Default: 0.99
	MinEffectSize float64

	// MinThresholdFactor sets a floor on the threshold relative to the median.
	// Default: 0.10
	MinThresholdFactor float64

	// PostWindowSize is the number of points after the crossing to validate.
	// Used for relative change and effect size checks. Default: 60
	PostWindowSize int
}

// NewHardenedCUSUMDetector creates a HardenedCUSUMDetector with default settings.
func NewHardenedCUSUMDetector() *HardenedCUSUMDetector {
	return &HardenedCUSUMDetector{
		MinPoints:              60,
		BaselineWindowFraction: 0.25,
		SlackFactor:            1.0,
		ThresholdFactor:        10.0,
		MinDeviationAtCrossing: 15.0,
		MinRelativeChange:      1.00,
		MinEffectSize:          0.99,
		MinThresholdFactor:     0.10,
		PostWindowSize:         60,
	}
}

// Name returns the detector name.
func (d *HardenedCUSUMDetector) Name() string {
	return "cusum_hardened_detector"
}

// Detect runs hardened CUSUM on the series and returns an anomaly if a
// high-confidence shift is detected. Returns at most one anomaly (single-fire).
func (d *HardenedCUSUMDetector) Detect(series observer.Series) observer.MetricsDetectionResult {
	minPoints := d.MinPoints
	if minPoints <= 0 {
		minPoints = 60
	}
	baselineWindowFrac := d.BaselineWindowFraction
	if baselineWindowFrac <= 0 {
		baselineWindowFrac = 0.25
	}
	slackFactor := d.SlackFactor
	if slackFactor <= 0 {
		slackFactor = 1.0
	}
	thresholdFactor := d.ThresholdFactor
	if thresholdFactor <= 0 {
		thresholdFactor = 10.0
	}
	minDeviation := d.MinDeviationAtCrossing
	if minDeviation <= 0 {
		minDeviation = 15.0
	}
	minRelChange := d.MinRelativeChange
	if minRelChange < 0 {
		minRelChange = 1.00
	}
	minEffect := d.MinEffectSize
	if minEffect <= 0 {
		minEffect = 0.99
	}
	minThresholdFactor := d.MinThresholdFactor
	if minThresholdFactor <= 0 {
		minThresholdFactor = 0.10
	}
	postWindowSize := d.PostWindowSize
	if postWindowSize <= 0 {
		postWindowSize = 60
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.MetricsDetectionResult{}
	}

	// Phase 1: Find the most stable baseline window via sliding window MAD minimization
	windowSize := int(float64(n) * baselineWindowFrac)
	if windowSize < 10 {
		windowSize = 10
	}
	if windowSize >= n {
		windowSize = n - 1
	}

	bestStart := 0
	bestMAD := math.Inf(1)

	for start := 0; start+windowSize <= n; start++ {
		windowPoints := series.Points[start : start+windowSize]
		med := medianOfPoints(windowPoints)
		mad := madOfPoints(windowPoints, med)
		if mad < bestMAD {
			bestMAD = mad
			bestStart = start
		}
	}

	baselineEndIdx := bestStart + windowSize
	baselinePoints := series.Points[bestStart:baselineEndIdx]

	// Phase 2: Compute robust baseline statistics (median + MAD)
	baselineMedian := medianOfPoints(baselinePoints)
	baselineMAD := madOfPoints(baselinePoints, baselineMedian)

	const madScaleFactor = 1.4826
	robustSigma := baselineMAD * madScaleFactor

	const epsilon = 1e-10

	if robustSigma < epsilon {
		if math.Abs(baselineMedian) > epsilon {
			robustSigma = math.Abs(baselineMedian) * minThresholdFactor
		} else {
			return observer.MetricsDetectionResult{}
		}
	}

	// CUSUM parameters
	k := slackFactor * robustSigma
	h := thresholdFactor * robustSigma

	// Phase 3: Run two-sided CUSUM, looking for the FIRST valid crossing
	var sHigh, sLow float64

	for i, p := range series.Points {
		sHigh = math.Max(0, sHigh+(p.Value-baselineMedian-k))
		sLow = math.Max(0, sLow+(baselineMedian-p.Value-k))

		// Skip detections within the baseline window
		if i >= bestStart && i < baselineEndIdx {
			continue
		}

		var crossed bool
		var deviation float64

		// Filter 1: CUSUM threshold crossing
		if sHigh > h {
			deviation = (p.Value - baselineMedian) / robustSigma
			crossed = true
		} else if sLow > h {
			deviation = (baselineMedian - p.Value) / robustSigma
			crossed = true
			deviation = -deviation
		}

		if !crossed {
			continue
		}

		// Filter 2: Instantaneous deviation gate
		if math.Abs(deviation) < minDeviation {
			continue
		}

		// Need post-crossing window for remaining filters
		postEnd := i + postWindowSize
		if postEnd > n {
			postEnd = n
		}
		if postEnd-i < 20 {
			continue // not enough post-crossing data
		}

		postPoints := series.Points[i:postEnd]
		postVals := make([]float64, len(postPoints))
		for j, pp := range postPoints {
			postVals[j] = pp.Value
		}

		// Filter 3: Relative change
		postMedian := hcMedian(postVals)
		absBaseline := math.Abs(baselineMedian)
		if absBaseline < 1e-6 {
			absBaseline = 1e-6
		}
		relChange := math.Abs(postMedian-baselineMedian) / absBaseline

		if relChange < minRelChange {
			continue
		}

		// Filter 4: Mann-Whitney U test + effect size
		// Use a window just before the crossing and the post window
		preStart := i - postWindowSize
		if preStart < 0 {
			preStart = 0
		}
		preVals := make([]float64, 0, i-preStart)
		for j := preStart; j < i; j++ {
			preVals = append(preVals, series.Points[j].Value)
		}
		if len(preVals) < 20 {
			continue
		}

		u, pValue := mannWhitneyU(preVals, postVals)
		effectSize := rankBiserialCorrelation(u, len(preVals), len(postVals))

		// Require both extreme statistical significance AND high effect size
		if pValue >= 1e-20 {
			continue
		}
		if math.Abs(effectSize) < minEffect {
			continue
		}

		// All filters passed — emit single anomaly
		direction := "increased"
		if deviation < 0 {
			direction = "decreased"
		}

		metricSource := hardenedCUSUMMetricID(series.Name, series.Tags)

		score := math.Abs(deviation)
		anomaly := observer.Anomaly{
			Source: observer.MetricName(metricSource),
			Title:  fmt.Sprintf("Hardened CUSUM shift: %s", metricSource),
			Description: fmt.Sprintf("%s %s from %.2f to %.2f (%.1fσ, relΔ=%.1f%%, effect=%.2f)",
				series.Name, direction, baselineMedian, postMedian, math.Abs(deviation), relChange*100, effectSize),
			Tags:      series.Tags,
			Timestamp: series.Points[i].Timestamp,
			Score:     &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineStart:  series.Points[bestStart].Timestamp,
				BaselineEnd:    series.Points[baselineEndIdx-1].Timestamp,
				BaselineMean:   baselineMedian,
				BaselineMedian: baselineMedian,
				BaselineMAD:    baselineMAD,
				Threshold:      h,
				SlackParam:     k,
				CurrentValue:   p.Value,
				DeviationSigma: deviation,
			},
		}

		return observer.MetricsDetectionResult{Anomalies: []observer.Anomaly{anomaly}}
	}

	return observer.MetricsDetectionResult{}
}

// hcMedian computes the median of a float64 slice without modifying the input.
func hcMedian(vals []float64) float64 {
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

// hardenedCUSUMMetricID builds a metric identifier in "service:metricName" format
// when a service tag is available. Same pattern as mwMetricID in Mann-Whitney.
func hardenedCUSUMMetricID(name string, tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "service:") {
			service := tag[len("service:"):]
			if service != "" {
				return service + ":" + name
			}
		}
	}
	return name
}
