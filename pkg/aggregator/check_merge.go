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

// checkOutputMerger is where the outputs of all check samplers meet before they are sent, so that the
// series of tag-filtered metrics are merged across check instances.
//
// Each check instance has its own sampler, so when a metric_tag_filterlist rule removes the tags that
// distinguish check instances (a pod name, with one autodiscovered instance per pod), several samplers
// emit points for the same series. The intake keeps the last point it receives for a series and second,
// so points that share a second are merged here instead of overwriting each other: counts and rates are
// summed, and sketches are merged. Points in different seconds are left as they are. Series whose tags
// were not filtered, or whose values cannot be summed, pass through unchanged.
//
// Check instances commit independently of flushes, so a flush holds the points that another instance can
// still commit a point to merge into. When that is depends on where a point's timestamp comes from:
//
//   - A counter point is timestamped when its instance commits, and every later commit is after the flush,
//     so only points in the second of the flush are held.
//   - A sketch point is timestamped when its samples were submitted, which can be long before they are
//     committed. A sketch point is held while a check sampler holds a sketch for its context and second
//     that it has not handed over yet, and while its samples could still be on their way to the
//     aggregator, which is assumed to take less than a flush interval: an aggregator slower than that is
//     already missing its flushes.
type checkOutputMerger struct {
	// sketchHold is how long, in seconds, a sketch point is held at least: the flush interval.
	sketchHold int64
	series     map[serieKey]*metrics.Serie
	sketches   map[ckey.ContextKey]*metrics.SketchSeries
}

// serieKey identifies a series: a context can have several, such as the statistics of a histogram.
type serieKey struct {
	contextKey ckey.ContextKey
	nameSuffix string
}

func newCheckOutputMerger(flushInterval time.Duration) *checkOutputMerger {
	return &checkOutputMerger{
		sketchHold: int64(flushInterval / time.Second),
		series:     make(map[serieKey]*metrics.Serie),
		sketches:   make(map[ckey.ContextKey]*metrics.SketchSeries),
	}
}

// appendSeries appends series to sink, except the counts and rates of tag-filtered metrics, which it keeps
// to merge.
func (m *checkOutputMerger) appendSeries(sink metrics.SerieSink, series []*metrics.Serie) {
	for _, serie := range series {
		summable := serie.MType == metrics.APICountType || serie.MType == metrics.APIRateType
		if !serie.TagFiltered || !summable {
			sink.Append(serie)
			continue
		}

		key := serieKey{contextKey: serie.ContextKey, nameSuffix: serie.NameSuffix}
		existing, ok := m.series[key]
		if !ok {
			m.series[key] = serie
			continue
		}
		for _, point := range serie.Points {
			existing.Points = mergeSeriePoint(existing.Points, point)
		}
	}
}

// appendSketches appends sketches to sink, except those of tag-filtered metrics, which it keeps to merge.
func (m *checkOutputMerger) appendSketches(sink metrics.SketchesSink, sketches metrics.SketchSeriesList) {
	for _, sketch := range sketches {
		if !sketch.TagFiltered {
			sink.Append(sketch)
			continue
		}

		existing, ok := m.sketches[sketch.ContextKey]
		if !ok {
			m.sketches[sketch.ContextKey] = sketch
			continue
		}
		for _, point := range sketch.Points {
			existing.Points = mergeSketchPoint(existing.Points, point)
		}
	}
}

func mergeSeriePoint(points []metrics.Point, point metrics.Point) []metrics.Point {
	for i := range points {
		if int64(points[i].Ts) == int64(point.Ts) {
			points[i].Value += point.Value
			return points
		}
	}
	return append(points, point)
}

func mergeSketchPoint(points []metrics.SketchPoint, point metrics.SketchPoint) []metrics.SketchPoint {
	for i := range points {
		if points[i].Ts == point.Ts {
			points[i].Sketch.Merge(quantile.Default(), point.Sketch)
			return points
		}
	}
	return append(points, point)
}

// flush sends the points kept to merge that no check instance can still commit a point to merge into, and
// holds the others for a later flush. cutoff is the Unix time in seconds of the flush, and isPending
// returns whether a check sampler holds a sketch for a context and second that it has not handed over yet.
func (m *checkOutputMerger) flush(cutoff int64, isPending func(ckey.ContextKey, int64) bool, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	for key, serie := range m.series {
		ready, held := splitPoints(serie.Points, func(p metrics.Point) bool { return int64(p.Ts) < cutoff })
		if len(ready) > 0 {
			sent := *serie
			sent.Points = ready
			seriesSink.Append(&sent)
		}
		if len(held) == 0 {
			delete(m.series, key)
		} else {
			serie.Points = held
		}
	}

	for key, sketch := range m.sketches {
		ready, held := splitPoints(sketch.Points, func(p metrics.SketchPoint) bool {
			return p.Ts < cutoff-m.sketchHold && !isPending(key, p.Ts)
		})
		if len(ready) > 0 {
			sent := *sketch
			sent.Points = ready
			sketchesSink.Append(&sent)
		}
		if len(held) == 0 {
			delete(m.sketches, key)
		} else {
			sketch.Points = held
		}
	}
}

// drain sends every point kept to merge, for when no flush follows to send the points flush would hold.
func (m *checkOutputMerger) drain(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	for _, serie := range m.series {
		seriesSink.Append(serie)
	}
	clear(m.series)

	for _, sketch := range m.sketches {
		sketchesSink.Append(sketch)
	}
	clear(m.sketches)
}

// splitPoints partitions points into those ready to send and those to hold, keeping their order.
func splitPoints[P any](points []P, isReady func(P) bool) (ready, held []P) {
	for _, p := range points {
		if isReady(p) {
			ready = append(ready, p)
		} else {
			held = append(held, p)
		}
	}
	return ready, held
}
