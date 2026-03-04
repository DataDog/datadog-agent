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

const (
	dimensionSeparator = string(byte(0))
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
// It builds the new tags slice in a single allocation, avoiding the intermediate slice from getTags.
func (d *Dimensions) WithAttributeMap(labels pcommon.Map) *Dimensions {
	labelsLen := labels.Len()
	if labelsLen == 0 {
		return d
	}
	// Match AddTags ordering: attribute tags come first, then existing d.tags.
	newTags := make([]string, 0, labelsLen+len(d.tags))
	labels.Range(func(key string, value pcommon.Value) bool {
		v := value.AsString()
		newTags = append(newTags, utils.FormatKeyValueTag(key, v))
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
	return &Dimensions{
		name:                d.name + "." + suffix,
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
	// Sort only the tags slice (copy to avoid mutating d.tags).
	sortedTags := make([]string, len(d.tags))
	copy(sortedTags, d.tags)
	sort.Strings(sortedTags)

	// The three fixed fields, in sorted order by their full "prefix:value" representation.
	// "host:" < "name:" < "originID:" (lexicographic), so insertion order is fixed.
	const (
		prefixHost     = "host:"
		prefixName     = "name:"
		prefixOriginID = "originID:"
	)
	fixed := [3]struct{ prefix, value string }{
		{prefixHost, d.host},
		{prefixName, d.name},
		{prefixOriginID, d.originID},
	}

	// Pre-compute total length for a single Builder allocation.
	totalLen := len(dimensionSeparator) * (len(sortedTags) + 3)
	for _, t := range sortedTags {
		totalLen += len(t)
	}
	for _, f := range fixed {
		totalLen += len(f.prefix) + len(f.value)
	}

	// Merge sortedTags and fixed entries in sorted order, writing directly to the builder.
	// This avoids allocating the "prefix:value" concatenated strings.
	var b strings.Builder
	b.Grow(totalLen)
	ti := 0 // index into sortedTags
	for _, f := range fixed {
		// Flush all tags that sort before this fixed entry.
		for ti < len(sortedTags) {
			// Compare tag to "prefix" + value. We avoid building a full string:
			// a tag t sorts before fixed entry f if t < f.prefix+f.value.
			t := sortedTags[ti]
			if len(t) < len(f.prefix) {
				if t < f.prefix[:len(t)] || (t == f.prefix[:len(t)]) {
					b.WriteString(t)
					b.WriteString(dimensionSeparator)
					ti++
					continue
				}
			} else {
				if t[:len(f.prefix)] < f.prefix || (t[:len(f.prefix)] == f.prefix && t[len(f.prefix):] < f.value) {
					b.WriteString(t)
					b.WriteString(dimensionSeparator)
					ti++
					continue
				}
			}
			break
		}
		b.WriteString(f.prefix)
		b.WriteString(f.value)
		b.WriteString(dimensionSeparator)
	}
	// Flush any remaining tags.
	for ; ti < len(sortedTags); ti++ {
		b.WriteString(sortedTags[ti])
		b.WriteString(dimensionSeparator)
	}
	return b.String()
}
