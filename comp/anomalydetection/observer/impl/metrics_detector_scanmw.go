// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
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
	lastObservedTime   int64
	observedPoints     int // cumulative completed points, independent of storage eviction
	lastProcessedCount int // cumulative point count at the last scan

	// Segment tracking: only scan [segmentStartTime, dataTime].
	// 0 initially (scan full history), advances to changepoint timestamp on fire.
	segmentStartTime int64
}

// ScanMWConfig contains construction-time statistical parameters.
// Only MinPoints and MaxPoints are exposed through Agent/testbench settings.
type ScanMWConfig struct {
	MinSegment            int     `json:"-"` // minimum points on each side of a split
	MinPoints             int     `json:"min_points"`
	MaxPoints             int     `json:"max_points"`
	SignificanceThreshold float64 `json:"-"` // maximum accepted p-value
	MinEffectSize         float64 `json:"-"` // minimum absolute rank-biserial correlation
	MinDeviationMAD       float64 `json:"-"` // median separation in pre-segment MADs
}

// DefaultScanMWConfig preserves unrestricted detection on averages and counts.
func DefaultScanMWConfig() ScanMWConfig {
	return ScanMWConfig{
		MinSegment:            12,
		MinPoints:             30,
		MaxPoints:             scanMaxPoints,
		SignificanceThreshold: 1e-8,
		MinEffectSize:         0.85,
		MinDeviationMAD:       3.0,
	}
}

// ScanMWDetector scans candidate splits with a Mann-Whitney U test using ranks
// assigned once per scan (O(n log n)). After a changepoint, only the new segment
// is scanned. Configuration is immutable; state is single-writer.
type ScanMWDetector struct {
	config ScanMWConfig
	ready  bool

	series map[scanmwStateKey]*scanmwSeriesState
	// Shared scratch avoids retaining a scan buffer per series and aggregation.
	scanBuf   []observer.Point
	workspace scanDetectorWorkspace

	// Cache the discovered series list across Detect calls.
	cachedRefs []observer.SeriesRef
	cachedGen  uint64
}

// NewScanMWDetector normalizes and copies configuration once at construction.
func NewScanMWDetector(cfg ScanMWConfig) *ScanMWDetector {
	defaults := DefaultScanMWConfig()
	if cfg.MinSegment <= 0 {
		cfg.MinSegment = defaults.MinSegment
	}
	if cfg.MinPoints <= 0 {
		cfg.MinPoints = defaults.MinPoints
	}
	if cfg.MaxPoints <= 0 {
		cfg.MaxPoints = defaults.MaxPoints
	}
	if cfg.MaxPoints < cfg.MinPoints {
		logging.Warnf("ScanMW max_points=%d is below min_points=%d; using %d", cfg.MaxPoints, cfg.MinPoints, cfg.MinPoints)
		cfg.MaxPoints = cfg.MinPoints
	}
	if cfg.SignificanceThreshold <= 0 {
		cfg.SignificanceThreshold = defaults.SignificanceThreshold
	}
	if cfg.MinEffectSize <= 0 {
		cfg.MinEffectSize = defaults.MinEffectSize
	}
	if cfg.MinDeviationMAD <= 0 {
		cfg.MinDeviationMAD = defaults.MinDeviationMAD
	}
	return &ScanMWDetector{
		config: cfg,
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
	return observer.DetectorPointWindow{MinPoints: d.config.MinPoints, MaxPoints: d.config.MaxPoints}
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
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range [...]observer.Aggregate{observer.AggregateAverage, observer.AggregateSum, observer.AggregateCount} {
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
func (d *ScanMWDetector) Detect(storage observer.StorageReader, dataTime int64, filter observer.DetectionFilter) observer.DetectionResult {
	aggregations, numAggregations := detectorAggregations(filter.Aggregations)
	gen := storage.SeriesGeneration()
	if d.cachedRefs == nil || gen != d.cachedGen {
		d.cachedRefs = listSeriesRefs(storage, detectorSeriesFilter(filter), d.cachedRefs)
		d.cachedGen = gen
	}

	// Bulk-fetch visible point counts for series activation.
	bulkStatus := bulkSeriesStatus(storage, d.cachedRefs, dataTime)

	var allAnomalies []observer.Anomaly

	for i, ref := range d.cachedRefs {
		status := bulkStatus[i]

		for _, agg := range aggregations[:numAggregations] {
			if !supportsSeriesAggregate(storage, ref, agg) {
				continue
			}
			sk := scanmwStateKey{ref: ref, agg: agg}

			state, exists := d.series[sk]
			if !exists && status.pointCount < d.config.MinPoints {
				continue
			}
			activated := !exists
			if !exists {
				state = &scanmwSeriesState{}
				d.series[sk] = state
			}

			// Count newly visible completed buckets, not storage writes. Preloaded
			// replay has already written future points; streaming writes them as
			// time advances. Retained point counts also stop growing on eviction.
			storage.ForEachPoint(ref, state.lastObservedTime, dataTime, agg, func(_ *observer.Series, p observer.Point) {
				state.observedPoints++
				state.lastObservedTime = p.Timestamp
			})
			if !activated && state.observedPoints < state.lastProcessedCount+d.config.MinSegment {
				continue
			}

			var seriesMeta *observer.Series
			seriesMeta, d.scanBuf = collectLastPoints(storage, ref, dataTime, d.config.MaxPoints, agg, d.scanBuf)
			if state.segmentStartTime > 0 {
				kept := d.scanBuf[:0]
				for _, p := range d.scanBuf {
					if p.Timestamp > state.segmentStartTime {
						kept = append(kept, p)
					}
				}
				d.scanBuf = kept
			}

			if seriesMeta == nil || len(d.scanBuf) < d.config.MinPoints {
				state.lastProcessedCount = state.observedPoints
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
				state.lastProcessedCount = 1 + ((state.observedPoints-1)/d.config.MinSegment)*d.config.MinSegment
			} else {
				state.lastProcessedCount = state.observedPoints
			}
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

	minSeg := d.config.MinSegment
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

	if bestPValue >= d.config.SignificanceThreshold {
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
	if math.Abs(effectSize) < d.config.MinEffectSize {
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
	if deviation < d.config.MinDeviationMAD {
		return observer.Anomaly{}, 0, false
	}

	changePtTime := points[bestK].Timestamp
	score := -math.Log10(bestPValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	anomaly := observer.Anomaly{
		Type:                observer.AnomalyTypeMetric,
		Source:              observer.SeriesDescriptor{Namespace: series.Namespace, Name: series.Name, Host: series.Host, Tags: series.Tags, Aggregate: agg},
		DetectorName:        d.Name(),
		Timestamp:           changePtTime,
		Score:               &score,
		SamplingIntervalSec: d.workspace.medianPointInterval(points),
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: preMedian,
			BaselineMAD:    preMAD,
			CurrentValue:   postMedian,
			DeviationSigma: deviation,
			PValue:         bestPValue,
			EffectSize:     effectSize,
		},
	}

	return anomaly, bestK, true
}
