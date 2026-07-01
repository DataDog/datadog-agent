// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitor provides a small materialized health view for metric
// lookback. The initial monitor watches one retained metric and reports whether
// recent high-resolution points stay above a configured minimum baseline.
package monitor

import (
	"math"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	// DefaultEvaluationInterval is the approximate retained-point window size used
	// by the monitor.
	DefaultEvaluationInterval = 15 * time.Second
	// DefaultMinPoints is the minimum number of retained points required before a
	// window can be treated as healthy or breaching. Sparse windows are reported as
	// unknown so egress can remain conservative.
	DefaultMinPoints = 6
)

var (
	tlmMonitorEvaluations = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "monitor_evaluations", []string{"result"}, "Count of metric lookback monitor window evaluations")
	tlmMonitorValue       = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_value", nil, "Last metric lookback monitor window minimum value")
	tlmMonitorBaselineMin = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_baseline_min", nil, "Configured metric lookback monitor baseline minimum")
	tlmMonitorWindowPts   = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_window_points", nil, "Number of points in the last metric lookback monitor evaluation window")
)

// Point is a retained scalar point read by the monitor.
type Point struct {
	Ts    time.Time
	Value float64
}

// PointReader returns retained points for a metric in an inclusive time range.
type PointReader interface {
	PointsBetween(metricName string, from, to time.Time) []Point
}

// PointReaderFunc adapts a function to PointReader.
type PointReaderFunc func(metricName string, from, to time.Time) []Point

// PointsBetween implements PointReader.
func (f PointReaderFunc) PointsBetween(metricName string, from, to time.Time) []Point {
	if f == nil {
		return nil
	}
	return f(metricName, from, to)
}

// State is the monitor's health classification for one evaluation window.
type State int

const (
	// Unknown means the monitor did not have enough valid data to prove the
	// watched metric was healthy.
	Unknown State = iota
	// Healthy means all valid points in the window were at or above BaselineMin.
	Healthy
	// Breach means at least one valid point in the window dropped below BaselineMin.
	Breach
)

// String returns a stable label for telemetry and diagnostics.
func (s State) String() string {
	switch s {
	case Healthy:
		return "healthy"
	case Breach:
		return "breach"
	default:
		return "unknown"
	}
}

// Decision describes one evaluated monitor window.
type Decision struct {
	MetricName  string
	WindowFrom  time.Time
	WindowTo    time.Time
	State       State
	Value       float64 // window minimum when State is Healthy or Breach
	BaselineMin float64
	PointCount  int
}

// DecisionSink receives monitor decisions. Implementations should avoid
// blocking the caller; monitor evaluation runs synchronously on the selected
// metric's append path.
type DecisionSink interface {
	OnDecision(Decision)
}

// DecisionSinkFunc adapts a function to DecisionSink.
type DecisionSinkFunc func(Decision)

// OnDecision implements DecisionSink.
func (f DecisionSinkFunc) OnDecision(decision Decision) {
	if f != nil {
		f(decision)
	}
}

// Config controls a Watcher. Zero values select conservative defaults so the
// public config surface only needs an enabled flag, metric name, and baseline
// minimum initially.
type Config struct {
	// MetricName is the exact metric name to watch.
	MetricName string
	// BaselineMin is the configured minimum healthy value. A window breaches when
	// its minimum valid value is strictly less than BaselineMin.
	BaselineMin float64
	// EvaluationInterval is the approximate high-resolution window size. Defaults
	// to DefaultEvaluationInterval.
	EvaluationInterval time.Duration
	// MinPoints is the minimum number of valid points required in a window.
	// Defaults to DefaultMinPoints.
	MinPoints int
}

// Watcher periodically evaluates a selected metric from a retained point store.
// It reports a decision for each completed evaluation window. It is safe for
// concurrent use.
type Watcher struct {
	metricName         string
	baselineMin        float64
	evaluationInterval time.Duration
	minPoints          int
	reader             PointReader
	sink               DecisionSink

	mu             sync.Mutex
	lastEvaluation time.Time
	decisions      uint64
	breaches       uint64
}

