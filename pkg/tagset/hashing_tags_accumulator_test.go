// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/filterlist"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/murmur3"
)

func TestNewHashingTagsAccumulator(t *testing.T) {
	tb := NewHashingTagsAccumulator()
	assert.NotNil(t, tb)
	assert.Equal(t, []string{}, tb.data)
}

func TestNewHashingTagsAccumulatorWithTags(t *testing.T) {
	test := []string{"a", "b", "c"}
	tb := NewHashingTagsAccumulatorWithTags(test)
	assert.NotNil(t, tb)
	assert.Equal(t, test, tb.data)
}

func TestHashingTagsAccumulatorAppend(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Append("d")
	assert.Equal(t, []string{"a", "b", "c", "d"}, tb.data)
}

func TestHashingTagsAccumulatorReset(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Reset()
	assert.Equal(t, []string{}, tb.data)
}

func TestHashingTagsAccumulatorGet(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	internalData := tb.Get()
	assert.Equal(t, []string{"a", "b", "c"}, internalData)

	// check that the internal buffer was indeed returned and not a copy
	internalData[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, internalData)
	assert.Equal(t, []string{"test", "b", "c"}, tb.data)
}

func TestHashingTagsAccumulatorCopy(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	tagsCopy := tb.Copy()
	assert.Equal(t, []string{"a", "b", "c"}, tagsCopy)
	assert.NotSame(t, &tagsCopy, &tb.data)

	tagsCopy[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, tagsCopy)
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)
}

func TestRemoveSorted(t *testing.T) {
	l := NewHashingTagsAccumulator()
	r := NewHashingTagsAccumulator()
	l.Append("a", "b", "c", "d")
	l.SortUniq()
	r.Append("a", "b", "e", "f")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, l.Get())
	assert.ElementsMatch(t, []string{"e", "f"}, r.Get())

	r.Reset()
	r.Append("c", "d", "e", "f")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"e", "f"}, r.Get())

	r.Reset()
	r.Append("a", "aa", "ab", "b")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"aa", "ab"}, r.Get())

	r.Reset()
	r.Append("A", "B", "a", "d")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"A", "B"}, r.Get())

	r.Reset()
	r.Append("A", "a", "b", "e")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"A", "e"}, r.Get())
}

func testTagsMatchHash(t *testing.T, acc *HashingTagsAccumulator) {
	assert.Equal(t, len(acc.data), len(acc.hash))
	for idx, tag := range acc.data {
		assert.Equal(t, murmur3.StringSum64(tag), acc.hash[idx])
	}
}

func TestStripTags(t *testing.T) {
	tests := []struct {
		name         string
		matcherTags  map[string]filterlist.MetricTagList
		lookupName   string
		inputTags    []string
		expectedTags []string
	}{
		{
			name: "no tags config for name (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric2",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"env:prod", "host:server1", "version:1.0"},
		},
		{
			name: "strip single tag with value (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"host:server1", "version:1.0"},
		},
		{
			name: "strip multiple tags (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"version:1.0", "region:us"},
		},
		{
			name: "strip all tags (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host", "region", "version"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{},
		},
		{
			name: "no matching tags to strip (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"foo", "bar"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "empty input tags (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{},
			expectedTags: []string{},
		},
		{
			name: "tags without values (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env", "host:server1", "version"},
			expectedTags: []string{"version"},
		},
		{
			name: "invalid tag starting with colon (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "invalid"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", ":invalid", "host:server1"},
			expectedTags: []string{":invalid", "host:server1"},
		},
		{
			name: "partial tag name match should not strip (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"environment:prod", "env_var:test", "host:server1"},
			expectedTags: []string{"environment:prod", "env_var:test", "host:server1"},
		},
		{
			name: "tag name with empty value (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:", "host:server1"},
			expectedTags: []string{"host:server1"},
		},
		{
			name:         "nil matcher tags map",
			matcherTags:  nil,
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "empty tags to strip list (deny list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := filterlist.NewTagMatcher(tt.matcherTags)

			tagMatcher, ok := matcher.ShouldStripTags(tt.lookupName)
			_, tagConfigured := tt.matcherTags[tt.lookupName]
			assert.Equal(t, tagConfigured, ok)

			if ok {
				// Filter the tags
				acc := NewHashingTagsAccumulatorWithTags(tt.inputTags)
				acc.FilterTags(tagMatcher)

				resultTags := acc.Get()
				assert.Equal(t, tt.expectedTags, resultTags)
				testTagsMatchHash(t, acc)
			}
		})
	}
}

func TestStripTagsAllowList(t *testing.T) {
	tests := []struct {
		name         string
		matcherTags  map[string]filterlist.MetricTagList
		lookupName   string
		inputTags    []string
		expectedTags []string
	}{
		{
			name: "keep single tag (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "keep multiple tags (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "keep all tags (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host", "region", "version"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"env:prod", "host:server1", "version:1.0", "region:us"},
		},
		{
			name: "no matching tags in allow list",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"foo", "bar"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{},
		},
		{
			name: "empty input tags (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{},
			expectedTags: []string{},
		},
		{
			name: "tags without values (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env", "host:server1", "version"},
			expectedTags: []string{"env", "host:server1"},
		},
		{
			name: "invalid tag starting with colon (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env", "invalid"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", ":invalid", "host:server1"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "partial tag name match should not keep (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"environment:prod", "env_var:test", "env:prod", "host:server1"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "tag name with empty value (allow list)",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:", "host:server1"},
			expectedTags: []string{"env:"},
		},
		{
			name: "empty allow list strips all tags",
			matcherTags: map[string]filterlist.MetricTagList{"metric1": {
				Tags:    []string{},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := filterlist.NewTagMatcher(tt.matcherTags)

			tagMatcher, ok := matcher.ShouldStripTags(tt.lookupName)
			_, tagConfigured := tt.matcherTags[tt.lookupName]
			assert.Equal(t, tagConfigured, ok)

			if ok {
				// Filter the tags
				acc := NewHashingTagsAccumulatorWithTags(tt.inputTags)
				acc.FilterTags(tagMatcher)

				resultTags := acc.Get()
				assert.Equal(t, tt.expectedTags, resultTags)
				testTagsMatchHash(t, acc)
			}
		})
	}
}
