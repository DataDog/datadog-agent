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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestWithAttributeMap(t *testing.T) {
	attributes := pcommon.NewMap()
	attributes.FromRaw(map[string]interface{}{
		"key1": "val1",
		"key2": "val2",
		"key3": "",
	})

	dims := Dimensions{}
	assert.ElementsMatch(t,
		dims.WithAttributeMap(attributes).tags,
		[...]string{"key1:val1", "key2:val2", "key3:n/a"},
	)
}

func TestMetricDimensionsString(t *testing.T) {
	getKey := func(name string, tags []string, host string) string {
		dims := Dimensions{name: name, tags: tags, host: host}
		return dims.String()
	}
	metricName := "metric.name"
	hostOne := "host-one"
	hostTwo := "host-two"
	noTags := getKey(metricName, []string{}, hostOne)
	someTags := getKey(metricName, []string{"key1:val1", "key2:val2"}, hostOne)
	sameTags := getKey(metricName, []string{"key2:val2", "key1:val1"}, hostOne)
	diffTags := getKey(metricName, []string{"key3:val3"}, hostOne)
	diffHost := getKey(metricName, []string{"key1:val1", "key2:val2"}, hostTwo)

	assert.NotEqual(t, noTags, someTags)
	assert.NotEqual(t, someTags, diffTags)
	assert.Equal(t, someTags, sameTags)
	assert.NotEqual(t, someTags, diffHost)
}

func TestMetricDimensionsStringNoTagsChange(t *testing.T) {
	// The original metricDimensionsToMapKey had an issue where:
	// - if the capacity of the tags array passed to it was higher than its length
	// - and the metric name is earlier (in alphabetical order) than one of the tags
	// then the original tag array would be modified (without a reallocation, since there is enough capacity),
	// and would contain a tag labeled as the metric name, while the final tag (in alphabetical order)
	// would get left out.
	// This test checks that this doesn't happen anymore.

	originalTags := make([]string, 2, 3)
	originalTags[0] = "key1:val1"
	originalTags[1] = "key2:val2"

	dims := Dimensions{
		name: "a.metric.name",
		tags: originalTags,
	}

	_ = dims.String()
	assert.Equal(t, []string{"key1:val1", "key2:val2"}, originalTags)

}

func TestWithAttributeMapEmptyLabels(t *testing.T) {
	// WithAttributeMap with an empty map should return the same pointer (early-return path).
	dims := &Dimensions{name: "metric", tags: []string{"k:v"}, host: "h"}
	result := dims.WithAttributeMap(pcommon.NewMap())
	assert.Same(t, dims, result)
}

func TestMetricDimensionsStringTagShorterThanPrefix(t *testing.T) {
	// Tags that are lexicographically shorter than a fixed prefix ("host:", "name:", "originID:")
	// exercise the len(t) < len(f.prefix) branch in the merge loop.
	//
	// "h" < "host:" and "n" < "name:", so both sort before their respective fixed fields.
	dims := Dimensions{name: "m", tags: []string{"h", "n"}, host: "myhost"}
	key := dims.String()
	// Must be stable across repeated calls.
	assert.Equal(t, dims.String(), key)
	// Must differ from a key with no tags.
	assert.NotEqual(t, (&Dimensions{name: "m", host: "myhost"}).String(), key)
}

func TestMetricDimensionsStringTagEqualToFixedPrefix(t *testing.T) {
	// A tag exactly equal to a fixed prefix string (e.g. "host:") exercises the
	// `t == f.prefix[:len(t)]` equality branch; the tag should sort before the fixed field.
	dims := Dimensions{name: "m", tags: []string{"host:"}, host: "myhost"}
	key := dims.String()
	assert.Equal(t, dims.String(), key)
	// Verify the tag is actually included and not swallowed.
	dimsNoTag := &Dimensions{name: "m", host: "myhost"}
	assert.NotEqual(t, dimsNoTag.String(), key)
}

func TestMetricDimensionsStringTagsAfterAllFixed(t *testing.T) {
	// Tags that sort after all three fixed fields (e.g. starting with 'z') exercise
	// the trailing-flush loop after the fixed-field merge loop.
	dims := Dimensions{name: "m", tags: []string{"z:last"}, host: "h"}
	key := dims.String()
	assert.Equal(t, dims.String(), key)
	dimsNoTag := &Dimensions{name: "m", host: "h"}
	assert.NotEqual(t, dimsNoTag.String(), key)
}

func TestMetricDimensionsStringDoesNotMutateOriginalTags(t *testing.T) {
	// Regression: String() must not reorder or append to the original tags slice
	// even when the slice has spare capacity.
	tags := make([]string, 2, 5)
	tags[0] = "z:last"
	tags[1] = "a:first"
	dims := Dimensions{name: "metric", tags: tags, host: "host"}
	_ = dims.String()
	assert.Equal(t, []string{"z:last", "a:first"}, tags, "String() must not mutate the original tags slice")
}

var testDims = Dimensions{
	name: "test.metric",
	tags: []string{"key:val"},
	host: "host",
}

func TestWithSuffix(t *testing.T) {
	dimsSuf1 := testDims.WithSuffix("suffixOne")
	dimsSuf2 := testDims.WithSuffix("suffixTwo")

	assert.Equal(t, "test.metric", testDims.name)
	assert.Equal(t, "test.metric.suffixOne", dimsSuf1.name)
	assert.Equal(t, "test.metric.suffixTwo", dimsSuf2.name)
}

func TestAddTags(t *testing.T) {
	dimsWithTags := testDims.AddTags("key1:val1", "key2:val2")
	assert.ElementsMatch(t, []string{"key:val", "key1:val1", "key2:val2"}, dimsWithTags.tags)
	assert.ElementsMatch(t, []string{"key:val"}, testDims.tags)
}

func TestAllFieldsAreCopied(t *testing.T) {
	dims := &Dimensions{
		name:     "example.name",
		host:     "hostname",
		tags:     []string{"tagOne:a", "tagTwo:b"},
		originID: "origin_id",
	}

	attributes := pcommon.NewMap()
	attributes.FromRaw(map[string]interface{}{
		"tagFour": "d",
	})
	newDims := dims.
		AddTags("tagThree:c").
		WithSuffix("suffix").
		WithAttributeMap(attributes)

	assert.Equal(t, "example.name.suffix", newDims.Name())
	assert.Equal(t, "hostname", newDims.Host())
	assert.ElementsMatch(t, []string{"tagOne:a", "tagTwo:b", "tagThree:c", "tagFour:d"}, newDims.Tags())
	assert.Equal(t, "origin_id", newDims.OriginID())
}
