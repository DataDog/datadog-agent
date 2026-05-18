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
// Behaviour:
//   - For each (check_id, ContextKey, final metric identity), a window
//     is opened lazily on the first arriving Serie. The window's
//     deadline is opened_at + window_duration (window_duration from
//     config; default 15s).
//   - Singleton windows (count = 1 at deadline) pass through unchanged,
//     preserving commit_timestamp. Byte-identical for slow checks.
//   - Multi-series windows (count > 1) apply per-API-type roll-up to
//     emit one aggregated Serie per (check_id, context, metric
//     identity, window). The cost constraint (1 emission per window) is
//     non-negotiable: at 1Hz collection the backend cannot sustain 15
//     series per 15s per context. Strategies:
//     APIGaugeType  → emit the latest value (`last`)
//     APICountType  → emit the sum across the window (`sum`)
//     APIRateType   → emit the average across the window (`avg`)
//     (others)      → conservative `last`
//   - Empty windows emit nothing.
//
// Sketches (Distribution / Histogram quantile sketches) are routed
// through a parallel sketch-windowing path. On window close, buffered
// sketches are merged via *quantile.Sketch.Merge. For sketches already
// represented as DDSketches, merge preserves the combined sketch
// representation while reducing N flush-window inputs to one output.
//
// Accepted approximations:
//   - A check that emits Rate metrics is collapsed by CheckSampler to
//     APIGaugeType at the Serie level (see pkg/metrics/rate.go). The
//     `last` strategy applies to such Series in fast-cadence windows —
//     the backend sees the most-recent rate value rather than the
//     average rate over the window. Cost goal met; math is an
//     approximation bounded by workload volatility within the window.
//     Proper fix requires plumbing the internal MetricType through
//     pkg/metrics; deferred until customer impact warrants it.
//   - Histogram suffix Series are handled with a local name-suffix
//     heuristic to avoid plumbing the original MetricType through the
//     metrics pipeline: .max uses max-of-maxes, .min uses min-of-mins,
//     .sum sums window sums, and .avg / percentiles retain the most
//     recent value. This preserves common scalar histogram aggregates
//     while keeping the change scoped; .avg and percentiles remain
//     bounded approximations at sub-flush cadence.
//   - GaugeWithTimestamp / CountWithTimestamp Series in fast-cadence
//     windows are aggregated per their APIMetricType (last / sum). Per-
//     timestamp fidelity at the backend is lost — recoverable via the
//     observer/recorder which sees raw samples pre-aggregation.
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

// aggregationWindow buffers CommittedSeries (CheckSampler's per-commit
// *Serie output) for a single (check_id, context) until the deadline
// is reached.
type aggregationWindow struct {
	series       []*metrics.Serie
	openedAt     float64 // seconds-since-epoch, set from first series' point timestamp
	deadline     float64 // openedAt + windowDuration.Seconds()
	droppedCount int     // count of series rejected due to max_series_per_window
}

// sketchAggregationWindow buffers per-commit SketchSeries for a single
// (check_id, context) until the deadline is reached. On close, the
// underlying quantile sketches are merged via *quantile.Sketch.Merge.
type sketchAggregationWindow struct {
	sketches     []*metrics.SketchSeries
	openedAt     float64
	deadline     float64
	droppedCount int
}

