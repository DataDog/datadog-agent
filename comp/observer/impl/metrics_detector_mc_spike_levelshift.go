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

// MCSpikeLevelShiftConfig configures the MC-style spike + level-shift detector.
//
// Defaults are deliberately tighter than production Metric Correlations,
// which prefers false positives. WS2's adaptive-sending-rate use case pays
// real cost per false positive (a 30-min ring-buffer flush), so the defaults
// favour precision.
type MCSpikeLevelShiftConfig struct {
	// BaselinePoints is the number of older points used to build the baseline
	// distribution (p1/p99/mean/std). Default: 240 (~4 minutes at 1Hz).
	BaselinePoints int `json:"baseline_points"`

	// AnomalyPoints is the number of recent points evaluated as the
	// suspected-anomaly window for level-shift detection.
	// Default: 60 (~1 minute at 1Hz).
	AnomalyPoints int `json:"anomaly_points"`

	// StdMultiplier is the spike threshold: a current value triggers a spike
	// candidate only when |x - mean| > StdMultiplier * baseline_std.
	// MC production default is 2.0; we tighten to 5.0.
	StdMultiplier float64 `json:"std_multiplier"`

	// KurtosisMultiplier is the spike threshold for excess kurtosis of the
	// anomaly window relative to the baseline kurtosis. Tightened relative to
	// MC production. Default: 5.0
	KurtosisMultiplier float64 `json:"kurtosis_multiplier"`

	// LowerPercentileCoef multiplies baseline p1 to form the lower bound of
	// "stable" range when LevelShiftMode = "percentile".
	// MC production uses 0.8; we tighten to 0.5.
	LowerPercentileCoef float64 `json:"lower_percentile_coef"`

	// UpperPercentileCoef multiplies baseline p99 to form the upper bound of
	// "stable" range when LevelShiftMode = "percentile".
	// MC production uses 1.2; we tighten to 1.5.
	UpperPercentileCoef float64 `json:"upper_percentile_coef"`

	// LevelShiftMode selects how the level-shift "stable range" is built:
	//   - "percentile" (default, MC-spec): [LowerCoef×p1, UpperCoef×p99]
	//   - "mad":                            [median - K×MAD, median + K×MAD]
	// MAD is robust to outlier-contaminated baselines that inflate p99
	// and would suppress percentile-based detection.
	LevelShiftMode string `json:"level_shift_mode"`

	// MADMultiplier is K in [median ± K×MAD] when LevelShiftMode = "mad".
	// 3.0 covers ≈99.5% of a normal distribution; tighter than that
	// over-fires on heavy-tailed real series. Default: 3.0.
	MADMultiplier float64 `json:"mad_multiplier"`

	// MinAbsDeviation is an additional absolute-magnitude floor: tiny shifts
	// (e.g. 0.001 → 0.0011 on a near-constant series) are suppressed even
	// if they breach the percentile bounds. Expressed as a fraction of the
	// baseline mean magnitude. Default: 0.05 (5%).
	MinAbsDeviation float64 `json:"min_abs_deviation"`

	// RecoveryPoints is the number of consecutive non-triggering points
	// required to leave alert state and become eligible to fire again.
	// Default: 30.
	RecoveryPoints int `json:"recovery_points"`

	// MinAlertGapSec is the minimum data-time gap (seconds) between
	// consecutive alerts on the same series. Acts as a post-recovery
	// hysteresis: even after RecoveryPoints clears the in-alert flag,
	// a new alert is suppressed until this gap has elapsed. Suppresses
	// flutter on noisy series that re-trigger immediately after recovery.
	// Default: 60 (~1 minute at 1Hz).
	MinAlertGapSec int64 `json:"min_alert_gap_sec"`

	// Aggregations to run detection on. Default: [Average].
	Aggregations []observer.Aggregate `json:"-"`
}

