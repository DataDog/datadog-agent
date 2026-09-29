// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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
	lastWriteGen       int64
	lastProcessedCount int // visible point count (PointCountUpTo) at last Detect

	// Segment tracking: only scan [segmentStartTime, dataTime].
	// 0 initially (scan full history), advances to changepoint timestamp on fire.
	segmentStartTime int64
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
	ready bool
	// MinSegment is the minimum number of points in each segment.
	// Default: 12
	MinSegment int

	// MinPoints is the minimum total points before detection runs.
	// Default: 30
	MinPoints int `json:"min_points"`
	// MaxPoints bounds the scan window. Default: 120.
	MaxPoints int `json:"max_points"`

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
	// scanBuf is shared by this single-writer detector instead of being retained
	// once per live series and aggregation.
	scanBuf   []observer.Point
	workspace scanDetectorWorkspace

	// Cache the discovered series list across Detect calls.
	cachedRefs []observer.SeriesRef
	cachedGen  uint64
}

// NewScanMWDetector creates a ScanMW detector with default settings.
func NewScanMWDetector() *ScanMWDetector {
	return &ScanMWDetector{
		MinSegment:            12,
		MinPoints:             30,
		MaxPoints:             scanMaxPoints,
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

func (d *ScanMWDetector) Ready() bool { return d.ready }

// DetectorPointWindow implements observer.DetectorPointWindowRequirement.
func (d *ScanMWDetector) DetectorPointWindow() observer.DetectorPointWindow {
	d.ensureDefaults()
	return observer.DetectorPointWindow{MinPoints: d.MinPoints, MaxPoints: d.MaxPoints}
}

// Reset clears all per-series state for replay/reanalysis.
func (d *ScanMWDetector) Reset() {
	d.series = make(map[scanmwStateKey]*scanmwSeriesState)
	d.cachedRefs = nil
	d.cachedGen = 0
	d.ready = false
}

// RemoveSeries drops segment-tracking state for refs that storage has freed.
func (d *ScanMWDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.Aggregations {
			delete(d.series, scanmwStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedRefs = nil
	d.cachedGen = 0
}

// Detect implements Detector. It discovers series, reads segment data,
// and scans for changepoints. After finding one, the segment start advances
// so subsequent calls only examine post-change data.
//
// Iteration pattern is the same as BOCPD (metrics_detector_bocpd.go Detect loop)
// and ScanWelch — consider dedup if more scan-based detectors are added.
func (d *ScanMWDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedRefs == nil || gen != d.cachedGen {
		d.cachedRefs = workloadSeriesRefs(storage, d.cachedRefs)
		d.cachedGen = gen
	}

	// Bulk-fetch write generations in a single lock acquisition.
	bulkStatus := bulkSeriesStatus(storage, d.cachedRefs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, ref := range d.cachedRefs {
		status := bulkStatus[i]

		for _, agg := range d.Aggregations {
			if !supportsSeriesAggregate(storage, ref, agg) {
				continue
			}
			sk := scanmwStateKey{ref: ref, agg: agg}

			state, exists := d.series[sk]
			if !exists && status.pointCount < d.MinPoints {
				continue
			}
			activated := !exists
			if !exists {
				state = &scanmwSeriesState{}
				d.series[sk] = state
			}

			// Skip unless at least MinSegment new points are visible since the
			// last scan. Gating on visible point count (not WriteGeneration
			// alone) is required for replay: storage may already hold future
			// points, so WriteGeneration reaches its final value before the
			// simulated dataTime has advanced far enough to expose them, which
			// would otherwise suppress every scan. WriteGeneration is still
			// checked to catch same-bucket merges that leave the count
			// unchanged but move stored values. Mirrors the BOCPD replay gate.
			if status.pointCount < state.lastProcessedCount+d.MinSegment && status.writeGeneration == state.lastWriteGen {
				continue
			}

			var seriesMeta *observer.Series
			seriesMeta, d.scanBuf = collectLastPoints(storage, ref, dataTime, d.MaxPoints, agg, d.scanBuf)
			if state.segmentStartTime > 0 {
				kept := d.scanBuf[:0]
				for _, p := range d.scanBuf {
					if p.Timestamp > state.segmentStartTime {
						kept = append(kept, p)
					}
				}
				d.scanBuf = kept
			}

			if seriesMeta == nil || len(d.scanBuf) < d.MinPoints {
				state.lastProcessedCount = status.pointCount
				state.lastWriteGen = status.writeGeneration
				continue
			}
			d.ready = true

			anomaly, changeIdx, found := d.scanMW(d.scanBuf, seriesMeta, agg)
			if found {
				anomaly.SourceRef = &observer.QueryHandle{Ref: ref, Aggregate: agg}
				allAnomalies = append(allAnomalies, anomaly)
				state.segmentStartTime = d.scanBuf[changeIdx].Timestamp - 1
			}

			if activated {
				state.lastProcessedCount = 1 + ((status.pointCount-1)/d.MinSegment)*d.MinSegment
			} else {
				state.lastProcessedCount = status.pointCount
			}
			state.lastWriteGen = status.writeGeneration
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// scanMW runs the scan algorithm on points within the current segment.
// Returns (anomaly, changeIndex, found). Pure function over the input data.
func (d *ScanMWDetector) scanMW(points []observer.Point, series *observer.Series, agg observer.Aggregate) (observer.Anomaly, int, bool) {
	n := len(points)

	values := d.workspace.valuesFromPoints(points)

	// Efficient O(n log n) scan: assign ranks once, then slide the split point.
	ranks, tieCorrection := d.workspace.assignRanks(values)

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
	preMedian := d.workspace.median(preVals)
	postMedian := d.workspace.median(postVals)
	preMAD := d.workspace.mad(preVals, preMedian)

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
		Source:       observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Host: series.Host, Tags: series.Tags, Aggregate: agg},
		DetectorName: d.Name(),
		Title:        "ScanMW changepoint: " + seriesName,
		Description: fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, p=%.2e, effect=%.2f, %.1f MADs)",
			seriesName, direction, preMedian, postMedian, bestPValue, effectSize, deviation),
		Timestamp:           changePtTime,
		Score:               &score,
		SamplingIntervalSec: d.workspace.medianPointInterval(points),
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
	if d.MaxPoints <= 0 {
		d.MaxPoints = scanMaxPoints
	}
	if d.MaxPoints < d.MinPoints {
		pkglog.Warnf("[observer] ScanMW max_points=%d is below min_points=%d; using %d", d.MaxPoints, d.MinPoints, d.MinPoints)
		d.MaxPoints = d.MinPoints
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
		d.series = make(map[scanmwStateKey]*scanmwSeriesState)
	}
	if len(d.Aggregations) == 0 {
		d.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}
