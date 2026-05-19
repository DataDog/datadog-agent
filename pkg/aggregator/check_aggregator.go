// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// CheckAggregator wraps the consumer of BufferedAggregator's
// getSeriesAndSketches to provide wall-clock window aggregation across
// CheckSampler commits — decoupling check collection cadence (e.g. 1Hz)
// from send cadence (default flush interval).
//
// Windows are keyed by check ID, context key, and final metric identity.
// Singleton windows pass through unchanged; multi-sample windows roll up
// Series by API type (gauge latest, count sum, rate average) and merge
// SketchSeries into one sketch for the flush window.
//
// Known approximations are intentionally scoped here rather than plumbing new
// metric-type metadata through the pipeline: RateType reaches this layer as
// APIGaugeType, histogram .avg/percentile scalar series keep the latest value,
// and timestamped Series are windowed when emitted at fast cadence.
//
// CheckAggregator is safe for concurrent use.
type CheckAggregator struct {
	windowDuration     time.Duration
	maxSeriesPerWindow int

	mu            sync.Mutex
	windows       map[windowKey]*aggregationWindow
	sketchWindows map[windowKey]*sketchAggregationWindow
}

// windowKey identifies a per-(check_id, context, final metric identity)
// window. Name and API type are part of the key because one context key
// can produce multiple backend series, for example histogram suffixes
// such as .avg and .count. Keeping those identities separate prevents
// aggregation from collapsing distinct backend semantics.
type windowKey struct {
	checkID    checkid.ID
	contextKey ckey.ContextKey
	name       string
	mType      metrics.APIMetricType
}

// aggregationWindow maintains rolling Series state for a single
// (check_id, context, metric identity) until the deadline is reached.
type aggregationWindow struct {
	count        int
	singleton    *metrics.Serie
	latestSeries *metrics.Serie
	latestPoint  metrics.Point
	total        float64
	pointCount   int
	min          float64
	max          float64
	openedAt     float64
	deadline     float64
	droppedCount int
}

// sketchAggregationWindow maintains a rolling merged SketchSeries for a
// single (check_id, context, metric identity) until the deadline is reached.
type sketchAggregationWindow struct {
	count        int
	singleton    *metrics.SketchSeries
	metadata     *metrics.SketchSeries
	merged       *quantile.Sketch
	latestTs     int64
	openedAt     float64
	deadline     float64
	droppedCount int
}

// newCheckAggregator constructs a CheckAggregator with the effective window
// duration and per-window series cap. By default, the window duration follows
// the demultiplexer flush interval; check_aggregator.window_duration can
// override it.
//
// The same cap applies to sketch windows (per (check_id, context)) —
// sketches are typically much smaller in count per window than series
// (one merged sketch is roughly the size of one input sketch), so a
// single shared cap is sufficient.
func newCheckAggregator(windowDuration time.Duration, maxSeriesPerWindow int) *CheckAggregator {
	return &CheckAggregator{
		windowDuration:     windowDuration,
		maxSeriesPerWindow: maxSeriesPerWindow,
		windows:            make(map[windowKey]*aggregationWindow),
		sketchWindows:      make(map[windowKey]*sketchAggregationWindow),
	}
}

func seriesWindowKey(checkID checkid.ID, serie *metrics.Serie) windowKey {
	return windowKey{
		checkID:    checkID,
		contextKey: serie.ContextKey,
		name:       serie.Name,
		mType:      serie.MType,
	}
}

func sketchWindowKey(checkID checkid.ID, sketches *metrics.SketchSeries) windowKey {
	return windowKey{
		checkID:    checkID,
		contextKey: sketches.ContextKey,
		name:       sketches.Name,
	}
}

