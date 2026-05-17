// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// SerieRow is the serializer-visible representation of one metric series row.
//
// Unlike Serie, SerieRow is intended to be passed by value through producer-side
// direct serialization paths. It carries the final identity and row attributes
// needed by protobuf serializers without requiring the serializer to mutate a
// shared *Serie before writing each payload variant.
type SerieRow struct {
	Name           string
	Points         []Point
	Tags           tagset.CompositeTags
	Host           string
	Device         string
	MType          APIMetricType
	Interval       int64
	SourceTypeName string
	Unit           string
	NoIndex        bool
	Resources      []Resource
	Source         MetricSource
}

// SerieRowSink is implemented by sinks that can consume serializer-visible rows
// directly instead of requiring a fully materialized *Serie.
type SerieRowSink interface {
	AppendSerieRow(SerieRow)
}

// NewSerieRow constructs a normalized serializer row from its component fields.
func NewSerieRow(
	name string,
	points []Point,
	tags tagset.CompositeTags,
	host string,
	device string,
	mType APIMetricType,
	interval int64,
	sourceTypeName string,
	unit string,
	noIndex bool,
	resources []Resource,
	source MetricSource,
) SerieRow {
	row := SerieRow{
		Name:           name,
		Points:         points,
		Tags:           tags,
		Host:           host,
		Device:         device,
		MType:          mType,
		Interval:       interval,
		SourceTypeName: sourceTypeName,
		Unit:           unit,
		NoIndex:        noIndex,
		Resources:      resources,
		Source:         source,
	}
	row.NormalizeSpecialTags()
	return row
}

// SerieRowFromSerie creates a normalized row from a Serie without mutating the
// original Serie. Device and resource tags are projected into row fields using
// the same compatibility rules as Serie.PopulateDeviceField and
// Serie.PopulateResources.
func SerieRowFromSerie(serie *Serie) SerieRow {
	if serie == nil {
		return SerieRow{}
	}
	return NewSerieRow(
		serie.Name,
		serie.Points,
		serie.Tags,
		serie.Host,
		serie.Device,
		serie.MType,
		serie.Interval,
		serie.SourceTypeName,
		serie.Unit,
		serie.NoIndex,
		serie.Resources,
		serie.Source,
	)
}

// NormalizeSpecialTags projects tags that are represented as dedicated fields
// in the metrics protobuf API into Device/Resources and removes those tags from
// the row tagset. It intentionally mirrors Serie.PopulateDeviceField and
// Serie.PopulateResources without mutating a shared Serie.
func (row *SerieRow) NormalizeSpecialTags() {
	if row == nil || row.Tags.Len() == 0 {
		return
	}

	hasSpecialTag := row.Tags.Find(func(tag string) bool {
		return strings.HasPrefix(tag, "device:") || strings.HasPrefix(tag, internalResourceTagPrefix)
	})
	if !hasSpecialTag {
		return
	}

	filteredTags := make([]string, 0, row.Tags.Len())
	resources := row.Resources
	resourcesCopied := false

	row.Tags.ForEach(func(tag string) {
		switch {
		case strings.HasPrefix(tag, "device:"):
			row.Device = tag[7:]
		case strings.HasPrefix(tag, internalResourceTagPrefix):
			tagVal := tag[len(internalResourceTagPrefix):]
			commaIdx := strings.Index(tagVal, internalResourceTagSeparator)
			if commaIdx > 0 && commaIdx < len(tagVal)-1 {
				if !resourcesCopied {
					resources = append([]Resource(nil), resources...)
					resourcesCopied = true
				}
				resources = append(resources, Resource{
					Type: tagVal[:commaIdx],
					Name: tagVal[commaIdx+1:],
				})
			}
		default:
			filteredTags = append(filteredTags, tag)
		}
	})

	row.Tags = tagset.CompositeTagsFromSlice(filteredTags)
	row.Resources = resources
}

// GetName returns the metric name, allowing SerieRow to participate in
// serializer pipeline filtering.
func (row SerieRow) GetName() string {
	return row.Name
}

// ToSerie converts a row back to a Serie for compatibility with older sinks.
// It shares Points and Resources slices with the row.
func (row SerieRow) ToSerie() *Serie {
	return &Serie{
		Name:           row.Name,
		Points:         row.Points,
		Tags:           row.Tags,
		Host:           row.Host,
		Device:         row.Device,
		MType:          row.MType,
		Interval:       row.Interval,
		SourceTypeName: row.SourceTypeName,
		Unit:           row.Unit,
		NoIndex:        row.NoIndex,
		Resources:      row.Resources,
		Source:         row.Source,
	}
}

// String returns a JSON representation for debug logging.
func (row SerieRow) String() string {
	b, err := json.Marshal(row.ToSerie())
	if err != nil {
		return ""
	}
	return string(b)
}