// DefaultMCSpikeLevelShiftConfig returns defaults tightened from MC production.
func DefaultMCSpikeLevelShiftConfig() MCSpikeLevelShiftConfig {
	return MCSpikeLevelShiftConfig{
		BaselinePoints:      240,
		AnomalyPoints:       60,
		StdMultiplier:       5.0,
		KurtosisMultiplier:  5.0,
		LowerPercentileCoef: 0.5,
		UpperPercentileCoef: 1.5,
		// Default = percentile (matches MC spec). MAD is opt-in via
		// LevelShiftMode = "mad". Eval (5/5/2026) showed MAD regressing
		// system F1 by ~0.6 pp on the 12-scenario corpus — kept available
		// for outlier-contaminated baselines but not the default.
		LevelShiftMode: "percentile",
		MADMultiplier:  3.0,
		MinAbsDeviation:     0.05,
		RecoveryPoints:      30,
		MinAlertGapSec:      60,
		Aggregations: []observer.Aggregate{
			observer.AggregateAverage,
			observer.AggregateCount,
		},
	}
}

// mcStateKey uniquely identifies a (series, aggregation) pair for MC detector state.
type mcStateKey struct {
	ref observer.SeriesRef
	agg observer.Aggregate
}

// mcSeriesState holds per-series streaming MC detector state.
//
// POC note: the ring buffer + scratch reuse below is a proof-of-concept
// hot-path optimisation. It's behaviour-preserving (verified bit-identical
// in eval) but the algorithm-level work (e.g. streaming Welford for
// baseline moments, online quantile sketch for p1/p99) is deferred. Once
// those land, this struct's `baselineSorted` / `devSorted` scratch can
// shrink and the per-point sort cost goes to ~O(log N).
type mcSeriesState struct {
	// Ring buffer of recent point values. cap(values) = BaselinePoints +
	// AnomalyPoints; head is the index of the oldest valid entry; size
	// is the number of valid entries (grows during warmup, then equals cap).
	values []float64
	head   int
	size   int

	// Reusable scratch slices to avoid per-Detect allocation. Sized once
	// at first use; reused across all subsequent processPoint calls.
	baselineSorted []float64 // sorted baseline values, size BaselinePoints
	devSorted      []float64 // sorted |v - median|, size BaselinePoints
	anomalyValues  []float64 // anomaly window values (unsorted), size AnomalyPoints
	anomalySorted  []float64 // sorted anomaly values, size AnomalyPoints

	// Cursor: how many points we've processed so far.
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64

	// Alert lifecycle. inAlert covers the in-progress incident; lastAlertTime
	// + MinAlertGapSec covers the post-recovery cooldown.
	inAlert       bool
	recoveryCount int
	lastAlertTime int64
}

