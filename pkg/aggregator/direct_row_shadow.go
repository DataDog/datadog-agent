// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	tlmDirectRowShadowStats = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_direct_row_shadow", "stats",
		[]string{"stat"}, "Output-neutral direct aggregator row shadow stats")
	tlmDirectRowShadowDictionaryEntries = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_direct_row_shadow", "dictionary_entries",
		[]string{"dictionary"}, "Output-neutral direct aggregator row shadow dictionary entry counts")
	tlmDirectRowShadowDuration = telemetryimpl.GetCompatComponent().NewCounter("dogstatsd_direct_row_shadow", "duration_ns",
		[]string{"phase"}, "Cumulative direct aggregator row shadow duration, in nanoseconds")
)

type directRowShadowBuilder struct {
	names           map[string]struct{}
	tagStrings      map[string]struct{}
	tagsets         map[string]struct{}
	resourceStrings map[string]struct{}
	resources       map[string]struct{}
	sources         map[string]struct{}
	units           map[string]struct{}

	seriesRows int
	sketchRows int
	points     int
	sketchBins int
	tags       int
	estBytes   int
	fallbacks  int
}

func directRowShadowTelemetryEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROW_SHADOW_TELEMETRY"))
	return err == nil && enabled
}

func newOptionalDirectRowShadowBuilder() *directRowShadowBuilder {
	if !directRowShadowTelemetryEnabled() {
		return nil
	}
	return newDirectRowShadowBuilder()
}

func newDirectRowShadowBuilder() *directRowShadowBuilder {
	return &directRowShadowBuilder{
		names:           map[string]struct{}{},
		tagStrings:      map[string]struct{}{},
		tagsets:         map[string]struct{}{},
		resourceStrings: map[string]struct{}{},
		resources:       map[string]struct{}{},
		sources:         map[string]struct{}{},
		units:           map[string]struct{}{},
	}
}

func finishDirectRowShadow(builder *directRowShadowBuilder, phase string, start time.Time) {
	if builder == nil {
		return
	}
	builder.finish(phase, time.Since(start))
}

func (b *directRowShadowBuilder) observeSerie(serie *metrics.Serie) {
	if b == nil {
		return
	}
	if serie == nil {
		b.fallbacks++
		return
	}
	row := metrics.SerieRowFromSerie(serie)
	b.observeSerieRow(&row)
}

func (b *directRowShadowBuilder) observeSerieRow(row *metrics.SerieRow) {
	if b == nil {
		return
	}
	if row == nil {
		b.fallbacks++
		return
	}

	b.seriesRows++
	b.points += len(row.Points)
	b.estBytes += 32 + len(row.Points)*16

	b.internName(row.Name)
	tags := row.Tags.UnsafeToReadOnlySliceString()
	b.tags += len(tags)
	b.internTags(tags)
	b.internSource(row.Source)
	b.internUnit(row.Unit)

	resources := make([]metrics.Resource, 0, len(row.Resources)+2)
	if row.Host != "" {
		resources = append(resources, metrics.Resource{Type: "host", Name: row.Host})
	}
	if row.Device != "" {
		resources = append(resources, metrics.Resource{Type: "device", Name: row.Device})
	}
	resources = append(resources, row.Resources...)
	b.internResources(resources)
}

func (b *directRowShadowBuilder) observeV3MetricPointRow(row *metrics.V3MetricPointRow) {
	if b == nil {
		return
	}
	if row == nil {
		b.fallbacks++
		return
	}

	numPoints := row.NumPoints()
	b.seriesRows++
	b.points += numPoints
	b.estBytes += 32 + numPoints*16

	b.internName(row.Name)
	tags := row.Tags.UnsafeToReadOnlySliceString()
	b.tags += len(tags)
	b.internTags(tags)
	b.internSource(row.Source)
	b.internUnit(row.Unit)

	resources := make([]metrics.Resource, 0, len(row.Resources)+2)
	if row.Host != "" {
		resources = append(resources, metrics.Resource{Type: "host", Name: row.Host})
	}
	if row.Device != "" {
		resources = append(resources, metrics.Resource{Type: "device", Name: row.Device})
	}
	resources = append(resources, row.Resources...)
	b.internResources(resources)
}

func (b *directRowShadowBuilder) observeSketch(sketch *metrics.SketchSeries) {
	if b == nil {
		return
	}
	if sketch == nil {
		b.fallbacks++
		return
	}

	b.sketchRows++
	b.points += len(sketch.Points)
	b.estBytes += 32 + len(sketch.Points)*24

	b.internName(sketch.Name)
	tags := sketch.Tags.UnsafeToReadOnlySliceString()
	b.tags += len(tags)
	b.internTags(tags)
	b.internSource(sketch.Source)

	if sketch.Host != "" {
		b.internResources([]metrics.Resource{{Type: "host", Name: sketch.Host}})
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

func (b *directRowShadowBuilder) finish(phase string, duration time.Duration) {
	if b == nil {
		return
	}
	tlmDirectRowShadowDuration.Add(float64(duration.Nanoseconds()), phase)
	tlmDirectRowShadowStats.Inc("flushes")
	tlmDirectRowShadowStats.Add(float64(b.seriesRows), "series_rows")
	tlmDirectRowShadowStats.Add(float64(b.sketchRows), "sketch_rows")
	tlmDirectRowShadowStats.Add(float64(b.points), "points")
	tlmDirectRowShadowStats.Add(float64(b.sketchBins), "sketch_bins")
	tlmDirectRowShadowStats.Add(float64(b.tags), "tags")
	tlmDirectRowShadowStats.Add(float64(b.estBytes), "estimated_uncompressed_bytes")
	tlmDirectRowShadowStats.Add(float64(b.fallbacks), "fallbacks")

	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.names)), "names")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.tagStrings)), "tag_strings")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.tagsets)), "tagsets")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.resourceStrings)), "resource_strings")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.resources)), "resources")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.sources)), "sources")
	tlmDirectRowShadowDictionaryEntries.Add(float64(len(b.units)), "units")
}

func (b *directRowShadowBuilder) internName(name string) {
	if name == "" {
		return
	}
	if _, ok := b.names[name]; ok {
		return
	}
	b.names[name] = struct{}{}
	b.estBytes += len(name)
}

func (b *directRowShadowBuilder) internUnit(unit string) {
	if unit == "" {
		return
	}
	if _, ok := b.units[unit]; ok {
		return
	}
	b.units[unit] = struct{}{}
	b.estBytes += len(unit)
}

func (b *directRowShadowBuilder) internSource(source metrics.MetricSource) {
	b.sources[fmt.Sprintf("%d", source)] = struct{}{}
}

func (b *directRowShadowBuilder) internTags(tags []string) {
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

func (b *directRowShadowBuilder) internResources(resources []metrics.Resource) {
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