// newCheckAggregator constructs a CheckAggregator with the given window
// duration and per-window series cap. Values come from config:
//   - check_aggregator.window_duration (default 15s)
//   - check_aggregator.max_series_per_window (default 128)
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
// `now` is retained in the signature for parity with FlushExpired and the
// sketch path. Window rollover on submit is evaluated against the incoming
// series timestamp so a batched flush of historical 1Hz points does not split
// the window at the flush trigger time.
//
// Timestamped metrics (GaugeWithTimestamp / CountWithTimestamp) are NOT
// bypassed: per the cost constraint, every series routed through the
// check path must be aggregated to ≤1 emission per (context, window).
// For slow checks (1 commit per window) singleton-passthrough preserves
// multi-point Series unchanged; for fast checks the aggregation
// strategies apply across the multi-point payload (lossy for individual
// timestamps, but fidelity is recoverable via the observer/recorder).
func (ca *CheckAggregator) Submit(checkID checkid.ID, serie *metrics.Serie, _ float64, sink metrics.SerieSink) {
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
		ca.windows[key] = &aggregationWindow{
			series:   []*metrics.Serie{serie},
			openedAt: incomingTs,
			deadline: incomingTs + ca.windowDuration.Seconds(),
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

// SubmitSketch routes a *SketchSeries through the sketch-windowing
// pipeline. Behaviour mirrors Submit for Series:
//   - First SketchSeries for a context opens a sketch window.
//   - Subsequent SketchSeries for the same context append before deadline.
//   - On deadline-reached at submit time, the existing window closes
//     (emitted to sink) before opening a new one for this submission.
//   - When the window is full, the SketchSeries is dropped and counted.
//
// `now` is retained in the signature for parity with FlushExpiredSketches;
// submit-time rollover uses the incoming SketchSeries timestamp.
//
// On close, all buffered sketches are merged via *quantile.Sketch.Merge.
// This preserves the combined DDSketch representation while emitting a
// single SketchSeries for the flush window.
func (ca *CheckAggregator) SubmitSketch(checkID checkid.ID, sketches *metrics.SketchSeries, _ float64, sink metrics.SketchesSink) {
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
		ca.sketchWindows[key] = &sketchAggregationWindow{
			sketches: []*metrics.SketchSeries{sketches},
			openedAt: incomingTs,
			deadline: incomingTs + ca.windowDuration.Seconds(),
		}
		return
	}

	if len(window.sketches) >= ca.maxSeriesPerWindow {
		window.droppedCount++
		return
	}
	window.sketches = append(window.sketches, sketches)
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
		// CloseWindowOnDeadline: count > 1, apply per-mtype roll-up.
		// All series in a window share the same context (windowKey
		// includes ContextKey), so MType is consistent across the
		// buffered series; we read it from w.series[0].
		switch w.series[0].MType {
		case metrics.APIGaugeType:
			// Gauge-like series use `last` unless the final metric name
			// carries a known histogram aggregate suffix. The suffix
			// strategies improve histogram scalar semantics without
			// plumbing original MetricType through the metrics pipeline.
			sink.Append(aggregateGaugeLike(w.series))
		case metrics.APICountType:
			// `sum` strategy: covers both Count (sum-since-commit) and
			// MonotonicCount (per-commit delta) since CheckSampler maps
			// both to APICountType. Sum-of-sums and sum-of-deltas are
			// the correct window-total in each case.
			sink.Append(aggregateSum(w.series))
		case metrics.APIRateType:
			// `avg` strategy: average across per-commit rates. Correct
			// for CheckSampler-emitted Rate-via-APIRateType (e.g.
			// histogram .count suffix) because CheckSampler emits at
			// uniform commit intervals — averaging uniform-interval
			// per-commit rates equals the true window rate. The "avg of
			// rates ≠ true rate" warning applies only to non-uniform
			// commit cadences, which CheckSampler does not produce.
			sink.Append(aggregateAvg(w.series))
		default:
			// Unknown APIMetricType (future additions): conservative
			// `last` to satisfy the cost constraint while remaining
			// approximate. Cross-reference with the spec's MetricType
			// closed-set (Q5) — flagged for review if hit.
			sink.Append(aggregateLast(w.series))
		}
	}
}

// aggregateLast returns a single *Serie carrying the latest sample's
// value, stamped at the latest sample's timestamp. The other metadata
// (Name, Tags, Host, MType, etc.) is taken from the last series since
// all entries in a window share the same context.
func aggregateLast(series []*metrics.Serie) *metrics.Serie {
	last, lastPoint := latestPoint(series)
	return cloneSerieWithSinglePoint(last, lastPoint.Ts, lastPoint.Value)
}

func aggregateGaugeLike(series []*metrics.Serie) *metrics.Serie {
	name := series[0].Name
	switch {
	case strings.HasSuffix(name, ".max"):
		return aggregateMax(series)
	case strings.HasSuffix(name, ".min"):
		return aggregateMin(series)
	case strings.HasSuffix(name, ".sum"):
		return aggregateSum(series)
	default:
		return aggregateLast(series)
	}
}