// MCSpikeLevelShiftDetector adapts the production Metric Correlations
// per-series detection algorithm (spike + level-shift) to streaming
// per-host execution.
type MCSpikeLevelShiftDetector struct {
	config MCSpikeLevelShiftConfig

	series map[mcStateKey]*mcSeriesState

	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewMCSpikeLevelShiftDetector constructs the detector with sensible
// defaults filled in for any zero-valued fields.
func NewMCSpikeLevelShiftDetector(config MCSpikeLevelShiftConfig) *MCSpikeLevelShiftDetector {
	defaults := DefaultMCSpikeLevelShiftConfig()
	if config.BaselinePoints <= 0 {
		config.BaselinePoints = defaults.BaselinePoints
	}
	if config.AnomalyPoints <= 0 {
		config.AnomalyPoints = defaults.AnomalyPoints
	}
	if config.StdMultiplier <= 0 {
		config.StdMultiplier = defaults.StdMultiplier
	}
	if config.KurtosisMultiplier <= 0 {
		config.KurtosisMultiplier = defaults.KurtosisMultiplier
	}
	if config.LowerPercentileCoef <= 0 {
		config.LowerPercentileCoef = defaults.LowerPercentileCoef
	}
	if config.UpperPercentileCoef <= 0 {
		config.UpperPercentileCoef = defaults.UpperPercentileCoef
	}
	if config.LevelShiftMode == "" {
		config.LevelShiftMode = defaults.LevelShiftMode
	}
	if config.MADMultiplier <= 0 {
		config.MADMultiplier = defaults.MADMultiplier
	}
	if config.MinAbsDeviation < 0 {
		config.MinAbsDeviation = defaults.MinAbsDeviation
	}
	if config.RecoveryPoints <= 0 {
		config.RecoveryPoints = defaults.RecoveryPoints
	}
	if len(config.Aggregations) == 0 {
		config.Aggregations = defaults.Aggregations
	}
	return &MCSpikeLevelShiftDetector{
		config: config,
		series: make(map[mcStateKey]*mcSeriesState),
	}
}

// Name returns the detector name.
func (d *MCSpikeLevelShiftDetector) Name() string {
	return "mc_spike_levelshift_detector"
}

// newSeriesState allocates per-(series, agg) state with all scratch slices
// pre-sized so processPoint never allocates on the steady-state hot path.
func (d *MCSpikeLevelShiftDetector) newSeriesState(bufCap int) *mcSeriesState {
	return &mcSeriesState{
		values:         make([]float64, bufCap),
		baselineSorted: make([]float64, d.config.BaselinePoints),
		devSorted:      make([]float64, d.config.BaselinePoints),
		anomalyValues:  make([]float64, d.config.AnomalyPoints),
		anomalySorted:  make([]float64, d.config.AnomalyPoints),
	}
}

// Detect implements observer.Detector. It walks all workload series, reads
// only newly visible points, and tests for spike + level-shift anomalies
// against a rolling baseline.
func (d *MCSpikeLevelShiftDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
	}

	bufCap := d.config.BaselinePoints + d.config.AnomalyPoints

	var allAnomalies []observer.Anomaly

	for _, meta := range d.cachedSeries {
		for _, agg := range d.config.Aggregations {
			sk := mcStateKey{ref: meta.Ref, agg: agg}
			state, exists := d.series[sk]
			if !exists {
				state = d.newSeriesState(bufCap)
				d.series[sk] = state
			}

			visibleCount := storage.PointCountUpTo(meta.Ref, dataTime)
			currentGen := storage.WriteGeneration(meta.Ref)
			mergeOccurred := visibleCount == state.lastProcessedCount && currentGen != state.lastWriteGen
			if visibleCount <= state.lastProcessedCount && !mergeOccurred {
				continue
			}

			startTime := state.lastProcessedTime
			if mergeOccurred {
				startTime = state.lastProcessedTime - 1
				if startTime < 0 {
					startTime = 0
				}
			}

			pointsSeen := false
			prevLen := len(allAnomalies)
			storage.ForEachPoint(meta.Ref, startTime, dataTime, agg, func(series *observer.Series, p observer.Point) {
				pointsSeen = true
				anomaly := d.processPoint(state, p, series, agg, bufCap)
				if anomaly != nil {
					allAnomalies = append(allAnomalies, *anomaly)
				}
				state.lastProcessedTime = p.Timestamp
			})
			for k := prevLen; k < len(allAnomalies); k++ {
				allAnomalies[k].SourceRef = &observer.QueryHandle{Ref: meta.Ref, Aggregate: agg}
			}

			if !pointsSeen && mergeOccurred {
				state.lastWriteGen = currentGen
				continue
			}
			if pointsSeen {
				state.lastProcessedCount = visibleCount
				state.lastWriteGen = currentGen
			}
		}
	}

	return observer.DetectionResult{Anomalies: allAnomalies}
}