// Submit routes a *Serie through the windowing layer.
//
//   - If the series' context has no open window, a new window is opened.
//   - If the series' context has an open window whose deadline has not
//     yet passed, the series is appended.
//   - If the open window is full (count >= max_series_per_window), the
//     series is discarded and a drop counter is incremented.
//   - If the open window's deadline has passed before this submission,
//     the existing window is closed (emitted to sink) and a new window
//     is opened with this series as its first member.
//
// Window rollover on submit is evaluated against the incoming series timestamp
// so a batched flush of historical 1Hz points does not split the window at the
// flush trigger time.
//
// Timestamped metrics (GaugeWithTimestamp / CountWithTimestamp) are NOT
// bypassed: per the cost constraint, every series routed through the
// check path must be aggregated to ≤1 emission per (context, window).
// For slow checks (1 commit per window) singleton-passthrough preserves
// multi-point Series unchanged; for fast checks the aggregation
// strategies apply across the multi-point payload (lossy for individual
// timestamps, but fidelity is recoverable via the observer/recorder).
func (ca *CheckAggregator) Submit(checkID checkid.ID, serie *metrics.Serie, sink metrics.SerieSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	key := seriesWindowKey(checkID, serie)
	window, exists := ca.windows[key]
	incomingTs := seriesTimestamp(serie)

	// If a window exists and its deadline has passed, close it before
	// processing the new series. This implements the "accepting →
	// emitted" atomic transition per CloseWindowOnDeadline /
	// SingletonWindowPassThrough.
	if exists && window.deadline <= incomingTs {
		ca.closeWindowLocked(window, sink)
		delete(ca.windows, key)
		exists = false
	}

	if !exists {
		// OpenWindowForNewContext
		ca.windows[key] = newAggregationWindow(serie, incomingTs, ca.windowDuration.Seconds())
		return
	}

	// AppendSeriesToOpenWindow / IncrementDropCountWhenWindowFull
	if window.count >= ca.maxSeriesPerWindow {
		window.droppedCount++
		return
	}
	window.add(serie)
}

func newAggregationWindow(serie *metrics.Serie, incomingTs float64, durationSeconds float64) *aggregationWindow {
	w := &aggregationWindow{
		count:     1,
		singleton: serie,
		openedAt:  incomingTs,
		deadline:  incomingTs + durationSeconds,
	}
	w.addPoints(serie)
	return w
}

func (w *aggregationWindow) add(serie *metrics.Serie) {
	w.count++
	if w.count > 1 {
		w.singleton = nil
	}
	w.addPoints(serie)
}

func (w *aggregationWindow) addPoints(serie *metrics.Serie) {
	for _, p := range serie.Points {
		if w.pointCount == 0 {
			w.min = p.Value
			w.max = p.Value
			w.latestSeries = serie
			w.latestPoint = p
		}
		if p.Value < w.min {
			w.min = p.Value
		}
		if p.Value > w.max {
			w.max = p.Value
		}
		if p.Ts >= w.latestPoint.Ts {
			w.latestSeries = serie
			w.latestPoint = p
		}
		w.total += p.Value
		w.pointCount++
	}
}

func (w *aggregationWindow) last() *metrics.Serie {
	return cloneSerieWithSinglePoint(w.latestSeries, w.latestPoint.Ts, w.latestPoint.Value)
}

func (w *aggregationWindow) sum() *metrics.Serie {
	return cloneSerieWithSinglePoint(w.latestSeries, w.latestPoint.Ts, w.total)
}

func (w *aggregationWindow) avg() *metrics.Serie {
	return cloneSerieWithSinglePoint(w.latestSeries, w.latestPoint.Ts, w.total/float64(w.pointCount))
}

func (w *aggregationWindow) minSerie() *metrics.Serie {
	return cloneSerieWithSinglePoint(w.latestSeries, w.latestPoint.Ts, w.min)
}

func (w *aggregationWindow) maxSerie() *metrics.Serie {
	return cloneSerieWithSinglePoint(w.latestSeries, w.latestPoint.Ts, w.max)
}

func (w *aggregationWindow) gaugeLike() *metrics.Serie {
	name := w.latestSeries.Name
	switch {
	case strings.HasSuffix(name, ".max"):
		return w.maxSerie()
	case strings.HasSuffix(name, ".min"):
		return w.minSerie()
	case strings.HasSuffix(name, ".sum"):
		return w.sum()
	default:
		return w.last()
	}
}

func newSketchAggregationWindow(sketches *metrics.SketchSeries, incomingTs float64, durationSeconds float64) *sketchAggregationWindow {
	w := &sketchAggregationWindow{
		count:     1,
		singleton: sketches,
		metadata:  sketches,
		openedAt:  incomingTs,
		deadline:  incomingTs + durationSeconds,
	}
	w.addSketches(sketches)
	return w
}

func (w *sketchAggregationWindow) add(sketches *metrics.SketchSeries) {
	w.count++
	if w.count > 1 {
		w.singleton = nil
	}
	w.addSketches(sketches)
}

func (w *sketchAggregationWindow) addSketches(sketches *metrics.SketchSeries) {
	w.metadata = sketches
	for _, sp := range sketches.Points {
		s, ok := sp.Sketch.(*quantile.Sketch)
		if !ok || s == nil {
			continue
		}
		if w.merged == nil {
			w.merged = s.Copy()
		} else {
			w.merged.Merge(quantile.Default(), s)
		}
		if sp.Ts > w.latestTs {
			w.latestTs = sp.Ts
		}
	}
}

