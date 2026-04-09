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

// scanwelchStateKey identifies per-series state by ref and aggregation.
type scanwelchStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// scanwelchSeriesState holds per-series streaming state for the ScanWelch detector.
// Same pattern as scanmwSeriesState and BOCPD (metrics_detector_bocpd.go).
type scanwelchSeriesState struct {
	lastProcessedCount int
	lastWriteGen       int64

	// Segment tracking: only scan [segmentStartTime, dataTime].
	segmentStartTime int64

	// Reusable point buffer — grows once per series, reused across scans
	// to avoid per-call allocation from GetSeriesRange.
	buf []observer.Point
}

// ScanWelchDetector detects changepoints by scanning all possible split points
// with Welch's t-test for candidate selection, then verifies each candidate
// with a Mann-Whitney p-value filter and MAD-based deviation check.
//
// This hybrid uses parametric detection (t-test finds mean shifts efficiently)
// combined with nonparametric verification (MW p-value for selectivity).
//
// Implements Detector (streaming) — after finding a changepoint, advances
// the segment start so subsequent scans only examine post-change data.
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

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[scanwelchStateKey]*scanwelchSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
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
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[scanwelchStateKey]*scanwelchSeriesState),
	}
}

// Name returns the detector name.
func (d *ScanWelchDetector) Name() string {
	return "scanwelch"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *ScanWelchDetector) Reset() {
	d.series = make(map[scanwelchStateKey]*scanwelchSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements Detector. Same iteration pattern as ScanMW and BOCPD —
// consider dedup if more scan-based detectors are added.
func (d *ScanWelchDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	// Bulk-fetch point counts and write generations in a single lock acquisition.
	// This avoids 2×len(series) individual RLock/RUnlock calls per Detect() call.
	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := scanwelchStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &scanwelchSeriesState{}
				d.series[sk] = state
			}

			// Replay optimization: skip unless MinSegment new points are visible.
			// The scan needs MinSegment points per side to evaluate a split, so
			// fewer new points can't create a new valid split boundary. This cuts
			// per-series scans from O(timestamps) to O(timestamps/MinSegment).
			// During live ingestion, writeGen changes on every write so this
			// condition falls through to the gen check and behaves as before.
			if status.pointCount < state.lastProcessedCount+d.MinSegment && status.writeGeneration == state.lastWriteGen {
				continue
			}

			// Collect points into reusable buffer to avoid per-call allocation.
			// ForEachPoint uses a pooled buffer internally; we append into
			// state.buf which grows once and is reused across scans.
			state.buf = state.buf[:0]
			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.segmentStartTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
					// Capture series metadata on first point (valid during callback).
					sCopy := *s
					seriesMeta = &sCopy
				}
				state.buf = append(state.buf, p)
			})

			if seriesMeta == nil || len(state.buf) < d.MinPoints {
				state.lastProcessedCount = status.pointCount
				state.lastWriteGen = status.writeGeneration
				continue
			}

			anomaly, changeIdx, found := d.scanWelch(state.buf, seriesMeta, agg)
			if found {
				anomaly.SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
				allAnomalies = append(allAnomalies, anomaly)
				state.segmentStartTime = state.buf[changeIdx].Timestamp - 1
			}

			state.lastProcessedCount = status.pointCount
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// scanWelch runs the hybrid Welch/MW scan on points within the current segment.
// Returns (anomaly, changeIndex, found).
func (d *ScanWelchDetector) scanWelch(points []observer.Point, series *observer.Series, agg observer.Aggregate) (observer.Anomaly, int, bool) {
	n := len(points)

	values := make([]float64, n)
	for i, p := range points {
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
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
	}

	// Effect size check
	effectSize := rankBiserialCorrelation(U, bestK, n-bestK)
	if math.Abs(effectSize) < d.MinEffectSize {
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
	}

	changePtTime := points[bestK].Timestamp
	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	score := -math.Log10(pValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "ScanWelch changepoint: " + seriesName,
		Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, t=%.2f, p=%.2e, effect=%.2f, %.1f MADs)",
			seriesName, direction, preMedian, postMedian, bestTAbs, pValue, effectSize, deviation),
		Timestamp:           changePtTime,
		Score:               &score,
		SamplingIntervalSec: medianPointInterval(points),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: preMedian,
			BaselineMAD:    preMAD,
			CurrentValue:   postMedian,
			DeviationSigma: deviation,
		},
	}

	return anomaly, bestK, true
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (d *ScanWelchDetector) ensureDefaults() {
	if d.MinSegment <= 0 {
		d.MinSegment = 12
	}
	if d.MinPoints <= 0 {
		d.MinPoints = 30
	}
	if d.MinTStatistic <= 0 {
		d.MinTStatistic = 8.0
	}
	if d.SignificanceThreshold <= 0 {
		d.SignificanceThreshold = 1e-8
	}
	if d.MinEffectSize <= 0 {
		d.MinEffectSize = 0.85
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = 3.0
	}
	if d.series == nil {
		d.series = make(map[scanwelchStateKey]*scanwelchSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