func aggregateMax(series []*metrics.Serie) *metrics.Serie {
	last, lastPoint := latestPoint(series)
	max := series[0].Points[0].Value
	for _, s := range series {
		for _, p := range s.Points {
			if p.Value > max {
				max = p.Value
			}
		}
	}
	return cloneSerieWithSinglePoint(last, lastPoint.Ts, max)
}

func aggregateMin(series []*metrics.Serie) *metrics.Serie {
	last, lastPoint := latestPoint(series)
	min := series[0].Points[0].Value
	for _, s := range series {
		for _, p := range s.Points {
			if p.Value < min {
				min = p.Value
			}
		}
	}
	return cloneSerieWithSinglePoint(last, lastPoint.Ts, min)
}

// aggregateSum returns a single *Serie carrying the sum of all values
// across the buffered series, stamped at the latest sample's timestamp.
// Both Count (per-commit sums) and MonotonicCount (per-commit deltas)
// produce APICountType Series for which sum-across-window is the
// correct window total.
func aggregateSum(series []*metrics.Serie) *metrics.Serie {
	var total float64
	for _, s := range series {
		for _, p := range s.Points {
			total += p.Value
		}
	}
	last, lastPoint := latestPoint(series)
	return cloneSerieWithSinglePoint(last, lastPoint.Ts, total)
}

// aggregateAvg returns a single *Serie carrying the arithmetic mean of
// all values across the buffered series, stamped at the latest sample's
// timestamp. Used for APIRateType: at uniform commit cadence (which
// CheckSampler produces), the mean of per-commit rates equals the true
// rate over the window.
func aggregateAvg(series []*metrics.Serie) *metrics.Serie {
	var total float64
	var count int
	for _, s := range series {
		for _, p := range s.Points {
			total += p.Value
			count++
		}
	}
	last, lastPoint := latestPoint(series)
	avg := total / float64(count)
	return cloneSerieWithSinglePoint(last, lastPoint.Ts, avg)
}

func latestPoint(series []*metrics.Serie) (*metrics.Serie, metrics.Point) {
	latestSeries := series[0]
	latest := series[0].Points[0]
	for _, s := range series {
		for _, p := range s.Points {
			if p.Ts >= latest.Ts {
				latestSeries = s
				latest = p
			}
		}
	}
	return latestSeries, latest
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
	switch len(w.sketches) {
	case 0:
		// DropEmptyWindowOnDeadline.
		return
	case 1:
		// SingletonWindowPassThrough: emit the single SketchSeries
		// unchanged. Preserves all original SketchPoints and timestamps.
		sink.Append(w.sketches[0])
		return
	default:
		// Merge all buffered sketches into one. The output is a single
		// SketchSeries carrying one SketchPoint with the merged sketch,
		// stamped at the latest commit's timestamp.
		merged := mergeSketches(w.sketches)
		if merged != nil {
			sink.Append(merged)
		}
	}
}

// mergeSketches collapses N buffered SketchSeries into a single output
// SketchSeries by merging the underlying *quantile.Sketch points.
//
// Returns nil if there are no usable sketches to merge (defensive).
// The merge uses Sketch.Copy() to avoid mutating any input — the caller
// may still reference the buffered Series until close completes.
//
// The merged sketch's wire size is bounded by the value-range log of the
// inputs, not by their count — so this is the path with the cleanest
// cost/fidelity tradeoff without storing sketches in the observer.
func mergeSketches(sketches []*metrics.SketchSeries) *metrics.SketchSeries {
	if len(sketches) == 0 {
		return nil
	}

	// Initialize merged from a copy of the first concrete *quantile.Sketch
	// we find. Skip nil / unexpected types defensively.
	var merged *quantile.Sketch
	var latestTs int64
	for _, ss := range sketches {
		for _, sp := range ss.Points {
			s, ok := sp.Sketch.(*quantile.Sketch)
			if !ok || s == nil {
				continue
			}
			if merged == nil {
				merged = s.Copy()
			} else {
				merged.Merge(quantile.Default(), s)
			}
			if sp.Ts > latestTs {
				latestTs = sp.Ts
			}
		}
	}
	if merged == nil {
		return nil
	}

	// Clone metadata from the last input series, replace Points with one
	// merged SketchPoint at the latest timestamp.
	last := sketches[len(sketches)-1]
	out := *last
	out.Points = []metrics.SketchPoint{{Sketch: merged, Ts: latestTs}}
	return &out
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
