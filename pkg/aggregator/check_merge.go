// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// checkOutputMerger merges the outputs of check samplers that filter to the same series, from one check instance or
// several, before they are sent: the intake keeps only the last point it receives for a series and second. Counts
// and rates are summed, and sketches merged, per series and second, each with the metadata of its own sources.
//
// It sends each second of a series once. Samplers stamp what they hand it at commit, and the aggregator's run loop
// handles commits and flushes one at a time, so a commit handled after a flush is stamped at or after its cutoff: a
// flush sends the seconds before its cutoff, and holds the rest for the next flush to merge with later commits. A
// metric that starts being filtered has its points moved past the latest second committed before, which its output
// sent before may have used. After a backward clock step, a point in a second already sent moves to the
// next unsent one, unless the step is maxShiftSeconds or longer.
type checkOutputMerger struct {
	series            map[serieSecond]*metrics.Serie
	sketches          map[sketchSecond]*metrics.SketchSeries
	lastSentSecond    int64
	notBeforeByMetric map[string]int64
	maxShiftSeconds   int64
}

// mergedSketch is a sketch series handed to the merger, with the key of the series it merges into.
type mergedSketch struct {
	key    ckey.ContextKey
	sketch *metrics.SketchSeries
}

// serieSecond identifies a second of a merged series.
type serieSecond struct {
	contextKey ckey.ContextKey
	second     int64
}

// sketchSecond identifies a second of a merged sketch series.
type sketchSecond struct {
	contextKey ckey.ContextKey
	second     int64
}

// maxHold is how far, in seconds, ahead of a flush's cutoff points can be held: only a backward clock step gets them
// that far ahead, and the clock won't reach them soon.
const maxHold = int64(time.Hour / time.Second)

// newCheckOutputMerger returns a merger that moves points by at most maxShift, the flush interval, and at least a
// second.
func newCheckOutputMerger(maxShift time.Duration) *checkOutputMerger {
	return &checkOutputMerger{
		series:            make(map[serieSecond]*metrics.Serie),
		sketches:          make(map[sketchSecond]*metrics.SketchSeries),
		notBeforeByMetric: make(map[string]int64),
		maxShiftSeconds:   max(1, int64(maxShift/time.Second)),
	}
}

// mergeSeries merges series with the other sources of the same series, which their ContextKey identifies.
func (m *checkOutputMerger) mergeSeries(series []*metrics.Serie) {
	for _, serie := range series {
		for _, point := range serie.Points {
			second := m.unsentSecond(serie.Name, int64(point.Ts))
			key := serieSecond{contextKey: serie.ContextKey, second: second}
			merged, held := m.series[key]
			if !held {
				merged = new(metrics.Serie)
				*merged = *serie
				merged.Points = []metrics.Point{{Ts: float64(second)}}
				m.series[key] = merged
			} else {
				merged.NoIndex = merged.NoIndex || serie.NoIndex
				merged.Source = min(merged.Source, serie.Source)
			}
			merged.Points[0].Value += point.Value
		}
	}
}

// mergeSketches merges sketch series with the other sources of the same series.
func (m *checkOutputMerger) mergeSketches(sketches []mergedSketch) {
	for _, s := range sketches {
		for _, point := range s.sketch.Points {
			second := m.unsentSecond(s.sketch.Name, point.Ts)
			key := sketchSecond{contextKey: s.key, second: second}
			merged, held := m.sketches[key]
			if !held {
				merged = new(metrics.SketchSeries)
				*merged = *s.sketch
				merged.Points = []metrics.SketchPoint{{Sketch: point.Sketch, Ts: second}}
				m.sketches[key] = merged
				continue
			}
			merged.NoIndex = merged.NoIndex || s.sketch.NoIndex
			merged.Source = min(merged.Source, s.sketch.Source)
			merged.Points[0].Sketch.Merge(quantile.Default(), point.Sketch)
		}
	}
}

// unsentSecond returns the second a point of a metric is sent in: its own, unless the merger has already sent it, or
// output sent before the metric started being filtered may have used it. Then it's the next second the point can
// use, if that's at most maxShiftSeconds later.
func (m *checkOutputMerger) unsentSecond(name string, ts int64) int64 {
	if clockSteppedBackTooFar := m.lastSentSecond-ts >= m.maxShiftSeconds; clockSteppedBackTooFar {
		m.lastSentSecond = ts - 1
	}
	moveTo := max(m.lastSentSecond+1, m.notBeforeByMetric[name])
	if moveTo-ts > m.maxShiftSeconds {
		return ts
	}
	return max(ts, moveTo)
}

// startedFiltering records, for metrics that started being filtered, the first second their points can use: their
// output didn't come to the merger before, and output sent before may have used the earlier seconds.
func (m *checkOutputMerger) startedFiltering(notBefore map[string]int64) {
	for name, second := range notBefore {
		m.notBeforeByMetric[name] = second
	}
}

// flush sends the seconds before cutoff, and holds the others for a later flush, unless they're more than maxHold
// after cutoff.
func (m *checkOutputMerger) flush(cutoff int64, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	m.send(func(second int64) bool { return second < cutoff || second > cutoff+maxHold }, seriesSink, sketchesSink)
	for name, second := range m.notBeforeByMetric {
		if second <= cutoff {
			delete(m.notBeforeByMetric, name)
		}
	}
}

// drain sends every second, for when no flush follows to send the seconds flush would hold.
func (m *checkOutputMerger) drain(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	m.send(func(int64) bool { return true }, seriesSink, sketchesSink)
}

func (m *checkOutputMerger) send(ready func(second int64) bool, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	for key, serie := range m.series {
		if ready(key.second) {
			m.lastSentSecond = max(m.lastSentSecond, key.second)
			seriesSink.Append(serie)
			delete(m.series, key)
		}
	}
	for key, sketch := range m.sketches {
		if ready(key.second) {
			m.lastSentSecond = max(m.lastSentSecond, key.second)
			sketchesSink.Append(sketch)
			delete(m.sketches, key)
		}
	}
}
