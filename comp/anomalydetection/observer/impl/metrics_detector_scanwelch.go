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

// scanwelchStateKey identifies per-series state by ref and aggregation.
type scanwelchStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// scanwelchSeriesState holds per-series streaming state for the ScanWelch detector.
// Same pattern as scanmwSeriesState and BOCPD (metrics_detector_bocpd.go).
type scanwelchSeriesState struct {
	lastObservedTime   int64
	observedPoints     int // cumulative completed points, independent of storage eviction
	lastProcessedCount int // cumulative point count at the last scan

	// Segment tracking: only scan [segmentStartTime, dataTime].
	segmentStartTime int64
}

// ScanWelchConfig contains construction-time statistical parameters.
type ScanWelchConfig struct {
	// MinSegment is the minimum points on each side of a split.
	MinSegment int `json:"-"`

	// MinPoints is the minimum observations before detection.
	MinPoints int `json:"min_points"`
	// MaxPoints bounds the scan window. Default: 120.
	MaxPoints int `json:"max_points"`

	// MinTStatistic is the candidate |t| threshold.
	MinTStatistic float64 `json:"-"`

	// SignificanceThreshold is the maximum MW p-value for reporting.
	SignificanceThreshold float64 `json:"-"`

	// MinEffectSize is the minimum |rank-biserial correlation|.
	MinEffectSize float64 `json:"-"`

	// MinDeviationMAD is the minimum |post_median - pre_median| / MAD.
	MinDeviationMAD float64 `json:"-"`
}

// ScanWelchDetector verifies Welch candidates with Mann-Whitney and MAD gates.
type ScanWelchDetector struct {
	ready  bool
	config ScanWelchConfig

	// per-series state keyed by ref+agg
	series map[scanwelchStateKey]*scanwelchSeriesState
	// scanBuf is shared by this single-writer detector instead of being retained
	// once per live series and aggregation.
	scanBuf   []observer.Point
	workspace scanDetectorWorkspace

	// Cache the discovered series list across Detect calls.
	cachedRefs []observer.SeriesRef
	cachedGen  uint64
}

// DefaultScanWelchConfig returns the default statistical parameters.
func DefaultScanWelchConfig() ScanWelchConfig {
	return ScanWelchConfig{
		MinSegment:            12,
		MinPoints:             30,
		MaxPoints:             scanMaxPoints,
		MinTStatistic:         8.0,
		SignificanceThreshold: 1e-8,
		MinEffectSize:         0.85,
		MinDeviationMAD:       3.0,
	}
}

// NewScanWelchDetector normalizes and copies configuration once at construction.
func NewScanWelchDetector(cfg ScanWelchConfig) *ScanWelchDetector {
	defaults := DefaultScanWelchConfig()
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
		logging.Warnf("ScanWelch max_points=%d is below min_points=%d; using %d", cfg.MaxPoints, cfg.MinPoints, cfg.MinPoints)
		cfg.MaxPoints = cfg.MinPoints
	}
	if cfg.MinTStatistic <= 0 {
		cfg.MinTStatistic = defaults.MinTStatistic
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
	return &ScanWelchDetector{
		config: cfg,
		series: make(map[scanwelchStateKey]*scanwelchSeriesState),
	}
}

// Name returns the detector name.
func (d *ScanWelchDetector) Name() string {
	return "scanwelch"
}

func (d *ScanWelchDetector) Ready() bool { return d.ready }

// DetectorPointWindow implements observer.DetectorPointWindowRequirement.
func (d *ScanWelchDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return observer.DetectorPointWindow{MinPoints: d.config.MinPoints, MaxPoints: d.config.MaxPoints}
}

// Reset clears all per-series state for replay/reanalysis.
func (d *ScanWelchDetector) Reset() {
	d.series = make(map[scanwelchStateKey]*scanwelchSeriesState)
	d.cachedRefs = nil
	d.cachedGen = 0
	d.ready = false
}

// RemoveSeries drops segment-tracking state for refs that storage has freed.
func (d *ScanWelchDetector) RemoveSeries(refs []observer.SeriesRef) {
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range [...]observer.Aggregate{observer.AggregateAverage, observer.AggregateSum, observer.AggregateCount} {
			delete(d.series, scanwelchStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedRefs = nil
	d.cachedGen = 0
}

// Detect implements Detector. Same iteration pattern as ScanMW and BOCPD —
// consider dedup if more scan-based detectors are added.
func (d *ScanWelchDetector) Detect(storage observer.StorageReader, dataTime int64, filter observer.DetectionFilter) observer.DetectionResult {
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
			sk := scanwelchStateKey{ref: ref, agg: agg}

			state, exists := d.series[sk]
			if !exists && status.pointCount < d.config.MinPoints {
				continue
			}
			activated := !exists
			if !exists {
				state = &scanwelchSeriesState{}
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

			anomaly, changeIdx, found := d.scanWelch(d.scanBuf, seriesMeta, agg)
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

// scanWelch runs the hybrid Welch/MW scan on points within the current segment.
// Returns (anomaly, changeIndex, found).
func (d *ScanWelchDetector) scanWelch(points []observer.Point, series *observer.Series, agg observer.Aggregate) (observer.Anomaly, int, bool) {
	n := len(points)
	minSeg := d.config.MinSegment
	// A valid split requires MinSegment points on each side. MinPoints is
	// independently configurable, so a newly activated series may legitimately
	// reach this method before a candidate split exists.
	if n < 2*minSeg {
		return observer.Anomaly{}, 0, false
	}

	values := d.workspace.valuesFromPoints(points)

	// Phase 1: Scan using Welch's t-statistic. Keep the total moments and
	// advance the left-side moments as the split moves, instead of retaining a
	// prefix array for every candidate split.
	var totalSum, totalSumSq float64
	for _, v := range values {
		totalSum += v
		totalSumSq += v * v
	}
	var leftSum, leftSumSq float64
	for _, v := range values[:minSeg] {
		leftSum += v
		leftSumSq += v * v
	}

	bestTAbs := 0.0
	bestK := -1
	for k := minSeg; k <= n-minSeg; k++ {
		fk := float64(k)
		fnk := float64(n - k)

		leftMean := leftSum / fk
		rightSum := totalSum - leftSum
		rightMean := rightSum / fnk

		leftVar := leftSumSq/fk - leftMean*leftMean
		rightVar := (totalSumSq-leftSumSq)/fnk - rightMean*rightMean

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

		if k < n-minSeg {
			v := values[k]
			leftSum += v
			leftSumSq += v * v
		}
	}

	if bestK < 0 || bestTAbs < d.config.MinTStatistic {
		return observer.Anomaly{}, 0, false
	}

	// Phase 2: Verify using Mann-Whitney at the best split point.
	ranks, tieCorrection := d.workspace.assignRanks(values)
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
	if pValue >= d.config.SignificanceThreshold {
		return observer.Anomaly{}, 0, false
	}

	// Effect size check
	effectSize := rankBiserialCorrelation(U, bestK, n-bestK)
	if math.Abs(effectSize) < d.config.MinEffectSize {
		return observer.Anomaly{}, 0, false
	}

	// Phase 3: Robust deviation check
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
	score := -math.Log10(pValue)
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
		LevelChange: &observer.LevelChangeEvidence{
			BeforeMedian: preMedian,
			AfterMedian:  postMedian,
		},
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMedian: preMedian,
			BaselineMAD:    preMAD,
			CurrentValue:   postMedian,
			DeviationSigma: deviation,
			PValue:         pValue,
			EffectSize:     effectSize,
			TestStatistic:  bestTAbs,
		},
	}

	return anomaly, bestK, true
}
