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

// scanmwStateKey identifies per-series state by ref and aggregation.
type scanmwStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// scanmwSeriesState holds per-series streaming state for the ScanMW detector.
// NOTE: This per-series state struct follows the same pattern used by BOCPD
// (metrics_detector_bocpd.go). If more scan-based detectors are added,
// consider extracting a shared scanSeriesState base.
type scanmwSeriesState struct {
	// Cursor (same pattern as BOCPD: metrics_detector_bocpd.go:16-22)
	lastProcessedCount int
	lastWriteGen       int64

	// Segment tracking: only scan [segmentStartTime, dataTime].
	// 0 initially (scan full history), advances to changepoint timestamp on fire.
	segmentStartTime int64

	// Reusable point buffer — grows once per series, reused across scans
	// to avoid per-call allocation from GetSeriesRange.
	buf []observer.Point
}

// ScanMWDetector detects changepoints by scanning all possible split points
// with the Mann-Whitney U test. It picks the split that gives the most
// significant test result (smallest p-value), making it a non-parametric
// changepoint detector that's robust to distribution shape.
//
// Uses an efficient O(n log n) implementation: ranks are assigned once via
// sorting, then the rank sum is updated incrementally as the split point moves.
//
// Implements Detector (streaming) — after finding a changepoint, advances
// the segment start so subsequent scans only examine post-change data.
type ScanMWDetector struct {
	// MinSegment is the minimum number of points in each segment.
	// Default: 12
	MinSegment int

	// MinPoints is the minimum total points before detection runs.
	// Default: 30
	MinPoints int

	// SignificanceThreshold is the maximum p-value for the best split to be
	// considered a changepoint. Default: 1e-8
	SignificanceThreshold float64

	// MinEffectSize is the minimum |rank-biserial correlation| for reporting.
	// Default: 0.85
	MinEffectSize float64

	// MinDeviationMAD is the minimum |post_median - pre_median| / MAD.
	// Default: 3.0
	MinDeviationMAD float64

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate

	// per-series state keyed by ref+agg
	series map[scanmwStateKey]*scanmwSeriesState

	// Cache the discovered series list across Detect calls.
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewScanMWDetector creates a ScanMW detector with default settings.
func NewScanMWDetector() *ScanMWDetector {
	return &ScanMWDetector{
		MinSegment:            12,
		MinPoints:             30,
		SignificanceThreshold: 1e-8,
		MinEffectSize:         0.85,
		MinDeviationMAD:       3.0,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[scanmwStateKey]*scanmwSeriesState),
	}
}

// Name returns the detector name.
func (d *ScanMWDetector) Name() string {
	return "scanmw"
}

// Reset clears all per-series state for replay/reanalysis.
func (d *ScanMWDetector) Reset() {
	d.series = make(map[scanmwStateKey]*scanmwSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements Detector. It discovers series, reads segment data,
// and scans for changepoints. After finding one, the segment start advances
// so subsequent calls only examine post-change data.
//
// Iteration pattern is the same as BOCPD (metrics_detector_bocpd.go:140-221)
// and ScanWelch — consider dedup if more scan-based detectors are added.
func (d *ScanMWDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	// Bulk-fetch point counts and write generations in a single lock acquisition.
	refs := make([]observer.SeriesRef, len(d.cachedSeries))
	for i, meta := range d.cachedSeries {
		refs[i] = meta.Ref
	}
	bulkStatus := bulkSeriesStatus(storage, refs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, meta := range d.cachedSeries {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			sk := scanmwStateKey{ref: meta.Ref, agg: agg}

			state, exists := d.series[sk]
			if !exists {
				state = &scanmwSeriesState{}
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
			state.buf = state.buf[:0]
			var seriesMeta *observer.Series
			storage.ForEachPoint(meta.Ref, state.segmentStartTime, dataTime, agg, func(s *observer.Series, p observer.Point) {
				if seriesMeta == nil {
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

			anomaly, changeIdx, found := d.scanMW(state.buf, seriesMeta, agg)
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

// scanMW runs the scan algorithm on points within the current segment.
// Returns (anomaly, changeIndex, found). Pure function over the input data.
func (d *ScanMWDetector) scanMW(points []observer.Point, series *observer.Series, agg observer.Aggregate) (observer.Anomaly, int, bool) {
	n := len(points)

	values := make([]float64, n)
	for i, p := range points {
		values[i] = p.Value
	}

	// Efficient O(n log n) scan: assign ranks once, then slide the split point.
	ranks, tieCorrection := assignRanks(values)

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
			R1 += ranks[k-1]
		}

		fk := float64(k)
		fnK := float64(n - k)

		U1 := R1 - fk*(fk+1)/2
		U := math.Min(U1, fk*fnK-U1)

		meanU := fk * fnK / 2
		varU := (fk * fnK / 12) * (fN + 1 - tieCorrection/(fN*(fN-1)))
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
		return observer.Anomaly{}, 0, false
	}

	// Convert best z to p-value.
	bestPValue := 2 * normalCDFUpper(bestZAbs)
	if bestPValue > 1.0 {
		bestPValue = 1.0
	}

	if bestPValue >= d.SignificanceThreshold {
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
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
		return observer.Anomaly{}, 0, false
	}

	changePtTime := points[bestK].Timestamp
	direction := "increased"
	if postMedian < preMedian {
		direction = "decreased"
	}

	score := -math.Log10(bestPValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	seriesName := series.Name + ":" + aggSuffix(agg)
	anomaly := observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "ScanMW changepoint: " + seriesName,
		Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, p=%.2e, effect=%.2f, %.1f MADs)",
			seriesName, direction, preMedian, postMedian, bestPValue, effectSize, deviation),
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
func (d *ScanMWDetector) ensureDefaults() {
	if d.MinSegment <= 0 {
		d.MinSegment = 12
	}
	if d.MinPoints <= 0 {
		d.MinPoints = 30
	}
	if d.SignificanceThreshold <= 0 {
		d.SignificanceThreshold = 1e-8
	}
	if d.MinEffectSize <= 0 {
		d.MinEffectSize = 0.85
	}
	if d.MinDeviationMAD <= 0 {
		d.MinDeviationMAD = 15.0
	}
	if d.series == nil {
		d.series = make(map[scanmwStateKey]*scanmwSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
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