func (w *sketchAggregationWindow) mergedSeries() *metrics.SketchSeries {
	if w.merged == nil {
		return nil
	}
	out := *w.metadata
	out.Points = []metrics.SketchPoint{{Sketch: w.merged, Ts: w.latestTs}}
	return &out
}

// FlushExpired closes every window whose deadline has passed at `now`,
// emitting to sink. Called by BufferedAggregator at the end of each
// getSeriesAndSketches cycle, after all CheckSamplers have submitted
// their flush output. Windows that have not yet reached their deadline
// remain open and accumulate further series in subsequent flushes.
func (ca *CheckAggregator) FlushExpired(now float64, sink metrics.SerieSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key, window := range ca.windows {
		if window.deadline <= now {
			ca.closeWindowLocked(window, sink)
			delete(ca.windows, key)
		}
	}
}

// SubmitSketch routes a *SketchSeries through the sketch-windowing
// pipeline. Behaviour mirrors Submit for Series:
//   - First SketchSeries for a context opens a sketch window.
//   - Subsequent SketchSeries for the same context append before deadline.
//   - On deadline-reached at submit time, the existing window closes
//     (emitted to sink) before opening a new one for this submission.
//   - When the window is full, the SketchSeries is dropped and counted.
//
// On close, all buffered sketches are merged via *quantile.Sketch.Merge.
// This preserves the combined DDSketch representation while emitting a
// single SketchSeries for the flush window.
func (ca *CheckAggregator) SubmitSketch(checkID checkid.ID, sketches *metrics.SketchSeries, sink metrics.SketchesSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	key := sketchWindowKey(checkID, sketches)
	window, exists := ca.sketchWindows[key]
	incomingTs := sketchSeriesTimestamp(sketches)

	if exists && window.deadline <= incomingTs {
		ca.closeSketchWindowLocked(window, sink)
		delete(ca.sketchWindows, key)
		exists = false
	}

	if !exists {
		ca.sketchWindows[key] = newSketchAggregationWindow(sketches, incomingTs, ca.windowDuration.Seconds())
		return
	}

	if window.count >= ca.maxSeriesPerWindow {
		window.droppedCount++
		return
	}
	window.add(sketches)
}

// FlushExpiredSketches closes every sketch window whose deadline has
// passed at `now`, emitting the merged sketch to sink. Counterpart to
// FlushExpired for the sketches path.
func (ca *CheckAggregator) FlushExpiredSketches(now float64, sink metrics.SketchesSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key, window := range ca.sketchWindows {
		if window.deadline <= now {
			ca.closeSketchWindowLocked(window, sink)
			delete(ca.sketchWindows, key)
		}
	}
}

// Drain forces every accepting window to close, regardless of deadline.
// Intended to be called by BufferedAggregator's stop sequence so no
// buffered series are lost at shutdown. Drains both Series and Sketches
// windows — the caller passes both sinks.
//
// BufferedAggregator invokes Drain during AgentDemultiplexer.Stop(true)'s
// final flush so a partial last window is not lost on shutdown.
func (ca *CheckAggregator) Drain(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key, window := range ca.windows {
		ca.closeWindowLocked(window, seriesSink)
		delete(ca.windows, key)
	}
	for key, window := range ca.sketchWindows {
		ca.closeSketchWindowLocked(window, sketchesSink)
		delete(ca.sketchWindows, key)
	}
}

func (ca *CheckAggregator) HasCheckWindows(checkID checkid.ID) bool {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key := range ca.windows {
		if key.checkID == checkID {
			return true
		}
	}
	for key := range ca.sketchWindows {
		if key.checkID == checkID {
			return true
		}
	}
	return false
}

// DrainCheck closes all open windows for one check ID. It is used when a
// check moves to a non-windowed cadence so already buffered fast-cadence data
// is emitted before new samples bypass the windowing layer.
func (ca *CheckAggregator) DrainCheck(checkID checkid.ID, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key, window := range ca.windows {
		if key.checkID != checkID {
			continue
		}
		ca.closeWindowLocked(window, seriesSink)
		delete(ca.windows, key)
	}
	for key, window := range ca.sketchWindows {
		if key.checkID != checkID {
			continue
		}
		ca.closeSketchWindowLocked(window, sketchesSink)
		delete(ca.sketchWindows, key)
	}
}

