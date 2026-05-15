// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// CheckAggregator wraps the consumer of BufferedAggregator's
// getSeriesAndSketches to provide wall-clock window aggregation across
// CheckSampler commits — decoupling check collection cadence (e.g. 1Hz)
// from send cadence (default flush interval).
//
// Phase 1 (this implementation): skeleton with per-(check_id, ContextKey)
// window tracking. Singleton windows (count = 1 at deadline) pass through
// unchanged, preserving commit_timestamp for byte-identical slow-check
// behaviour. Empty windows emit nothing. The roll-up math for windows
// with count > 1 is deferred to Phase 2; in Phase 1 those windows
// currently emit each buffered Serie individually as a placeholder.
//
// CheckAggregator is safe for concurrent use.
type CheckAggregator struct {
	windowDuration     time.Duration
	maxSeriesPerWindow int

	mu      sync.Mutex
	windows map[windowKey]*aggregationWindow
}

// windowKey identifies a per-(check_id, context) window. The
// SingleAcceptingWindowPerContext invariant (spec) is enforced by this
// map's one-key-one-value property.
type windowKey struct {
	checkID    checkid.ID
	contextKey ckey.ContextKey
}

// aggregationWindow buffers CommittedSeries (CheckSampler's per-commit
// *Serie output) for a single (check_id, context) until the deadline
// is reached.
type aggregationWindow struct {
	series       []*metrics.Serie
	openedAt     float64 // seconds-since-epoch, set from first series' point timestamp
	deadline     float64 // openedAt + windowDuration.Seconds()
	droppedCount int     // count of series rejected due to max_series_per_window
}

// newCheckAggregator constructs a CheckAggregator with the given window
// duration and per-window series cap. Values come from config:
//   - check_aggregator.window_duration (default 15s)
//   - check_aggregator.max_series_per_window (default 128)
func newCheckAggregator(windowDuration time.Duration, maxSeriesPerWindow int) *CheckAggregator {
	return &CheckAggregator{
		windowDuration:     windowDuration,
		maxSeriesPerWindow: maxSeriesPerWindow,
		windows:            make(map[windowKey]*aggregationWindow),
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
// `now` is the current wall-clock time used to evaluate window deadlines.
// At flush time, BufferedAggregator passes the flush trigger's timestamp.
//
// TODO(Phase 2): GaugeWithTimestamp / CountWithTimestamp metrics should
// bypass windowing entirely per the spec's TimestampedSeriesNeverWindowed
// invariant. Phase 1 routes all metric types through the same path
// because *Serie does not carry the original internal MetricType after
// ContextMetrics.Flush — it only carries APIMetricType. For slow checks
// the behaviour is byte-identical (singleton passthrough preserves the
// multi-point Series unchanged), so Phase 1 is safe; the invariant is
// only literally violated for fast checks emitting timestamped metrics.
// Phase 2 will either heuristically detect timestamped series (e.g.
// len(Points) > 1) or plumb the original MetricType through.
func (ca *CheckAggregator) Submit(checkID checkid.ID, serie *metrics.Serie, now float64, sink metrics.SerieSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	key := windowKey{checkID: checkID, contextKey: serie.ContextKey}
	window, exists := ca.windows[key]

	// If a window exists and its deadline has passed, close it before
	// processing the new series. This implements the "accepting →
	// emitted" atomic transition per CloseWindowOnDeadline /
	// SingletonWindowPassThrough.
	if exists && window.deadline <= now {
		ca.closeWindowLocked(window, sink)
		delete(ca.windows, key)
		exists = false
	}

	if !exists {
		// OpenWindowForNewContext
		openedAt := seriesTimestamp(serie)
		ca.windows[key] = &aggregationWindow{
			series:   []*metrics.Serie{serie},
			openedAt: openedAt,
			deadline: openedAt + ca.windowDuration.Seconds(),
		}
		return
	}

	// AppendSeriesToOpenWindow / IncrementDropCountWhenWindowFull
	if len(window.series) >= ca.maxSeriesPerWindow {
		window.droppedCount++
		return
	}
	window.series = append(window.series, serie)
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

// Drain forces every accepting window to close, regardless of deadline.
// Intended to be called by BufferedAggregator's stop sequence so no
// buffered series are lost at shutdown.
//
// TODO(Phase 4): wire this into BufferedAggregator.Stop() and add the
// 5s hard timeout. Phase 1 ships Drain as a standalone primitive,
// fully tested, but it is not yet invoked anywhere in the agent's
// shutdown path.
func (ca *CheckAggregator) Drain(sink metrics.SerieSink) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for key, window := range ca.windows {
		ca.closeWindowLocked(window, sink)
		delete(ca.windows, key)
	}
}

// closeWindowLocked emits the window's contents to sink per the spec's
// per-count rules. Caller must hold ca.mu.
func (ca *CheckAggregator) closeWindowLocked(w *aggregationWindow, sink metrics.SerieSink) {
	switch len(w.series) {
	case 0:
		// DropEmptyWindowOnDeadline: never emit a synthetic zero for a
		// context that produced nothing in the window.
		return
	case 1:
		// SingletonWindowPassThrough: emit the single series unchanged.
		// commit_timestamp is already preserved in the Serie's Points,
		// so byte-identicality holds for slow checks.
		sink.Append(w.series[0])
		return
	default:
		// CloseWindowOnDeadline — count > 1, requires per-mtype roll-up.
		// Phase 2 will implement the strategies (last/sum/sketch_merge/
		// pass_through). For Phase 1, emit each series individually as a
		// placeholder so the pipeline keeps working; this is NOT
		// spec-compliant (the spec promises one AggregatedSeries per
		// window per context) but it preserves data while we wait for
		// Phase 2.
		//
		// TODO(Phase 2): replace this with aggregate(window) per spec.
		for _, s := range w.series {
			sink.Append(s)
		}
	}
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
