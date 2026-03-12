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

// mwSeriesState holds per-series streaming state for the Mann-Whitney detector.
type mwSeriesState struct {
	key observer.SeriesKey
	agg observer.Aggregate

	// Cursor: how many points we've processed so far.
	lastProcessedTime  int64
	lastProcessedCount int

	// Warmup: accumulate points before running detection.
	initialized  bool
	warmupCount  int
	warmupBuffer []float64

	// Baseline: the first WindowSize points from warmup (fixed after init).
	baselineValues []float64
	baselineMean   float64
	baselineStddev float64

	// Sliding window: the most recent WindowSize points (rolling buffer).
	recentBuffer []float64
	recentPos    int // next write position in circular buffer
	recentFull   bool

	// Alert lifecycle.
	inAlert       bool
	alertStart    int64
	recoveryCount int
}

// MannWhitneyDetector uses the Mann-Whitney U test as a streaming changepoint
// detector. It maintains per-series state and processes only new points on
// each advance, comparing a fixed baseline window against a sliding recent window.
//
// Reference: Mann & Whitney (1947). Non-parametric two-sample test.
//
// Key properties:
//   - Non-parametric: no Gaussian assumption
//   - Rank-based, robust to outliers
//   - Distribution-free under H0
//
// Precision-focused: uses multiple layered filters (statistical significance,
// effect size, deviation sigma, and relative change) to ensure only
// practically meaningful changepoints are reported.
type MannWhitneyDetector struct {
	// MinPoints is the minimum number of points in the after-window before
	// detection runs. Default: 50
	MinPoints int

	// WindowSize is the number of points in each comparison window.
	// Default: 60
	WindowSize int

	// SignificanceThreshold is the p-value below which a changepoint is flagged.
	// Default: 1e-12
	SignificanceThreshold float64

	// MinEffectSize is the minimum |rank-biserial correlation| (effect size).
	// Default: 0.95
	MinEffectSize float64

	// MinDeviationSigma is the minimum |median_after - median_before| / MAD_before.
	// Default: 3.0
	MinDeviationSigma float64

	// MinRelativeChange is the minimum |mean_after - mean_before| / max(|mean_before|, 1e-6).
	// Default: 0.20
	MinRelativeChange float64

	// WarmupPoints is the number of initial points before detection begins.
	// Default: WindowSize * 2 + MinPoints
	WarmupPoints int

	// RecoveryPoints is how many consecutive non-triggering checks are needed
	// to exit alert state. Default: 10
	RecoveryPoints int

	// Aggregations to run detection on. Default: [Average, Count]
	Aggregations []observer.Aggregate

	// per-series state keyed by "namespace|name|tags|agg"
	series map[string]*mwSeriesState
}

// NewMannWhitneyDetector creates a MannWhitneyDetector with default settings.
func NewMannWhitneyDetector() *MannWhitneyDetector {
	return &MannWhitneyDetector{
		MinPoints:             50,
		WindowSize:            60,
		SignificanceThreshold: 1e-12,
		MinEffectSize:         0.95,
		MinDeviationSigma:     3.0,
		MinRelativeChange:     0.20,
		RecoveryPoints:        10,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
		series: make(map[string]*mwSeriesState),
	}
}

// Name returns the detector name.
func (m *MannWhitneyDetector) Name() string {
	return "mannwhitney_detector"
}

