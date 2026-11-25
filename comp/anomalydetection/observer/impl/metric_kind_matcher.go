// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const (
	defaultMatcherMinPoints               = 30
	defaultMatcherMaxPoints               = 30
	defaultMatcherTransitionConfirmations = 3

	matcherNoiseFloorFrac  = 0.002 // 0.2% of |level|
	matcherNoiseFloorAbs   = 1e-6
	matcherMADToSigma      = 1.4826
	matcherFitDenomEpsilon = 1e-12
	matcherVarianceEpsilon = 1e-12

	// A linear model needs a strong fit and distributed, low-curvature movement.
	matcherLinearMinRSquared          = 0.85
	matcherLinearMaxStepFraction      = 0.45
	matcherLinearMinDriftNoiseRatio   = 3.5
	matcherLinearMinDriftFloorRatio   = 2.0
	matcherLinearMaxCurvatureFraction = 0.25

	// A plateau needs limited relative noise, drift and separation between halves.
	matcherStableMaxRSquared          = 0.65
	matcherStableMaxRelativeNoise     = 0.08
	matcherRelativeNoiseLevelFloor    = 1.0
	matcherStableMaxDriftNoiseRatio   = 2.5
	matcherStableMaxDriftFloorRatio   = 2.0
	matcherStableMaxHalfGapNoiseRatio = 3.0
	matcherStableMaxHalfGapFloorRatio = 2.5
	matcherStableEdgeShiftNoiseRatio  = 3.0
)

// DefaultMetricKindMatcherConfig returns the default MetricKindMatcherConfig.
func DefaultMetricKindMatcherConfig() observerdef.MetricKindMatcherConfig {
	return observerdef.MetricKindMatcherConfig{
		MinPoints:               defaultMatcherMinPoints,
		MaxPoints:               defaultMatcherMaxPoints,
		TransitionConfirmations: defaultMatcherTransitionConfirmations,
	}
}

type metricKindCandidateState struct {
	candidate       observerdef.MetricKind
	confirmations   int
	lastEvaluatedTs int64
}

// metricKindClassifierFunc classifies a chronological window of points into STABLE, LINEAR, or UNKNOWN.
type metricKindClassifierFunc func(points []observerdef.Point, scratch *matcherWorkspace) observerdef.MetricKind

type matcherWorkspace struct {
	values    []float64
	residuals []float64
	sorted    []float64
}

func (w *matcherWorkspace) median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	w.sorted = reuseFloat64(w.sorted, len(vals))
	copy(w.sorted, vals)
	sort.Float64s(w.sorted)
	n := len(w.sorted)
	if n%2 == 0 {
		return (w.sorted[n/2-1] + w.sorted[n/2]) / 2
	}
	return w.sorted[n/2]
}

func (w *matcherWorkspace) mad(vals []float64, med float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	w.sorted = reuseFloat64(w.sorted, len(vals))
	for i, v := range vals {
		w.sorted[i] = math.Abs(v - med)
	}
	sort.Float64s(w.sorted)
	n := len(w.sorted)
	if n%2 == 0 {
		return (w.sorted[n/2-1] + w.sorted[n/2]) / 2
	}
	return w.sorted[n/2]
}

// medianTimeOffset uses chronological order without sorting or absolute-time rounding.
func medianTimeOffset(points []observerdef.Point, origin int64) float64 {
	return 0.5 * (float64(points[(len(points)-1)/2].Timestamp-origin) +
		float64(points[len(points)/2].Timestamp-origin))
}

// Consistent edge pairs distinguish a persistent level departure from one spike.
func hasEdgeLevelShift(values []float64, med, tolerance float64) bool {
	for _, start := range [2]int{0, len(values) - 2} {
		first, second := values[start], values[start+1]
		if math.Abs(first-second) > tolerance {
			continue
		}
		if (first > med+tolerance && second > med+tolerance) ||
			(first < med-tolerance && second < med-tolerance) {
			return true
		}
	}
	return false
}

// metricKindMatcher implements observerdef.MetricKindMatcher.
// Committed kinds live in storage; the matcher tracks per-series candidate
// predictions and confirmation counts across fresh observation windows.
type metricKindMatcher struct {
	cfg       observerdef.MetricKindMatcherConfig
	series    map[observerdef.SeriesRef]*metricKindCandidateState
	classify  metricKindClassifierFunc
	workspace matcherWorkspace
}

// NewMetricKindMatcher constructs a stateful MetricKindMatcher with normalized defaults.
func NewMetricKindMatcher(cfg observerdef.MetricKindMatcherConfig) observerdef.MetricKindMatcher {
	return newMetricKindMatcherWithClassifier(cfg, classifyMetricKind)
}

