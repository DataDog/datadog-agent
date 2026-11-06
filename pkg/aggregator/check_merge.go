// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// checkOutputMerger merges the outputs of check samplers that strip to the same series, from one check instance or
// several, before they are sent: the intake keeps only the last point it receives for a series and second. Counts
// and rates are summed, and sketches merged, per series and second, each with the metadata of its own sources.
//
// Every flush sends what the samplers committed since the previous one: points for one second are merged when the
// same flush collects them.
type checkOutputMerger struct {
	series   map[serieSecond]*metrics.Serie
	sketches map[sketchSecond]*metrics.SketchSeries
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

func newCheckOutputMerger() *checkOutputMerger {
	return &checkOutputMerger{
		series:   make(map[serieSecond]*metrics.Serie),
		sketches: make(map[sketchSecond]*metrics.SketchSeries),
	}
}

// mergeSeries merges series with the other sources of the same series, which their ContextKey identifies.
func (m *checkOutputMerger) mergeSeries(series []*metrics.Serie) {
	for _, serie := range series {
		for _, point := range serie.Points {
			second := int64(point.Ts)
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
			second := point.Ts
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

// flush sends every second it holds.
func (m *checkOutputMerger) flush(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
	for key, serie := range m.series {
		seriesSink.Append(serie)
		delete(m.series, key)
	}
	for key, sketch := range m.sketches {
		sketchesSink.Append(sketch)
		delete(m.sketches, key)
	}
}
