// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const (
	defaultMatcherMinPoints               = 30
	defaultMatcherMaxPoints               = 120
	defaultMatcherTransitionConfirmations = 3

	matcherNoiseFloorFrac = 0.002 // 0.2% of |level|
	matcherNoiseFloorAbs  = 1e-6
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

	predicted := m.classify(visible, &m.workspace)
	if predicted == observerdef.UNSET {
		predicted = observerdef.UNKNOWN
	}

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
	if denomX <= 1e-12 {
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
	if sseConst > 1e-12 {
		r2 = 1.0 - sseLin/sseConst
	}
	maxStepFrac := 0.0
	if totalAbsStep > 0 {
		maxStepFrac = maxAbsStep / totalAbsStep
	}

	// Only compute residual statistics when a linear fit is still eligible.
	if r2 >= 0.85 && maxStepFrac < 0.45 {
		resMed := ws.median(ws.residuals)
		resMAD := ws.mad(ws.residuals, resMed) * 1.4826
		effResNoise := math.Max(resMAD, scaleFloor)
		if drift > math.Max(3.5*effResNoise, 2.0*scaleFloor) {
			third := n / 3
			var midResSum float64
			if third >= 2 {
				midResMed := ws.median(ws.residuals[third : 2*third])
				outerResMed := 0.5 * (ws.median(ws.residuals[:third]) + ws.median(ws.residuals[2*third:]))
				midResSum = math.Abs(midResMed - outerResMed)
			}
			if midResSum <= 0.25*drift {
				return observerdef.LINEAR
			}
		}
	}

	// Negation also rejects NaN, unlike r2 >= 0.65.
	if !(r2 < 0.65) {
		return observerdef.UNKNOWN
	}
	constMAD := ws.mad(ws.values, med) * 1.4826
	effConstNoise := math.Max(constMAD, scaleFloor)
	relSpread := effConstNoise / math.Max(math.Abs(med), 1.0)
	if !(drift <= math.Max(2.5*effConstNoise, scaleFloor*2.0) && relSpread <= 0.08) {
		return observerdef.UNKNOWN
	}

	// Compare plateau halves only after the cheaper stable-shape checks pass.
	mid := n / 2
	firstHalfMed := ws.median(ws.values[:mid])
	secondHalfMed := ws.median(ws.values[mid:])
	firstHalfMAD := ws.mad(ws.values[:mid], firstHalfMed) * 1.4826
	secondHalfMAD := ws.mad(ws.values[mid:], secondHalfMed) * 1.4826
	localNoise := math.Max(math.Max(firstHalfMAD, secondHalfMAD), scaleFloor)
	halfGap := math.Abs(secondHalfMed - firstHalfMed)
	if halfGap <= math.Max(3.0*localNoise, scaleFloor*2.5) {
		return observerdef.STABLE
	}

	return observerdef.UNKNOWN
}