// closeWindowLocked emits the window's contents to sink per the spec's
// per-count rules. Caller must hold ca.mu.
func (ca *CheckAggregator) closeWindowLocked(w *aggregationWindow, sink metrics.SerieSink) {
	switch w.count {
	case 0:
		// DropEmptyWindowOnDeadline: never emit a synthetic zero for a
		// context that produced nothing in the window.
		return
	case 1:
		// SingletonWindowPassThrough: emit the single series unchanged.
		// commit_timestamp is already preserved in the Serie's Points,
		// so byte-identicality holds for slow checks.
		sink.Append(w.singleton)
		return
	default:
		// CloseWindowOnDeadline: count > 1, apply per-mtype roll-up.
		// All series in a window share the same context (windowKey
		// includes ContextKey), so MType is consistent across the window.
		switch w.latestSeries.MType {
		case metrics.APIGaugeType:
			// Gauge-like series use `last` unless the final metric name
			// carries a known histogram aggregate suffix. The suffix
			// strategies improve histogram scalar semantics without
			// plumbing original MetricType through the metrics pipeline.
			sink.Append(w.gaugeLike())
		case metrics.APICountType:
			// `sum` strategy: covers both Count (sum-since-commit) and
			// MonotonicCount (per-commit delta) since CheckSampler maps
			// both to APICountType. Sum-of-sums and sum-of-deltas are
			// the correct window-total in each case.
			sink.Append(w.sum())
		case metrics.APIRateType:
			// `avg` strategy: average across per-commit rates. Correct
			// for CheckSampler-emitted Rate-via-APIRateType (e.g.
			// histogram .count suffix) because CheckSampler emits at
			// uniform commit intervals — averaging uniform-interval
			// per-commit rates equals the true window rate. The "avg of
			// rates ≠ true rate" warning applies only to non-uniform
			// commit cadences, which CheckSampler does not produce.
			sink.Append(w.avg())
		default:
			// Unknown APIMetricType (future additions): conservative
			// `last` to satisfy the cost constraint while remaining
			// approximate. Cross-reference with the spec's MetricType
			// closed-set (Q5) — flagged for review if hit.
			sink.Append(w.last())
		}
	}
}

// cloneSerieWithSinglePoint returns a shallow copy of `src` with its
// Points replaced by a single Point at (ts, value). Used by the
// aggregation strategies to produce one rolled-up Serie per window
// without mutating the buffered series (which the caller may still
// reference).
//
// Reference-shared fields (Tags via tagset.CompositeTags, Resources
// slice, ContextKey): the clone shares pointers/handles with src. Safe
// today because the buffered Series are dropped at window close and
// not referenced again. If a future change retains buffered Series
// past close (e.g. telemetry, debug dumps), this aliasing must be
// reconsidered — mutation through one Serie would affect the other.
func cloneSerieWithSinglePoint(src *metrics.Serie, ts float64, value float64) *metrics.Serie {
	out := *src
	out.Points = []metrics.Point{{Ts: ts, Value: value}}
	return &out
}

// seriesTimestamp returns the first point's timestamp from a Serie, used
// as the window's opened_at. A Serie from CheckSampler.commitSeries
// always has at least one point; defensively, return 0 if empty.
func seriesTimestamp(s *metrics.Serie) float64 {
	if len(s.Points) == 0 {
		return 0
	}
	return s.Points[0].Ts
}

// closeSketchWindowLocked emits the merged sketch (or the singleton
// untouched) to sink per the same per-count rules as closeWindowLocked.
// Caller must hold ca.mu.
func (ca *CheckAggregator) closeSketchWindowLocked(w *sketchAggregationWindow, sink metrics.SketchesSink) {
	switch w.count {
	case 0:
		// DropEmptyWindowOnDeadline.
		return
	case 1:
		// SingletonWindowPassThrough: emit the single SketchSeries
		// unchanged. Preserves all original SketchPoints and timestamps.
		sink.Append(w.singleton)
		return
	default:
		// Merge all buffered sketches into one. The output is a single
		// SketchSeries carrying one SketchPoint with the merged sketch,
		// stamped at the latest commit's timestamp.
		merged := w.mergedSeries()
		if merged != nil {
			sink.Append(merged)
		}
	}
}

// sketchSeriesTimestamp returns the first SketchPoint's timestamp from a
// SketchSeries, used as the window's opened_at. A SketchSeries from
// CheckSampler.commitSketches has at least one SketchPoint; defensively
// return 0 if empty.
func sketchSeriesTimestamp(ss *metrics.SketchSeries) float64 {
	if len(ss.Points) == 0 {
		return 0
	}
	return float64(ss.Points[0].Ts)
}
