// Copyright  The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"sort"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/utils"
)

// Dimensions of a metric that identify a timeseries uniquely.
// This is similar to the concept of 'context' in DogStatsD/check metrics.
// NOTE: Keep this in sync with the TestDimensions struct.
type Dimensions struct {
	name     string
	tags     []string
	host     string
	originID string

	originProduct       OriginProduct
	originSubProduct    OriginSubProduct
	originProductDetail OriginProductDetail
}

// Name of the metric.
func (d *Dimensions) Name() string {
	return d.name
}

// Tags of the metric (read-only).
func (d *Dimensions) Tags() []string {
	return d.tags
}

// Host of the metric (may be empty).
func (d *Dimensions) Host() string {
	return d.host
}

// OriginID of the metric (may be empty).
func (d *Dimensions) OriginID() string {
	return d.originID
}

// OriginProduct of the metric.
func (d *Dimensions) OriginProduct() OriginProduct {
	return d.originProduct
}

// OriginSubProduct of the metric.
func (d *Dimensions) OriginSubProduct() OriginSubProduct {
	return d.originSubProduct
}

// OriginProductDetail of the metric.
func (d *Dimensions) OriginProductDetail() OriginProductDetail {
	return d.originProductDetail
}

// AddTags to metrics dimensions.
func (d *Dimensions) AddTags(tags ...string) *Dimensions {
	// defensively copy the tags
	newTags := make([]string, 0, len(tags)+len(d.tags))
	newTags = append(newTags, tags...)
	newTags = append(newTags, d.tags...)
	return &Dimensions{
		name:                d.name,
		tags:                newTags,
		host:                d.host,
		originID:            d.originID,
		originProduct:       d.originProduct,
		originSubProduct:    d.originSubProduct,
		originProductDetail: d.originProductDetail,
	}
}

// WithAttributeMap creates a new metricDimensions struct with additional tags from attributes.
func (d *Dimensions) WithAttributeMap(labels pcommon.Map) *Dimensions {
	n := labels.Len()
	if n == 0 {
		return d
	}
	newTags := make([]string, 0, n+len(d.tags))
	labels.Range(func(key string, value pcommon.Value) bool {
		newTags = append(newTags, utils.FormatKeyValueTag(key, value.AsString()))
		return true
	})
	newTags = append(newTags, d.tags...)
	return &Dimensions{
		name:                d.name,
		tags:                newTags,
		host:                d.host,
		originID:            d.originID,
		originProduct:       d.originProduct,
		originSubProduct:    d.originSubProduct,
		originProductDetail: d.originProductDetail,
	}
}

// WithSuffix creates a new dimensions struct with an extra name suffix.
func (d *Dimensions) WithSuffix(suffix string) *Dimensions {
	name := d.name + "." + suffix
	return &Dimensions{
		name:                name,
		host:                d.host,
		tags:                d.tags,
		originID:            d.originID,
		originProduct:       d.originProduct,
		originSubProduct:    d.originSubProduct,
		originProductDetail: d.originProductDetail,
	}
}

// String maps dimensions to a string to use as an identifier.
// The tags order does not matter.
func (d *Dimensions) String() string {
	// Pre-compute fixed suffixes: "name:<n>\x00host:<h>\x00originID:<o>\x00"
	// and sort tags in place on a temporary copy.
	dimensions := make([]string, len(d.tags), len(d.tags)+3)
	copy(dimensions, d.tags)
	dimensions = append(dimensions, "name:"+d.name)
	dimensions = append(dimensions, "host:"+d.host)
	dimensions = append(dimensions, "originID:"+d.originID)
	sort.Strings(dimensions)

	// Compute total capacity to avoid Builder re-allocations.
	total := 0
	for _, dim := range dimensions {
		total += len(dim) + 1 // +1 for dimensionSeparator (single byte 0)
	}
	var metricKeyBuilder strings.Builder
	metricKeyBuilder.Grow(total)
	for _, dim := range dimensions {
		metricKeyBuilder.WriteString(dim)
		metricKeyBuilder.WriteByte(0)
	}
	return metricKeyBuilder.String()
}
