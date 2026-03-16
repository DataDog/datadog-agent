// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// edivisiveSeriesState holds per-series streaming state for the E-Divisive detector.
// Same pattern as scanmwSeriesState and BOCPD (metrics_detector_bocpd.go).
type edivisiveSeriesState struct {
	key observer.SeriesKey
	agg observer.Aggregate

	lastProcessedCount int
	lastWriteGen       int64

	// Segment tracking: only scan [segmentStartTime, dataTime].
	segmentStartTime int64
}

// EDivisiveDetector detects changepoints using the E-Divisive energy statistic.
// For 1D data, it scans all possible split points and finds the one that maximizes
// the energy distance between the two resulting segments. This is a nonparametric
// method that makes no distributional assumptions.
//
// Implements Detector (streaming) — after finding a changepoint, advances
// the segment start so subsequent scans only examine post-change data.
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
	// Default: 12.0
	PenaltyFactor float64

	// MinRelativeChange is the minimum |post_median - pre_median| / MAD for reporting.
	// Default: 4.0
	MinRelativeChange float64

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate

	// per-series state keyed by "namespace|name|tags|agg"
	series map[string]*edivisiveSeriesState

	// Cache the discovered series list across Detect calls.
	cachedKeys []observer.SeriesKey
	cachedGen  uint64
}

// NewEDivisiveDetector creates an E-Divisive detector with default settings.
func NewEDivisiveDetector() *EDivisiveDetector {
	return &EDivisiveDetector{
		MinSegment:        15,
		MinPoints:         30,
		PenaltyFactor:     12.0,
		MinRelativeChange: 4.0,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[string]*edivisiveSeriesState),
	}
}

// Name returns the detector name.
func (d *EDivisiveDetector) Name() string {
	return "edivisive"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *EDivisiveDetector) Reset() {
	d.series = make(map[string]*edivisiveSeriesState)
	d.cachedKeys = nil
	d.cachedGen = 0
}

// Detect implements Detector. Same iteration pattern as ScanMW, ScanWelch,
// and BOCPD — consider dedup if more scan-based detectors are added.
func (d *EDivisiveDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedKeys == nil || gen != d.cachedGen {
		d.cachedKeys = storage.ListSeries(observer.SeriesFilter{})
		d.cachedGen = gen
	}

	var allAnomalies []observer.Anomaly

	for _, key := range d.cachedKeys {
		for _, agg := range d.Aggregations {
			stateKey := d.stateKey(key, agg)

			state, exists := d.series[stateKey]
			if !exists {
				state = &edivisiveSeriesState{key: key, agg: agg}
				d.series[stateKey] = state
			}

			visibleCount := storage.PointCountUpTo(key, dataTime)
			currentGen := storage.WriteGeneration(key)
			if visibleCount <= state.lastProcessedCount && currentGen == state.lastWriteGen {
				continue
			}

			series := storage.GetSeriesRange(key, state.segmentStartTime, dataTime, agg)
			if series == nil || len(series.Points) < d.MinPoints {
				state.lastProcessedCount = visibleCount
				state.lastWriteGen = currentGen
				continue
			}

			anomaly, changeIdx, found := d.scanEDivisive(series.Points, key, agg)
			if found {
				allAnomalies = append(allAnomalies, anomaly)
				state.segmentStartTime = series.Points[changeIdx].Timestamp
			}

			state.lastProcessedCount = visibleCount
			state.lastWriteGen = currentGen
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// scanEDivisive runs the energy-statistic scan on points within the current segment.
// Returns (anomaly, changeIndex, found).
func (d *EDivisiveDetector) scanEDivisive(points []observer.Point, key observer.SeriesKey, agg observer.Aggregate) (observer.Anomaly, int, bool) {
	n := len(points)

	values := make([]float64, n)
	for i, p := range points {
		values[i] = p.Value
	}

	// Compute cumulative sums for O(n) variance computation.
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
		return observer.Anomaly{}, 0, false // constant series
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
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
	}

	changePtTime := points[bestK].Timestamp
	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	score := bestGain
	seriesName := key.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:           observer.AnomalyTypeMetric,
		Source:         observer.MetricName(seriesName),
		SourceSeriesID: observer.SeriesID(seriesKey(key.Namespace, seriesName, key.Tags)),
		DetectorName:   d.Name(),
		Title:          fmt.Sprintf("E-Divisive changepoint: %s", seriesName),
		Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, gain=%.2f, relΔ=%.1f MADs)",
			seriesName, direction, preMedian, postMedian, bestGain, relChange),
		Tags:      key.Tags,
		Timestamp: changePtTime,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: preMedian,
			BaselineMAD:    preMAD,
			CurrentValue:   postMedian,
			DeviationSigma: relChange,
		},
	}

	return anomaly, bestK, true
}

// stateKey returns a unique key for per-series state tracking.
func (d *EDivisiveDetector) stateKey(key observer.SeriesKey, agg observer.Aggregate) string {
	return seriesKey(key.Namespace, key.Name, key.Tags) + "|" + aggSuffix(agg)
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *EDivisiveDetector) ensureDefaults() {
	if d.MinSegment <= 0 {
		d.MinSegment = 15
	}
	if d.MinPoints <= 0 {
		d.MinPoints = 30
	}
	if d.PenaltyFactor <= 0 {
		d.PenaltyFactor = 12.0
	}
	if d.MinRelativeChange <= 0 {
		d.MinRelativeChange = 4.0
	}
	if d.series == nil {
		d.series = make(map[string]*edivisiveSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
