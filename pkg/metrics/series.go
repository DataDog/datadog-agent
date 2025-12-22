// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

const (
	// internalResourceTagPrefix is the tag name used for propagating resources to be emitted on metrics.
	// The format for the tag is dd.internal.resource:resource_type,resource_name. Resource names
	// should comply with the Datadog tagging requirements documented at
	// https://docs.datadoghq.com/getting_started/tagging/#define-tags.
	// Note: resources are only supported on metrics api v2.
	internalResourceTagPrefix    = "dd.internal.resource:"
	internalResourceTagSeparator = ":"
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    float64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
// Note: it is not used with jsoniter, encodePoints takes over
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", int64(p.Ts), p.Value)), nil
}

// Resource holds a resource name and type
type Resource struct {
	Name string
	Type string
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name           string               `json:"metric"`
	Points         []Point              `json:"points"`
	Tags           tagset.CompositeTags `json:"tags"`
	Host           string               `json:"host"`
	Device         string               `json:"device,omitempty"`
	MType          APIMetricType        `json:"type"`
	Interval       int64                `json:"interval"`
	SourceTypeName string               `json:"source_type_name,omitempty"`
	ContextKey     ckey.ContextKey      `json:"-"`
	NameSuffix     string               `json:"-"`
	NoIndex        bool                 `json:"-"` // This is only used by api V2
	Resources      []Resource           `json:"-"` // This is only used by api V2
	Source         MetricSource         `json:"-"` // This is only used by api V2
}

// GetName returns the name of the Serie
func (serie *Serie) GetName() string {
	return serie.Name
}

// Metadata holds metadata about the metric
type Metadata struct {
	Origin Origin `json:"origin,omitempty"`
}

// Origin holds the metric origins metadata
type Origin struct {
	OriginProduct       int32 `json:"origin_product,omitempty"`
	OriginSubProduct    int32 `json:"origin_sub_product,omitempty"`
	OriginProductDetail int32 `json:"origin_product_detail,omitempty"`
}

// SeriesAPIV2Enum returns the enumeration value for MetricPayload.MetricType in
// https://github.com/DataDog/agent-payload/blob/master/proto/metrics/agent_payload.proto
func (a APIMetricType) SeriesAPIV2Enum() int32 {
	switch a {
	case APICountType:
		return 1
	case APIRateType:
		return 2
	case APIGaugeType:
		return 3
	default:
		panic("unknown APIMetricType")
	}
}

// UnmarshalJSON is a custom unmarshaller for Point (used for testing)
func (p *Point) UnmarshalJSON(buf []byte) error {
	tmp := []interface{}{&p.Ts, &p.Value}
	wantLen := len(tmp)
	if err := json.Unmarshal(buf, &tmp); err != nil {
		return err
	}
	if len(tmp) != wantLen {
		return fmt.Errorf("wrong number of fields in Point: %d != %d", len(tmp), wantLen)
	}
	return nil
}

// String could be used for debug logging
func (serie Serie) String() string {
	s, err := json.Marshal(serie)
	if err != nil {
		return ""
	}
	return string(s)
}

// PopulateDeviceField removes any `device:` tag in the series tags and uses the value to
// populate the Serie.Device field
// FIXME(olivier v): remove this as soon as the v1 API can handle `device` as a regular tag
func (serie *Serie) PopulateDeviceField() {
	if !serie.hasDeviceTag() {
		return
	}
	// make a copy of the tags array. Otherwise the underlying array won't have
	// the device tag for the Nth iteration (N>1), and the device field will
	// be lost
	filteredTags := make([]string, 0, serie.Tags.Len())

	serie.Tags.ForEach(func(tag string) {
		if strings.HasPrefix(tag, "device:") {
			serie.Device = tag[7:]
		} else {
			filteredTags = append(filteredTags, tag)
		}
	})

	serie.Tags = tagset.CompositeTagsFromSlice(filteredTags)
}

// hasDeviceTag checks whether a series contains a device tag
func (serie *Serie) hasDeviceTag() bool {
	return serie.Tags.Find(func(tag string) bool {
		return strings.HasPrefix(tag, "device:")
	})
}

// PopulateResources removes any dd.internal.resource tags in the series tags and uses the values to
// populate the Serie.Resources field. The format for the dd.internal.resource tag values is
// <resource_type>:<resource_name>. Any dd.internal.resource tag not matching the expected format
// will be dropped.
func (serie *Serie) PopulateResources() {
	if !serie.hasResourceTag() {
		return
	}
	// make a copy of the tags array. Otherwise the underlying array won't have
	// the resource tag for the Nth iteration (N>1), and the resources field will
	// be lost
	filteredTags := make([]string, 0, serie.Tags.Len())

	serie.Tags.ForEach(func(tag string) {
		if strings.HasPrefix(tag, internalResourceTagPrefix) {
			tagVal := tag[len(internalResourceTagPrefix):]
			commaIdx := strings.Index(tagVal, internalResourceTagSeparator)
			if commaIdx > 0 && commaIdx < len(tagVal)-1 {
				resource := Resource{
					Type: tagVal[:commaIdx],
					Name: tagVal[commaIdx+1:],
				}
				serie.Resources = append(serie.Resources, resource)
			}
		} else {
			filteredTags = append(filteredTags, tag)
		}
	})

	serie.Tags = tagset.CompositeTagsFromSlice(filteredTags)
}

// hasResourceTag checks whether a series contains a resource tag
func (serie *Serie) hasResourceTag() bool {
	return serie.Tags.Find(func(tag string) bool {
		return strings.HasPrefix(tag, internalResourceTagPrefix)
	})
}

// Series is a collection of `Serie`
type Series []*Serie

// SerieSink is a sink for series.
// It provides a way to append a serie into `Series` or `IterableSerie`
type SerieSink interface {
	Append(*Serie)
}

// Append appends a serie into series. Implement `SerieSink` interface.
func (series *Series) Append(serie *Serie) {
	*series = append(*series, serie)
}

// MarshalStrings converts the timeseries to a sorted slice of string slices
func (series Series) MarshalStrings() ([]string, [][]string) {
	headers := []string{"Metric", "Type", "Timestamp", "Value", "Tags"}
	payload := make([][]string, len(series))

	for _, serie := range series {
		payload = append(payload, []string{
			serie.Name,
			serie.MType.String(),
			strconv.FormatFloat(serie.Points[0].Ts, 'f', 0, 64),
			strconv.FormatFloat(serie.Points[0].Value, 'f', -1, 64),
			serie.Tags.Join(", "),
		})
	}

	sort.Slice(payload, func(i, j int) bool {
		// edge cases
		if len(payload[i]) == 0 && len(payload[j]) == 0 {
			return false
		}
		if len(payload[i]) == 0 || len(payload[j]) == 0 {
			return len(payload[i]) == 0
		}
		// sort by metric name
		if payload[i][0] != payload[j][0] {
			return payload[i][0] < payload[j][0]
		}
		// then by timestamp
		if payload[i][2] != payload[j][2] {
			return payload[i][2] < payload[j][2]
		}
		// finally by tags (last field) as tie breaker
		return payload[i][len(payload[i])-1] < payload[j][len(payload[j])-1]
	})

	return headers, payload
}