// processPoint pushes a point into the ring buffer and tests for anomalies
// once both windows have filled.
func (d *MCSpikeLevelShiftDetector) processPoint(state *mcSeriesState, p observer.Point, series *observer.Series, agg observer.Aggregate, bufCap int) *observer.Anomaly {
	// Ring-buffer push: O(1) regardless of buffer size.
	if state.size < bufCap {
		state.values[state.size] = p.Value
		state.size++
	} else {
		state.values[state.head] = p.Value
		state.head = (state.head + 1) % bufCap
	}

	// Need a full baseline + anomaly window before evaluating.
	if state.size < bufCap {
		return nil
	}

	// Materialise baseline + anomaly windows into reusable scratch slices.
	// state.head points to the oldest entry, which is also the start of
	// the baseline window.
	bp := d.config.BaselinePoints
	ap := d.config.AnomalyPoints
	for i := 0; i < bp; i++ {
		state.baselineSorted[i] = state.values[(state.head+i)%bufCap]
	}
	for i := 0; i < ap; i++ {
		v := state.values[(state.head+bp+i)%bufCap]
		state.anomalyValues[i] = v
		state.anomalySorted[i] = v
	}

	mean, std, baselineKurt := momentMoments(state.baselineSorted)
	if math.IsNaN(std) || math.IsInf(std, 0) {
		std = 0
	}
	sort.Float64s(state.baselineSorted)
	p1 := nearestRank(state.baselineSorted, 0.01)
	p99 := nearestRank(state.baselineSorted, 0.99)
	baselineMed := midpoint(state.baselineSorted)
	for i, v := range state.baselineSorted {
		state.devSorted[i] = math.Abs(v - baselineMed)
	}
	sort.Float64s(state.devSorted)
	baselineMAD := midpoint(state.devSorted)

	current := p.Value
	floor := d.config.MinAbsDeviation * math.Abs(mean)

	// Hysteresis gate: in-alert blocks while an incident is in progress;
	// MinAlertGapSec blocks for a cooldown after recovery. lastAlertTime=0
	// makes the gate a no-op before the first alert.
	canFire := !state.inAlert &&
		(state.lastAlertTime == 0 || p.Timestamp-state.lastAlertTime >= d.config.MinAlertGapSec)

	// --- Spike test ---
	if std > 0 {
		spikeMagnitude := math.Abs(current - mean)
		if spikeMagnitude > d.config.StdMultiplier*std &&
			(current < p1 || current > p99) &&
			spikeMagnitude >= floor &&
			canFire {
			state.fireAlert(p.Timestamp)
			return d.makeAnomaly(p, series, agg, "spike", current, mean, std, p1, p99, spikeMagnitude/std)
		}
	}

	// --- Kurtosis spike test (heavy-tail anomalies the std test misses) ---
	// MC spec: "rolling kurtosis at the anomaly time > N× the max in the
	// preceding window". We use anomaly-window kurtosis vs baseline kurtosis.
	_, _, anomalyKurt := momentMoments(state.anomalyValues)
	const kurtAbsFloor = 6.0 // sample-kurtosis-noise guard near the Gaussian reference of 3.0
	if baselineKurt > 0 && !math.IsNaN(anomalyKurt) && !math.IsInf(anomalyKurt, 0) &&
		anomalyKurt > d.config.KurtosisMultiplier*baselineKurt &&
		anomalyKurt > kurtAbsFloor &&
		canFire {
		// Require at least one anomaly-window point outside baseline [p1, p99]
		// to filter pure numerical-noise kurtosis spikes.
		hasOutlier := false
		for _, v := range state.anomalyValues {
			if v < p1 || v > p99 {
				hasOutlier = true
				break
			}
		}
		if hasOutlier {
			state.fireAlert(p.Timestamp)
			return d.makeAnomaly(p, series, agg, "kurtosis_spike", current, mean, std, p1, p99, anomalyKurt/baselineKurt)
		}
	}

	// --- Level-shift test ---
	sort.Float64s(state.anomalySorted)
	medAnom := midpoint(state.anomalySorted)
	var lowerBound, upperBound float64
	if d.config.LevelShiftMode == "mad" {
		lowerBound = baselineMed - d.config.MADMultiplier*baselineMAD
		upperBound = baselineMed + d.config.MADMultiplier*baselineMAD
	} else {
		lowerBound = d.config.LowerPercentileCoef * p1
		upperBound = d.config.UpperPercentileCoef * p99
	}
	shiftMagnitude := math.Abs(medAnom - mean)
	if (medAnom < lowerBound || medAnom > upperBound) &&
		shiftMagnitude >= floor &&
		canFire {
		state.fireAlert(p.Timestamp)
		levelDev := 0.0
		if std > 0 {
			levelDev = shiftMagnitude / std
		}
		return d.makeAnomaly(p, series, agg, "level_shift", medAnom, mean, std, p1, p99, levelDev)
	}

	// Recovery: count consecutive non-triggering points until we re-arm.
	if state.inAlert {
		state.recoveryCount++
		if state.recoveryCount >= d.config.RecoveryPoints {
			state.inAlert = false
			state.recoveryCount = 0
		}
	}
	return nil
}