func newMetricKindMatcherWithClassifier(cfg observerdef.MetricKindMatcherConfig, classify metricKindClassifierFunc) *metricKindMatcher {
	defaults := DefaultMetricKindMatcherConfig()
	if cfg.MinPoints <= 0 {
		cfg.MinPoints = defaults.MinPoints
	}
	if cfg.MaxPoints <= 0 {
		cfg.MaxPoints = defaults.MaxPoints
	}
	if cfg.MaxPoints < cfg.MinPoints {
		logging.Warnf("MetricKindMatcher max_points=%d is below min_points=%d; using %d", cfg.MaxPoints, cfg.MinPoints, cfg.MinPoints)
		cfg.MaxPoints = cfg.MinPoints
	}
	if cfg.TransitionConfirmations <= 0 {
		cfg.TransitionConfirmations = defaults.TransitionConfirmations
	}
	if classify == nil {
		classify = classifyMetricKind
	}
	return &metricKindMatcher{
		cfg:      cfg,
		series:   make(map[observerdef.SeriesRef]*metricKindCandidateState),
		classify: classify,
	}
}

// Window returns the bounded point window required for classification.
func (m *metricKindMatcher) Window() observerdef.DetectorPointWindow {
	return observerdef.DetectorPointWindow{
		MinPoints: m.cfg.MinPoints,
		MaxPoints: m.cfg.MaxPoints,
	}
}

// Classify evaluates the supplied window without updating per-series state.
func (m *metricKindMatcher) Classify(points []observerdef.Point) observerdef.MetricKind {
	kind := m.classify(points, &m.workspace)
	if kind == observerdef.UNSET {
		return observerdef.UNKNOWN
	}
	return kind
}

// Observe counts one confirmation per fresh-data evaluation, not per advance or batch point.
func (m *metricKindMatcher) Observe(
	ref observerdef.SeriesRef,
	currentKind observerdef.MetricKind,
	points []observerdef.Point,
	dataTime int64,
) (observerdef.KindTransition, bool) {
	// Restrict to points visible at or before dataTime and bound to MaxPoints tail.
	visible := points
	for len(visible) > 0 && visible[len(visible)-1].Timestamp > dataTime {
		visible = visible[:len(visible)-1]
	}
	if len(visible) < m.cfg.MinPoints {
		return observerdef.KindTransition{}, false
	}
	if len(visible) > m.cfg.MaxPoints {
		visible = visible[len(visible)-m.cfg.MaxPoints:]
	}

	lastTs := visible[len(visible)-1].Timestamp
	state, exists := m.series[ref]
	if exists && lastTs <= state.lastEvaluatedTs {
		// No new completed observations since the last evaluation.
		return observerdef.KindTransition{}, false
	}
	if !exists {
		state = &metricKindCandidateState{}
		m.series[ref] = state
	}
	state.lastEvaluatedTs = lastTs

	predicted := m.Classify(visible)

	// Initial assignment from UNSET does not require transition confirmations.
	if currentKind == observerdef.UNSET {
		state.candidate = observerdef.UNSET
		state.confirmations = 0
		return observerdef.KindTransition{
			Ref:  ref,
			From: observerdef.UNSET,
			To:   predicted,
		}, true
	}

	// Prediction matches the committed kind: clear any pending candidate.
	if predicted == currentKind {
		state.candidate = observerdef.UNSET
		state.confirmations = 0
		return observerdef.KindTransition{}, false
	}

	if state.candidate == predicted {
		state.confirmations++
	} else {
		state.candidate = predicted
		state.confirmations = 1
	}

	if state.confirmations < m.cfg.TransitionConfirmations {
		return observerdef.KindTransition{}, false
	}

	state.candidate = observerdef.UNSET
	state.confirmations = 0
	return observerdef.KindTransition{
		Ref:  ref,
		From: currentKind,
		To:   predicted,
	}, true
}

// RemoveSeries deletes candidate tracking state for evicted series.
func (m *metricKindMatcher) RemoveSeries(refs []observerdef.SeriesRef) {
	if len(refs) == 0 || len(m.series) == 0 {
		return
	}
	for _, ref := range refs {
		delete(m.series, ref)
	}
}

// Reset clears all candidate tracking state.
func (m *metricKindMatcher) Reset() {
	clear(m.series)
}

// classifyMetricKind compares a plateau with a timestamp-based line.
// Robust residual summaries reject some mixed windows, but not every slope break.
func classifyMetricKind(points []observerdef.Point, ws *matcherWorkspace) observerdef.MetricKind {
	n := len(points)
	if n < 3 {
		return observerdef.UNKNOWN
	}

	t0 := points[0].Timestamp
	dtTotal := float64(points[n-1].Timestamp - t0)
	if dtTotal <= 0 {
		return observerdef.UNKNOWN
	}

	ws.values = reuseFloat64(ws.values, n)
	minVal := points[0].Value
	maxVal := points[0].Value
	var sumX, sumY, sumXX, sumXY float64
	for i, p := range points {
		v := p.Value
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return observerdef.UNKNOWN
		}
		ws.values[i] = v
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
		x := float64(p.Timestamp-t0) / dtTotal
		sumX += x
		sumY += v
		sumXX += x * x
		sumXY += x * v
	}

	valRange := maxVal - minVal
	med := ws.median(ws.values)
	scaleFloor := math.Max(math.Abs(med)*matcherNoiseFloorFrac, matcherNoiseFloorAbs)
	if valRange <= scaleFloor {
		return observerdef.STABLE
	}

	fn := float64(n)
	denomX := fn*sumXX - sumX*sumX
	if denomX <= matcherFitDenomEpsilon {
		return observerdef.UNKNOWN
	}
	slope := (fn*sumXY - sumX*sumY) / denomX
	intercept := (sumY - slope*sumX) / fn

	return evaluateFittedShape(points, ws, med, scaleFloor, intercept, slope, dtTotal)
}

