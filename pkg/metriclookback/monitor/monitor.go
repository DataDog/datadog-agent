// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitor provides a small materialized health view for metric
// lookback. The initial monitor watches one retained metric and reports whether
// recent high-resolution points vary by more than a configured range epsilon.
package monitor

import (
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	// DefaultEvaluationInterval is the approximate retained-point window size used
	// by the monitor.
	DefaultEvaluationInterval = 30 * time.Second
	// DefaultMinPoints is the minimum number of retained points required to compute
	// a range. A single valid point cannot show whether the window moved by more
	// than epsilon.
	DefaultMinPoints = 2
)

var (
	tlmMonitorEvaluations  = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "monitor_evaluations", []string{"result"}, "Count of metric lookback monitor window evaluations")
	tlmMonitorWindowMin    = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_window_min", nil, "Minimum value in the last metric lookback monitor evaluation window")
	tlmMonitorWindowMax    = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_window_max", nil, "Maximum value in the last metric lookback monitor evaluation window")
	tlmMonitorWindowRange  = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_window_range", nil, "Maximum minus minimum value in the last metric lookback monitor evaluation window")
	tlmMonitorRangeEpsilon = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_range_epsilon", nil, "Configured metric lookback monitor range epsilon")
	tlmMonitorWindowPts    = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "monitor_window_points", nil, "Number of points in the last metric lookback monitor evaluation window")
)

// Point is a retained scalar point read by the monitor.
type Point struct {
	Ts    time.Time
	Value float64
	Tags  []string
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
	// Healthy means the valid point range in the window was at or below the
	// configured range epsilon.
	Healthy
	// Breach means the valid point range in the window was greater than the
	// configured range epsilon.
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
	MetricName     string
	WindowFrom     time.Time
	WindowTo       time.Time
	State          State
	Min            float64
	Max            float64
	Range          float64
	RangeEpsilon   float64
	PointCount     int
	PartitionTags  []string
	PartitionKey   string
	PartitionCount int
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
// public config surface can enable the monitor with just a metric name and range
// epsilon, and can optionally partition evaluation by tag keys.
type Config struct {
	// MetricName is the exact metric name to watch.
	MetricName string
	// RangeEpsilon is the maximum healthy max-min range for a completed window. A
	// window breaches when its valid point range is strictly greater than
	// RangeEpsilon.
	RangeEpsilon float64
	// PartitionTags are tag keys used to partition range evaluation. When empty,
	// all retained points for MetricName are evaluated together. When set, each
	// selected tag-value tuple is evaluated independently and any sufficient
	// partition can breach the monitor.
	PartitionTags []string
	// EvaluationInterval is the approximate high-resolution window size. Defaults
	// to DefaultEvaluationInterval.
	EvaluationInterval time.Duration
	// MinPoints is the minimum number of valid points required in a window.
	// Defaults to DefaultMinPoints. Values below 2 are raised to 2 because a
	// single point cannot establish a range.
	MinPoints int
}

// Watcher periodically evaluates a selected metric from a retained point store.
// It reports a decision for each completed evaluation window. It is safe for
// concurrent use.
type Watcher struct {
	metricName         string
	rangeEpsilon       float64
	evaluationInterval time.Duration
	minPoints          int
	partitionTags      []string
	reader             PointReader
	sink               DecisionSink

	// observeMu serializes window evaluation and decision delivery in the same
	// order windows are claimed.
	observeMu sync.Mutex

	mu             sync.Mutex
	lastEvaluation time.Time
	decisions      uint64
	breaches       uint64
}

// New creates a Watcher. It returns nil when the configuration is inert or when
// reader/sink is nil, so callers can treat a nil Watcher as disabled.
func New(cfg Config, reader PointReader, sink DecisionSink) (*Watcher, error) {
	if cfg.MetricName == "" || reader == nil || sink == nil {
		return nil, nil
	}
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Watcher{
		metricName:         cfg.MetricName,
		rangeEpsilon:       cfg.RangeEpsilon,
		partitionTags:      append([]string(nil), cfg.PartitionTags...),
		evaluationInterval: cfg.EvaluationInterval,
		minPoints:          cfg.MinPoints,
		reader:             reader,
		sink:               sink,
	}, nil
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.RangeEpsilon < 0 {
		return Config{}, errors.New("range epsilon must be non-negative")
	}
	cfg.PartitionTags = normalizePartitionTags(cfg.PartitionTags)
	if cfg.EvaluationInterval <= 0 {
		cfg.EvaluationInterval = DefaultEvaluationInterval
	}
	if cfg.MinPoints <= 0 {
		cfg.MinPoints = DefaultMinPoints
	}
	if cfg.MinPoints < 2 {
		cfg.MinPoints = 2
	}
	return cfg, nil
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

	// Serialize window selection, evaluation, and decision delivery so concurrent
	// producers cannot emit monitor decisions out of chronological order.
	w.observeMu.Lock()
	defer w.observeMu.Unlock()

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
	tlmMonitorRangeEpsilon.Set(w.rangeEpsilon)

	partitionStats := windowPartitionStats(points, w.partitionTags)
	selected, sufficientCount, ok := selectMonitorPartition(partitionStats, w.minPoints)
	if !ok {
		decision := Decision{
			MetricName:     w.metricName,
			WindowFrom:     from,
			WindowTo:       to,
			State:          Unknown,
			RangeEpsilon:   w.rangeEpsilon,
			PointCount:     totalValidPointCount(partitionStats),
			PartitionTags:  append([]string(nil), w.partitionTags...),
			PartitionCount: len(partitionStats),
		}
		w.recordDecision(decision)
		tlmMonitorEvaluations.Inc("unknown")
		w.sink.OnDecision(decision)
		return Unknown
	}

	state := Healthy
	if selected.valueRange > w.rangeEpsilon {
		state = Breach
	}
	tlmMonitorWindowMin.Set(selected.min)
	tlmMonitorWindowMax.Set(selected.max)
	tlmMonitorWindowRange.Set(selected.valueRange)
	decision := Decision{
		MetricName:     w.metricName,
		WindowFrom:     from,
		WindowTo:       to,
		State:          state,
		Min:            selected.min,
		Max:            selected.max,
		Range:          selected.valueRange,
		RangeEpsilon:   w.rangeEpsilon,
		PointCount:     selected.validCount,
		PartitionTags:  append([]string(nil), w.partitionTags...),
		PartitionKey:   selected.key,
		PartitionCount: sufficientCount,
	}
	w.recordDecision(decision)
	tlmMonitorEvaluations.Inc(state.String())
	w.sink.OnDecision(decision)
	return state
}

func windowMinMax(points []Point) (float64, float64, int, bool) {
	minValue := math.Inf(1)
	maxValue := math.Inf(-1)
	validCount := 0
	for _, point := range points {
		if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
			continue
		}
		validCount++
		if point.Value < minValue {
			minValue = point.Value
		}
		if point.Value > maxValue {
			maxValue = point.Value
		}
	}
	if validCount == 0 {
		return 0, 0, 0, false
	}
	return minValue, maxValue, validCount, true
}