// fireAlert sets the in-alert flag and records the alert timestamp.
func (s *mcSeriesState) fireAlert(ts int64) {
	s.inAlert = true
	s.recoveryCount = 0
	s.lastAlertTime = ts
}

// makeAnomaly constructs an observer.Anomaly for an MC detection.
//
// severity is a kind-specific magnitude score (sigma deviation for spike,
// kurtosis ratio for kurtosis_spike, level-shift sigma for level_shift).
// It populates the Anomaly.Score field so downstream correlators / scorers
// can rank by confidence.
func (d *MCSpikeLevelShiftDetector) makeAnomaly(p observer.Point, series *observer.Series, agg observer.Aggregate, kind string, observed, mean, std, p1, p99, severity float64) *observer.Anomaly {
	source := observer.SeriesDescriptor{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	displayName := source.String()
	deviation := 0.0
	if std > 0 {
		deviation = (observed - mean) / std
	}
	score := severity
	return &observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		Source:       source,
		DetectorName: d.Name(),
		Title:        fmt.Sprintf("MC %s detected: %s", kind, displayName),
		Description: fmt.Sprintf("%s %s observed=%.4g baseline_mean=%.4g std=%.4g p1=%.4g p99=%.4g severity=%.2f",
			displayName, kind, observed, mean, std, p1, p99, severity),
		Timestamp: p.Timestamp,
		Score:     &score,
		DebugInfo: &observer.AnomalyDebugInfo{
			BaselineMean:   mean,
			BaselineStddev: std,
			CurrentValue:   observed,
			DeviationSigma: deviation,
		},
	}
}

// Reset clears all per-series state for replay.
func (d *MCSpikeLevelShiftDetector) Reset() {
	d.series = make(map[mcStateKey]*mcSeriesState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops state for refs that storage has freed.
func (d *MCSpikeLevelShiftDetector) RemoveSeries(refs []observer.SeriesRef) {
	if len(refs) == 0 || len(d.series) == 0 {
		return
	}
	for _, ref := range refs {
		for _, agg := range d.config.Aggregations {
			delete(d.series, mcStateKey{ref: ref, agg: agg})
		}
	}
	d.cachedSeries = nil
	d.cachedGen = 0
}

// --- helpers ---

// momentMoments computes mean, sample stddev (Bessel-corrected), and Pearson
// kurtosis (β2 = m4/m2², Gaussian reference = 3) over a window of values in
// two passes. O(N) time, O(1) extra memory. Kurtosis is reported as 0 for
// degenerate inputs (N < 4, zero variance).
func momentMoments(vals []float64) (mean, std, kurt float64) {
	n := len(vals)
	if n == 0 {
		return 0, 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(n)
	if n < 2 {
		return mean, 0, 0
	}
	var m2, m4 float64
	for _, v := range vals {
		d := v - mean
		d2 := d * d
		m2 += d2
		m4 += d2 * d2
	}
	variance := m2 / float64(n-1)
	std = math.Sqrt(variance)
	if n < 4 || variance <= 0 {
		return mean, std, 0
	}
	popVar := m2 / float64(n)
	kurt = (m4 / float64(n)) / (popVar * popVar)
	return mean, std, kurt
}

// midpoint returns the median of an already-sorted slice.
func midpoint(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return 0.5 * (sorted[n/2-1] + sorted[n/2])
}

// nearestRank returns the q-quantile of a sorted slice using the
// nearest-rank rule: index = ceil(q × n) - 1, clamped to [0, n-1].
func nearestRank(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(q*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