// Detect implements Detector. It discovers series, reads only new points,
// and updates per-series state incrementally.
func (m *MannWhitneyDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	m.ensureDefaults()

	seriesKeys := storage.ListSeries(observer.SeriesFilter{})

	var allAnomalies []observer.Anomaly

	for _, key := range seriesKeys {
		for _, agg := range m.Aggregations {
			stateKey := m.stateKey(key, agg)

			state, exists := m.series[stateKey]
			if !exists {
				state = m.newSeriesState(key, agg)
				m.series[stateKey] = state
			}

			visibleCount := storage.PointCountUpTo(key, dataTime)
			if visibleCount <= state.lastProcessedCount {
				continue
			}

			series := storage.GetSeriesRange(key, state.lastProcessedTime, dataTime, agg)
			if series == nil || len(series.Points) == 0 {
				continue
			}

			for _, p := range series.Points {
				anomaly := m.processPoint(state, p, series)
				if anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
			}

			state.lastProcessedTime = series.Points[len(series.Points)-1].Timestamp
			state.lastProcessedCount = visibleCount
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// Reset clears all per-series state for replay/reanalysis.
func (m *MannWhitneyDetector) Reset() {
	m.series = make(map[string]*mwSeriesState)
}

// processPoint handles a single new observation for a series.
func (m *MannWhitneyDetector) processPoint(state *mwSeriesState, p observer.Point, series *observer.Series) *observer.Anomaly {
	x := p.Value

	// Phase 1: Warmup — accumulate baseline + initial recent window.
	if !state.initialized {
		return m.warmupPoint(state, x)
	}

	// Phase 2: Add point to sliding recent window.
	m.addToRecentBuffer(state, x)

	// Phase 3: Run detection if we have enough recent points.
	if !state.recentFull {
		return nil
	}

	triggered := m.runTest(state)

	// Phase 4: Alert lifecycle.
	if triggered {
		state.recoveryCount = 0
		if !state.inAlert {
			state.inAlert = true
			state.alertStart = p.Timestamp
			return m.makeAnomaly(state, p, series)
		}
		return nil
	}

	if state.inAlert {
		state.recoveryCount++
		if state.recoveryCount >= m.RecoveryPoints {
			state.inAlert = false
			state.recoveryCount = 0
		}
	}
	return nil
}

// warmupPoint accumulates a point during warmup.
func (m *MannWhitneyDetector) warmupPoint(state *mwSeriesState, x float64) *observer.Anomaly {
	state.warmupCount++
	state.warmupBuffer = append(state.warmupBuffer, x)

	if state.warmupCount >= m.WarmupPoints {
		m.initializeFromWarmup(state)
	}
	return nil
}

// initializeFromWarmup sets up baseline and recent window from warmup data.
func (m *MannWhitneyDetector) initializeFromWarmup(state *mwSeriesState) {
	ws := m.WindowSize

	// Baseline: first WindowSize points.
	state.baselineValues = make([]float64, ws)
	copy(state.baselineValues, state.warmupBuffer[:ws])
	state.baselineMean = detectorMeanValues(state.baselineValues)
	state.baselineStddev = detectorSampleStddev(state.baselineValues, state.baselineMean)

	// Recent buffer: circular buffer of WindowSize.
	state.recentBuffer = make([]float64, ws)
	remaining := state.warmupBuffer[ws:]
	if len(remaining) >= ws {
		// Fill from the tail of remaining.
		copy(state.recentBuffer, remaining[len(remaining)-ws:])
		state.recentPos = 0
		state.recentFull = true
	} else {
		copy(state.recentBuffer, remaining)
		state.recentPos = len(remaining)
		state.recentFull = false
	}

	state.warmupBuffer = nil // free memory
	state.initialized = true
}

// addToRecentBuffer adds a value to the circular recent buffer.
func (m *MannWhitneyDetector) addToRecentBuffer(state *mwSeriesState, x float64) {
	state.recentBuffer[state.recentPos] = x
	state.recentPos++
	if state.recentPos >= m.WindowSize {
		state.recentPos = 0
		state.recentFull = true
	}
}

// recentValues returns the current recent window values in order.
func (m *MannWhitneyDetector) recentValues(state *mwSeriesState) []float64 {
	ws := m.WindowSize
	vals := make([]float64, ws)
	if state.recentFull {
		// Circular buffer: pos..end then 0..pos-1
		copy(vals, state.recentBuffer[state.recentPos:])
		copy(vals[ws-state.recentPos:], state.recentBuffer[:state.recentPos])
	} else {
		copy(vals, state.recentBuffer[:state.recentPos])
	}
	return vals
}

// runTest runs the Mann-Whitney U test: baseline vs recent window.
// Returns true if all 4 filters pass.
func (m *MannWhitneyDetector) runTest(state *mwSeriesState) bool {
	afterVals := m.recentValues(state)
	beforeVals := state.baselineValues

	u, pValue := mannWhitneyU(beforeVals, afterVals)

	// Filter 1: statistical significance.
	if pValue >= m.SignificanceThreshold {
		return false
	}

	// Filter 2: effect size.
	effectSize := rankBiserialCorrelation(u, len(beforeVals), len(afterVals))
	if math.Abs(effectSize) < m.MinEffectSize {
		return false
	}

	// Filter 3: robust deviation (median/MAD).
	beforeMedian := detectorMedian(beforeVals)
	beforeMAD := detectorMAD(beforeVals, beforeMedian, true)
	afterMedian := detectorMedian(afterVals)

	deviation := 0.0
	if beforeMAD > 1e-10 {
		deviation = (afterMedian - beforeMedian) / beforeMAD
	} else if math.Abs(beforeMedian) > 1e-10 {
		deviation = (afterMedian - beforeMedian) / (math.Abs(beforeMedian) * 0.05)
	}
	if math.Abs(deviation) < m.MinDeviationSigma {
		return false
	}

	// Filter 4: relative change.
	afterMean := detectorMeanValues(afterVals)
	absBaseline := math.Abs(state.baselineMean)
	if absBaseline < 1e-6 {
		absBaseline = 1e-6
	}
	relChange := math.Abs(afterMean-state.baselineMean) / absBaseline
	if relChange < m.MinRelativeChange {
		return false
	}

	return true
}

// makeAnomaly constructs an Anomaly for a new alert onset.
func (m *MannWhitneyDetector) makeAnomaly(state *mwSeriesState, p observer.Point, series *observer.Series) *observer.Anomaly {
	afterVals := m.recentValues(state)
	afterMean := detectorMeanValues(afterVals)
	beforeMedian := detectorMedian(state.baselineValues)
	beforeMAD := detectorMAD(state.baselineValues, beforeMedian, true)
	afterMedian := detectorMedian(afterVals)

	deviation := 0.0
	if beforeMAD > 1e-10 {
		deviation = (afterMedian - beforeMedian) / beforeMAD
	} else if math.Abs(beforeMedian) > 1e-10 {
		deviation = (afterMedian - beforeMedian) / (math.Abs(beforeMedian) * 0.05)
	}

	u, pValue := mannWhitneyU(state.baselineValues, afterVals)
	effectSize := rankBiserialCorrelation(u, len(state.baselineValues), len(afterVals))

	absBaseline := math.Abs(state.baselineMean)
	if absBaseline < 1e-6 {
		absBaseline = 1e-6
	}
	relChange := math.Abs(afterMean-state.baselineMean) / absBaseline

	direction := "increased"
	if afterMean < state.baselineMean {
		direction = "decreased"
	}

	seriesName := series.Name + ":" + aggSuffix(state.agg)
	score := -math.Log10(pValue)
	if math.IsInf(score, 1) {
		score = 300.0
	}

	return &observer.Anomaly{
		Type:           observer.AnomalyTypeMetric,
		Source:         observer.MetricName(seriesName),
		SourceSeriesID: observer.SeriesID(seriesKey(series.Namespace, seriesName, series.Tags)),
		DetectorName:   m.Name(),
		Title:          fmt.Sprintf("Mann-Whitney changepoint: %s", seriesName),
		Description: fmt.Sprintf("%s %s from %.2f to %.2f (p=%.2e, U=%.0f, effect=%.2f, %.1fσ, relΔ=%.1f%%)",
			seriesName, direction, state.baselineMean, afterMean, pValue, u, effectSize, deviation, relChange*100),
		Tags:      series.Tags,
		Timestamp: p.Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   state.baselineMean,
			BaselineStddev: state.baselineStddev,
			Threshold:      m.SignificanceThreshold,
			CurrentValue:   afterMean,
			DeviationSigma: deviation,
		},
	}
}

// stateKey returns a unique key for per-series state tracking.
func (m *MannWhitneyDetector) stateKey(key observer.SeriesKey, agg observer.Aggregate) string {
	return seriesKey(key.Namespace, key.Name, key.Tags) + "|" + aggSuffix(agg)
}

// newSeriesState creates a fresh per-series state entry.
func (m *MannWhitneyDetector) newSeriesState(key observer.SeriesKey, agg observer.Aggregate) *mwSeriesState {
	return &mwSeriesState{
		key: key,
		agg: agg,
	}
}

// ensureDefaults fills in zero-valued config fields with sensible defaults.
func (m *MannWhitneyDetector) ensureDefaults() {
	if m.MinPoints <= 0 {
		m.MinPoints = 50
	}
	if m.WindowSize <= 0 {
		m.WindowSize = 60
	}
	if m.SignificanceThreshold <= 0 {
		m.SignificanceThreshold = 1e-12
	}
	if m.MinEffectSize <= 0 {
		m.MinEffectSize = 0.95
	}
	if m.MinDeviationSigma <= 0 {
		m.MinDeviationSigma = 3.0
	}
	if m.MinRelativeChange < 0 {
		m.MinRelativeChange = 0.20
	}
	if m.WarmupPoints <= 0 {
		m.WarmupPoints = m.WindowSize*2 + m.MinPoints
	}
	if m.RecoveryPoints <= 0 {
		m.RecoveryPoints = 10
	}
	if m.series == nil {
		m.series = make(map[string]*mwSeriesState)
	}
	if len(m.Aggregations) == 0 {
		m.Aggregations = []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		}
	}
}

// extractValues extracts float64 values from a slice of Points.
func extractValues(points []observer.Point) []float64 {
	vals := make([]float64, len(points))
	for i, p := range points {
		vals[i] = p.Value
	}
	return vals
}

// mannWhitneyU computes the Mann-Whitney U statistic and approximate p-value
// using the normal approximation with continuity correction and tie correction.
// Returns (U, p-value) where U is min(U1, U2).
func mannWhitneyU(x, y []float64) (float64, float64) {
	n1 := len(x)
	n2 := len(y)
	if n1 == 0 || n2 == 0 {
		return 0, 1.0
	}

	// Combine and rank
	type rankedValue struct {
		value float64
		group int // 0 = x, 1 = y
	}

	N := n1 + n2
	combined := make([]rankedValue, 0, N)
	for _, v := range x {
		combined = append(combined, rankedValue{value: v, group: 0})
	}
	for _, v := range y {
		combined = append(combined, rankedValue{value: v, group: 1})
	}

	sort.Slice(combined, func(i, j int) bool {
		return combined[i].value < combined[j].value
	})

	// Assign ranks with tie averaging
	ranks := make([]float64, N)
	tieCorrection := 0.0

	i := 0
	for i < N {
		j := i
		for j < N && combined[j].value == combined[i].value {
			j++
		}
		// Positions i..j-1 are tied; average rank = (i+1 + j) / 2
		avgRank := float64(i+1+j) / 2.0
		tieSize := float64(j - i)
		for k := i; k < j; k++ {
			ranks[k] = avgRank
		}
		// Tie correction term: sum of (t^3 - t) for each tie group
		tieCorrection += tieSize*tieSize*tieSize - tieSize
		i = j
	}

	// Sum ranks for group x
	var R1 float64
	for k := 0; k < N; k++ {
		if combined[k].group == 0 {
			R1 += ranks[k]
		}
	}

	// U statistics
	fn1 := float64(n1)
	fn2 := float64(n2)
	U1 := R1 - fn1*(fn1+1)/2
	U2 := fn1*fn2 - U1

	U := math.Min(U1, U2)

	// Normal approximation
	meanU := fn1 * fn2 / 2
	fN := float64(N)
	// Variance with tie correction
	varU := (fn1 * fn2 / 12) * (fN + 1 - tieCorrection/(fN*(fN-1)))
	if varU <= 0 {
		return U, 1.0
	}
	stdU := math.Sqrt(varU)

	// Z-score with continuity correction
	z := (math.Abs(U-meanU) - 0.5) / stdU
	if z < 0 {
		z = 0
	}

	// Two-tailed p-value using normal CDF approximation
	pValue := 2 * normalCDFUpper(z)
	if pValue > 1.0 {
		pValue = 1.0
	}

	return U, pValue
}

// rankBiserialCorrelation computes the rank-biserial correlation coefficient
// as an effect size measure. Range: [-1, 1].
func rankBiserialCorrelation(u float64, n1, n2 int) float64 {
	fn1 := float64(n1)
	fn2 := float64(n2)
	product := fn1 * fn2
	if product == 0 {
		return 0
	}
	return 1 - 2*u/product
}

// normalCDFUpper computes P(Z > z) for z >= 0 using the Abramowitz & Stegun approximation.
func normalCDFUpper(z float64) float64 {
	if z < 0 {
		return 1 - normalCDFUpper(-z)
	}
	// Rational approximation (Abramowitz & Stegun 26.2.17)
	const (
		p  = 0.2316419
		b1 = 0.319381530
		b2 = -0.356563782
		b3 = 1.781477937
		b4 = -1.821255978
		b5 = 1.330274429
	)
	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t
	phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
	return phi * (b1*t + b2*t2 + b3*t3 + b4*t4 + b5*t5)
}