type partitionWindowStats struct {
	key        string
	min        float64
	max        float64
	valueRange float64
	validCount int
}

func windowPartitionStats(points []Point, partitionTags []string) []partitionWindowStats {
	byKey := make(map[string][]Point)
	for _, point := range points {
		key := monitorPartitionKey(point.Tags, partitionTags)
		byKey[key] = append(byKey[key], point)
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	stats := make([]partitionWindowStats, 0, len(keys))
	for _, key := range keys {
		minValue, maxValue, validCount, ok := windowMinMax(byKey[key])
		if !ok {
			stats = append(stats, partitionWindowStats{key: key})
			continue
		}
		stats = append(stats, partitionWindowStats{
			key:        key,
			min:        minValue,
			max:        maxValue,
			valueRange: maxValue - minValue,
			validCount: validCount,
		})
	}
	return stats
}

func selectMonitorPartition(stats []partitionWindowStats, minPoints int) (partitionWindowStats, int, bool) {
	var selected partitionWindowStats
	sufficientCount := 0
	for _, stat := range stats {
		if stat.validCount < minPoints {
			continue
		}
		sufficientCount++
		if sufficientCount == 1 || stat.valueRange > selected.valueRange || (stat.valueRange == selected.valueRange && stat.key < selected.key) {
			selected = stat
		}
	}
	return selected, sufficientCount, sufficientCount > 0
}

func totalValidPointCount(stats []partitionWindowStats) int {
	total := 0
	for _, stat := range stats {
		total += stat.validCount
	}
	return total
}

func monitorPartitionKey(tags []string, partitionTags []string) string {
	if len(partitionTags) == 0 {
		return ""
	}
	valuesByName := make(map[string]string, len(tags))
	for _, tag := range tags {
		name, value, ok := strings.Cut(tag, ":")
		if !ok || name == "" {
			continue
		}
		if _, exists := valuesByName[name]; !exists {
			valuesByName[name] = value
		}
	}

	parts := make([]string, 0, len(partitionTags))
	for _, name := range partitionTags {
		value, ok := valuesByName[name]
		if !ok {
			value = "<missing>"
		}
		parts = append(parts, name+":"+value)
	}
	return strings.Join(parts, ",")
}

func normalizePartitionTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(strings.TrimSuffix(tag, ":"))
		if name == "" {
			continue
		}
		if _, found := seen[name]; found {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
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