// New creates a Watcher. It returns nil when the configuration is inert or when
// reader/sink is nil, so callers can treat a nil Watcher as disabled.
func New(cfg Config, reader PointReader, sink DecisionSink) *Watcher {
	if cfg.MetricName == "" || reader == nil || sink == nil {
		return nil
	}
	cfg = normalizeConfig(cfg)
	return &Watcher{
		metricName:         cfg.MetricName,
		baselineMin:        cfg.BaselineMin,
		evaluationInterval: cfg.EvaluationInterval,
		minPoints:          cfg.MinPoints,
		reader:             reader,
		sink:               sink,
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.EvaluationInterval <= 0 {
		cfg.EvaluationInterval = DefaultEvaluationInterval
	}
	if cfg.MinPoints <= 0 {
		cfg.MinPoints = DefaultMinPoints
	}
	return cfg
}

// MetricName returns the metric this watcher evaluates.
func (w *Watcher) MetricName() string {
	if w == nil {
		return ""
	}
	return w.metricName
}

// Observe records that the watched metric has a newly admitted point at
// observedAt. When approximately one evaluation interval has elapsed, Observe
// reads the relevant metric points back from the retention store and evaluates
// the completed window. The sample value is intentionally not passed here: the
// monitor is a materialized view over retention, not a side tap with separate
// value storage.
func (w *Watcher) Observe(name string, observedAt time.Time) bool {
	if w == nil || name != w.metricName || observedAt.IsZero() {
		return false
	}

	from, to, shouldEvaluate := w.nextWindow(observedAt)
	if !shouldEvaluate {
		return false
	}
	return w.evaluateWindow(from, to) == Breach
}

func (w *Watcher) nextWindow(observedAt time.Time) (time.Time, time.Time, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.lastEvaluation.IsZero() {
		w.lastEvaluation = observedAt
		return time.Time{}, time.Time{}, false
	}
	if observedAt.Before(w.lastEvaluation) {
		// Ignore out-of-order monitor clocks. The point is still retained and can be
		// included in a later egress range if it falls inside one.
		return time.Time{}, time.Time{}, false
	}
	if observedAt.Sub(w.lastEvaluation) < w.evaluationInterval {
		return time.Time{}, time.Time{}, false
	}

	from := w.lastEvaluation
	to := observedAt
	w.lastEvaluation = observedAt
	return from, to, true
}

func (w *Watcher) evaluateWindow(from, to time.Time) State {
	points := w.reader.PointsBetween(w.metricName, from, to)
	tlmMonitorWindowPts.Set(float64(len(points)))
	tlmMonitorBaselineMin.Set(w.baselineMin)

	value, validCount, ok := windowMin(points)
	if !ok || validCount < w.minPoints {
		decision := Decision{
			MetricName:  w.metricName,
			WindowFrom:  from,
			WindowTo:    to,
			State:       Unknown,
			BaselineMin: w.baselineMin,
			PointCount:  validCount,
		}
		w.recordDecision(decision)
		tlmMonitorEvaluations.Inc("unknown")
		w.sink.OnDecision(decision)
		return Unknown
	}

	state := Healthy
	if value < w.baselineMin {
		state = Breach
	}
	tlmMonitorValue.Set(value)
	decision := Decision{
		MetricName:  w.metricName,
		WindowFrom:  from,
		WindowTo:    to,
		State:       state,
		Value:       value,
		BaselineMin: w.baselineMin,
		PointCount:  validCount,
	}
	w.recordDecision(decision)
	tlmMonitorEvaluations.Inc(state.String())
	w.sink.OnDecision(decision)
	return state
}

func windowMin(points []Point) (float64, int, bool) {
	min := math.Inf(1)
	validCount := 0
	for _, point := range points {
		if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
			continue
		}
		validCount++
		if point.Value < min {
			min = point.Value
		}
	}
	if validCount == 0 {
		return 0, 0, false
	}
	return min, validCount, true
}

func (w *Watcher) recordDecision(decision Decision) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.decisions++
	if decision.State == Breach {
		w.breaches++
	}
}

// Decisions returns how many monitor decisions this watcher has emitted.
func (w *Watcher) Decisions() uint64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.decisions
}

// Breaches returns how many breach decisions this watcher has emitted.
func (w *Watcher) Breaches() uint64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.breaches
}
