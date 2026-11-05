// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/util"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

var tlmCheckMergeConflicts = telemetryimpl.GetCompatComponent().NewCounter("aggregator", "check_merge_conflicts",
	[]string{"conflict"}, "Sources of a tag-filtered check series that disagree on its type or origin")

// checkOutputMerger is where the outputs of check samplers meet before they are sent, to merge the sources of each
// tag-filtered series.
//
// Check samplers aggregate every source separately, so that state kept per source (previous values, last bucket
// counts) stays per source, and strip the tags of filtered metrics from their output. Sources that strip to the
// same series, whether in one check instance or several, then send points for the same series and second, and the
// intake keeps only the last point it receives for a series and second. The merger combines those points: counts
// and rates are summed, sketches merged. Points in different seconds stay apart.
//
// Samplers hand the merger the output of the contexts a rule strips, of the types that can be stripped. Other
// output doesn't go through the merger.
//
// Every flush sends what the samplers committed since the previous one: points for one second are merged when the
// same flush collects them.
type checkOutputMerger struct {
	series   map[serieMergeKey]*metrics.Serie
	sketches map[ckey.ContextKey]*metrics.SketchSeries

	logThrottling util.SimpleThrottler
}

// mergedSketch is a sketch series handed to the merger, with the key of the series it merges into.
type mergedSketch struct {
	key    ckey.ContextKey
	sketch *metrics.SketchSeries
}

// serieIdentity identifies a series: a context can have several, such as the count of a histogram.
type serieIdentity struct {
	contextKey ckey.ContextKey
	nameSuffix string
}

// serieMergeKey identifies the series points are merged into: points of different types are never added up.
type serieMergeKey struct {
	serieIdentity
	mtype metrics.APIMetricType
}

func newCheckOutputMerger() *checkOutputMerger {
	return &checkOutputMerger{
		series:   make(map[serieMergeKey]*metrics.Serie),
		sketches: make(map[ckey.ContextKey]*metrics.SketchSeries),

		logThrottling: util.NewSimpleThrottler(5, 5*time.Minute, ""),
	}
}

// mergeSeries keeps series to merge with the other sources of the same series, which their ContextKey identifies.
func (m *checkOutputMerger) mergeSeries(series []*metrics.Serie) {
	for _, serie := range series {
		identity := serieIdentity{contextKey: serie.ContextKey, nameSuffix: serie.NameSuffix}
		key := serieMergeKey{serieIdentity: identity, mtype: serie.MType}
		merged, held := m.series[key]
		if !held {
			m.reportTypeConflict(serie, identity)
			merged = new(metrics.Serie)
			*merged = *serie
			merged.Points = nil
			m.series[key] = merged
		} else {
			merged.NoIndex = merged.NoIndex || serie.NoIndex
			merged.Source = m.mergeSource(merged.Source, serie.Source)
		}
		for _, point := range serie.Points {
			point.Ts = float64(int64(point.Ts)) // merged per second
			merged.Points = mergeSeriePoint(merged.Points, point)
		}
	}
}

// reportTypeConflict reports a series held with another type: values of different types can't be added up, so
// both are sent, and the intake keeps one.
func (m *checkOutputMerger) reportTypeConflict(serie *metrics.Serie, identity serieIdentity) {
	for _, mtype := range []metrics.APIMetricType{metrics.APIGaugeType, metrics.APIRateType, metrics.APICountType} {
		if _, ok := m.series[serieMergeKey{serieIdentity: identity, mtype: mtype}]; !ok || mtype == serie.MType {
			continue
		}
		tlmCheckMergeConflicts.Inc("type")
		if !m.logThrottling.ShouldThrottle() {
			log.Warnf("Tag-filtered check metric %q is reported as both %s and %s by different sources: they can't be merged", serie.Name, mtype, serie.MType)
		}
		return
	}
}

// mergeSketches keeps sketch series to merge with the other sources of the same series.
func (m *checkOutputMerger) mergeSketches(sketches []mergedSketch) {
	for _, s := range sketches {
		merged, held := m.sketches[s.key]
		if !held {
			merged = new(metrics.SketchSeries)
			*merged = *s.sketch
			merged.Points = nil
			m.sketches[s.key] = merged
		} else {
			merged.NoIndex = merged.NoIndex || s.sketch.NoIndex
			merged.Source = m.mergeSource(merged.Source, s.sketch.Source)
		}
		for _, point := range s.sketch.Points {
			merged.Points = mergeSketchPoint(merged.Points, point)
		}
	}
}

// mergeSource returns the origin of a merged series: the lowest of its sources', so that it doesn't depend on
// the order sources are merged in.
func (m *checkOutputMerger) mergeSource(a, b metrics.MetricSource) metrics.MetricSource {
	if a != b {
		tlmCheckMergeConflicts.Inc("source")
	}
	return min(a, b)
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

// flush sends every point, merged, and empties the merger.
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
