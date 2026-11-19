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
// several, before they are sent: the intake keeps only the last point it receives for a series and timestamp. Counts
// and rates are summed, and sketches merged, per series and timestamp, each with the metadata of its own sources.
//
// It sends each timestamp of a series once, as long as the clock doesn't step back. Samplers stamp what they hand it
// at commit, and the aggregator's run loop handles commits and flushes one at a time, so a commit handled after a
// flush is stamped at or after its cutoff: a flush sends the timestamps before its cutoff, and holds the others for
// the next flush to merge with later commits.
//
// Like the rest of the Agent's output, it doesn't guard against sending a timestamp twice when the clock steps back,
// or when a rule change moves a source between merged and unmerged output in the second it changes.
//
// Points are held per timestamp, of which there are only a few at a time: a flush checks each timestamp once, and
// drops the series it sends along with their map.
type checkOutputMerger struct {
	series   map[int64]map[ckey.ContextKey]*metrics.Serie
	sketches map[int64]map[ckey.ContextKey]*metrics.SketchSeries
}

// mergedSketch is a sketch series handed to the merger, with the key of the series it merges into.
type mergedSketch struct {
	key    ckey.ContextKey
	sketch *metrics.SketchSeries
}

// maxHold is how far, in seconds, ahead of a flush's cutoff points can be held: only a backward clock step gets them
// that far ahead, and the clock won't reach them soon.
const maxHold = int64(time.Hour / time.Second)

// newCheckOutputMerger returns an empty merger.
func newCheckOutputMerger() *checkOutputMerger {
	return &checkOutputMerger{
		series:   make(map[int64]map[ckey.ContextKey]*metrics.Serie),
		sketches: make(map[int64]map[ckey.ContextKey]*metrics.SketchSeries),
	}
}

// heldAt returns the map of what's held for a timestamp, creating it if there is none.
func heldAt[T any](held map[int64]map[ckey.ContextKey]T, timestamp int64) map[ckey.ContextKey]T {
	byKey, ok := held[timestamp]
	if !ok {
		byKey = make(map[ckey.ContextKey]T)
		held[timestamp] = byKey
	}
	return byKey
}

// mergeSeries merges series with the other sources of the same series, which their ContextKey identifies.
func (m *checkOutputMerger) mergeSeries(series []*metrics.Serie) {
	for _, serie := range series {
		for _, point := range serie.Points {
			timestamp := int64(point.Ts)
			byKey := heldAt(m.series, timestamp)
			merged, held := byKey[serie.ContextKey]
			if !held {
				merged = new(metrics.Serie)
				*merged = *serie
				merged.Points = []metrics.Point{{Ts: float64(timestamp)}}
				byKey[serie.ContextKey] = merged
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
			byKey := heldAt(m.sketches, point.Ts)
			merged, held := byKey[s.key]
			if !held {
				merged = new(metrics.SketchSeries)
				*merged = *s.sketch
				merged.Points = []metrics.SketchPoint{{Sketch: point.Sketch, Ts: point.Ts}}
				byKey[s.key] = merged
				continue
			}
			merged.NoIndex = merged.NoIndex || s.sketch.NoIndex
			merged.Source = min(merged.Source, s.sketch.Source)
			merged.Points[0].Sketch.Merge(quantile.Default(), point.Sketch)
		}
	}
}

// flush sends the timestamps before cutoff, and holds the others for a later flush, unless they're more than maxHold
// after cutoff.
func (m *checkOutputMerger) flush(cutoff int64, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	m.send(func(timestamp int64) bool { return timestamp < cutoff || timestamp > cutoff+maxHold }, seriesSink, sketchesSink)
}

// drain sends every timestamp, for when no flush follows to send the timestamps flush would hold.
func (m *checkOutputMerger) drain(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	m.send(func(int64) bool { return true }, seriesSink, sketchesSink)
}

func (m *checkOutputMerger) send(ready func(timestamp int64) bool, seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	for timestamp, byKey := range m.series {
		if ready(timestamp) {
			for _, serie := range byKey {
				seriesSink.Append(serie)
			}
			delete(m.series, timestamp)
		}
	}
	for timestamp, byKey := range m.sketches {
		if ready(timestamp) {
			for _, sketch := range byKey {
				sketchesSink.Append(sketch)
			}
			delete(m.sketches, timestamp)
		}
	}
}
