// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"slices"
	"strings"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	tlmSegmentShadowStats = telemetryimpl.GetCompatComponent().NewCounter("serializer", "segment_shadow_stats",
		[]string{"stat"}, "Shadow payload-aligned segment builder stats")
	tlmSegmentShadowDictionaryEntries = telemetryimpl.GetCompatComponent().NewCounter("serializer", "segment_shadow_dictionary_entries",
		[]string{"dictionary"}, "Shadow payload-aligned segment builder dictionary entry counts")
	tlmSegmentShadowDuration = telemetryimpl.GetCompatComponent().NewCounter("serializer", "segment_shadow_duration_ns",
		[]string{"phase"}, "Cumulative shadow payload-aligned segment builder duration, in nanoseconds")
)

type segmentShadowBuilder struct {
	names           map[string]struct{}
	tagStrings      map[string]struct{}
	tagsets         map[string]struct{}
	resourceStrings map[string]struct{}
	resources       map[string]struct{}
	sourceTypeNames map[string]struct{}
	origins         map[string]struct{}
	units           map[string]struct{}

	seriesRows int
	sketchRows int
	points     int
	sketchBins int
	estBytes   int
	fallbacks  int
}

func newSegmentShadowBuilder() *segmentShadowBuilder {
	return &segmentShadowBuilder{
		names:           map[string]struct{}{},
		tagStrings:      map[string]struct{}{},
		tagsets:         map[string]struct{}{},
		resourceStrings: map[string]struct{}{},
		resources:       map[string]struct{}{},
		sourceTypeNames: map[string]struct{}{},
		origins:         map[string]struct{}{},
		units:           map[string]struct{}{},
	}
}

func (b *segmentShadowBuilder) observeSerie(serie *pkgmetrics.Serie) {
	if serie == nil {
		b.fallbacks++
		return
	}
	row := pkgmetrics.SerieRowFromSerie(serie)
	b.observeSerieRow(&row)
}

func (b *segmentShadowBuilder) observeSerieRow(row *pkgmetrics.SerieRow) {
	if row == nil {
		b.fallbacks++
		return
	}

	b.seriesRows++
	b.points += len(row.Points)
	b.estBytes += 32 + len(row.Points)*16

	b.internName(row.Name)
	b.internTags(row.Tags.UnsafeToReadOnlySliceString())
	b.internSourceTypeName(row.SourceTypeName)
	b.internOrigin(row.Source)
	b.internUnit(row.Unit)

	resources := make([]pkgmetrics.Resource, 0, len(row.Resources)+2)
	if row.Host != "" {
		resources = append(resources, pkgmetrics.Resource{Type: resourceTypeHost, Name: row.Host})
	}
	if row.Device != "" {
		resources = append(resources, pkgmetrics.Resource{Type: "device", Name: row.Device})
	}
	resources = append(resources, row.Resources...)
	b.internResources(resources)
}

func (b *segmentShadowBuilder) observeSketch(sketch *pkgmetrics.SketchSeries) {
	if sketch == nil {
		b.fallbacks++
		return
	}

	b.sketchRows++
	b.points += len(sketch.Points)
	b.estBytes += 32 + len(sketch.Points)*24

	b.internName(sketch.Name)
	b.internTags(sketch.Tags.UnsafeToReadOnlySliceString())
	b.internOrigin(sketch.Source)

	if sketch.Host != "" {
		b.internResources([]pkgmetrics.Resource{{Type: resourceTypeHost, Name: sketch.Host}})
	}

	for _, pnt := range sketch.Points {
		if pnt.Sketch == nil {
			b.fallbacks++
			continue
		}
		keys, _ := pnt.Sketch.Cols()
		b.sketchBins += len(keys)
		b.estBytes += len(keys) * 16
	}
}

func (b *segmentShadowBuilder) finish(phase string, duration time.Duration) {
	tlmSegmentShadowDuration.Add(float64(duration.Nanoseconds()), phase)
	tlmSegmentShadowStats.Inc("flushes")
	tlmSegmentShadowStats.Add(float64(b.seriesRows), "series_rows")
	tlmSegmentShadowStats.Add(float64(b.sketchRows), "sketch_rows")
	tlmSegmentShadowStats.Add(float64(b.points), "points")
	tlmSegmentShadowStats.Add(float64(b.sketchBins), "sketch_bins")
	tlmSegmentShadowStats.Add(float64(b.estBytes), "estimated_uncompressed_bytes")
	tlmSegmentShadowStats.Add(float64(b.fallbacks), "fallbacks")

	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.names)), "names")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.tagStrings)), "tag_strings")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.tagsets)), "tagsets")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.resourceStrings)), "resource_strings")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.resources)), "resources")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.sourceTypeNames)), "source_type_names")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.origins)), "origins")
	tlmSegmentShadowDictionaryEntries.Add(float64(len(b.units)), "units")
}

func (b *segmentShadowBuilder) internName(name string) {
	if name == "" {
		return
	}
	if _, ok := b.names[name]; ok {
		return
	}
	b.names[name] = struct{}{}
	b.estBytes += len(name)
}

func (b *segmentShadowBuilder) internSourceTypeName(sourceTypeName string) {
	if sourceTypeName == "" {
		return
	}
	if _, ok := b.sourceTypeNames[sourceTypeName]; ok {
		return
	}
	b.sourceTypeNames[sourceTypeName] = struct{}{}
	b.estBytes += len(sourceTypeName)
}

func (b *segmentShadowBuilder) internUnit(unit string) {
	if unit == "" {
		return
	}
	if _, ok := b.units[unit]; ok {
		return
	}
	b.units[unit] = struct{}{}
	b.estBytes += len(unit)
}

func (b *segmentShadowBuilder) internTags(tags []string) {
	if len(tags) == 0 {
		return
	}
	copied := append([]string(nil), tags...)
	slices.Sort(copied)
	key := strings.Join(copied, "\x00")
	if _, ok := b.tagsets[key]; ok {
		return
	}
	b.tagsets[key] = struct{}{}
	b.estBytes += len(copied) * 4
	for _, tag := range copied {
		if _, ok := b.tagStrings[tag]; ok {
			continue
		}
		b.tagStrings[tag] = struct{}{}
		b.estBytes += len(tag)
	}
}

func (b *segmentShadowBuilder) internResources(resources []pkgmetrics.Resource) {
	if len(resources) == 0 {
		return
	}
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		parts = append(parts, resource.Type+"\x00"+resource.Name)
		if _, ok := b.resourceStrings[resource.Type]; !ok {
			b.resourceStrings[resource.Type] = struct{}{}
			b.estBytes += len(resource.Type)
		}
		if _, ok := b.resourceStrings[resource.Name]; !ok {
			b.resourceStrings[resource.Name] = struct{}{}
			b.estBytes += len(resource.Name)
		}
	}
	slices.Sort(parts)
	key := strings.Join(parts, "\x00")
	if _, ok := b.resources[key]; ok {
		return
	}
	b.resources[key] = struct{}{}
	b.estBytes += len(resources) * 8
}

func (b *segmentShadowBuilder) internOrigin(source pkgmetrics.MetricSource) {
	key := fmt.Sprintf("%d:%d:%d", metricSourceToOriginProduct(source), metricSourceToOriginCategory(source), metricSourceToOriginService(source))
	b.origins[key] = struct{}{}
}