func evaluateFittedShape(
	points []observerdef.Point,
	ws *matcherWorkspace,
	med float64,
	scaleFloor float64,
	intercept float64,
	slope float64,
	dtTotal float64,
) observerdef.MetricKind {
	n := len(points)
	t0 := points[0].Timestamp
	ws.residuals = reuseFloat64(ws.residuals, n)

	var sseConst, sseLin float64
	meanVal := 0.0
	for _, v := range ws.values {
		meanVal += v
	}
	meanVal /= float64(n)

	var totalAbsStep, maxAbsStep float64
	for i, p := range points {
		x := float64(p.Timestamp-t0) / dtTotal
		pred := intercept + slope*x
		res := p.Value - pred
		ws.residuals[i] = res
		dc := p.Value - meanVal
		sseConst += dc * dc
		sseLin += res * res
		if i > 0 {
			step := p.Value - points[i-1].Value
			absStep := math.Abs(step)
			totalAbsStep += absStep
			if absStep > maxAbsStep {
				maxAbsStep = absStep
			}
		}
	}

	drift := math.Abs(slope)
	r2 := 0.0
	if sseConst > matcherVarianceEpsilon {
		r2 = 1.0 - sseLin/sseConst
	}
	maxStepFrac := 0.0
	if totalAbsStep > 0 {
		maxStepFrac = maxAbsStep / totalAbsStep
	}

	// Only compute residual statistics when a linear fit is still eligible.
	if r2 >= matcherLinearMinRSquared && maxStepFrac < matcherLinearMaxStepFraction {
		resMed := ws.median(ws.residuals)
		resMAD := ws.mad(ws.residuals, resMed) * matcherMADToSigma
		effResNoise := math.Max(resMAD, scaleFloor)
		if drift > math.Max(matcherLinearMinDriftNoiseRatio*effResNoise, matcherLinearMinDriftFloorRatio*scaleFloor) {
			third := n / 3
			var midResSum float64
			if third >= 2 {
				midResMed := ws.median(ws.residuals[third : 2*third])
				outerResMed := 0.5 * (ws.median(ws.residuals[:third]) + ws.median(ws.residuals[2*third:]))
				midResSum = math.Abs(midResMed - outerResMed)
			}
			if midResSum <= matcherLinearMaxCurvatureFraction*drift {
				return observerdef.LINEAR
			}
		}
	}

	// Negation also rejects NaN fit scores.
	if !(r2 < matcherStableMaxRSquared) {
		return observerdef.UNKNOWN
	}
	constMAD := ws.mad(ws.values, med) * matcherMADToSigma
	effConstNoise := math.Max(constMAD, scaleFloor)
	relSpread := effConstNoise / math.Max(math.Abs(med), matcherRelativeNoiseLevelFloor)
	if !(relSpread <= matcherStableMaxRelativeNoise) {
		return observerdef.UNKNOWN
	}
	if hasEdgeLevelShift(ws.values, med, matcherStableEdgeShiftNoiseRatio*effConstNoise) {
		return observerdef.UNKNOWN
	}

	// Half-window medians prevent isolated spikes from tilting a plateau's drift.
	mid := n / 2
	firstHalfMed := ws.median(ws.values[:mid])
	secondHalfMed := ws.median(ws.values[mid:])
	halfGap := math.Abs(secondHalfMed - firstHalfMed)
	halfTimeGap := medianTimeOffset(points[mid:], t0) - medianTimeOffset(points[:mid], t0)
	if halfTimeGap <= 0 {
		return observerdef.UNKNOWN
	}
	plateauDrift := halfGap * (dtTotal / halfTimeGap)
	if !(plateauDrift <= math.Max(matcherStableMaxDriftNoiseRatio*effConstNoise, matcherStableMaxDriftFloorRatio*scaleFloor)) {
		return observerdef.UNKNOWN
	}
	firstHalfMAD := ws.mad(ws.values[:mid], firstHalfMed) * matcherMADToSigma
	secondHalfMAD := ws.mad(ws.values[mid:], secondHalfMed) * matcherMADToSigma
	localNoise := math.Max(math.Max(firstHalfMAD, secondHalfMAD), scaleFloor)
	if halfGap <= math.Max(matcherStableMaxHalfGapNoiseRatio*localNoise, matcherStableMaxHalfGapFloorRatio*scaleFloor) {
		return observerdef.STABLE
	}

	return observerdef.UNKNOWN
}
