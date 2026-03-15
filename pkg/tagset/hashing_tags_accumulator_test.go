// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/murmur3"
)

func TestNewHashingTagsAccumulator(t *testing.T) {
	tb := NewHashingTagsAccumulator()
	assert.NotNil(t, tb)
	assert.Equal(t, []TagHash{}, tb.tags)
}

func TestNewHashingTagsAccumulatorWithTags(t *testing.T) {
	test := []string{"a", "b", "c"}
	tb := NewHashingTagsAccumulatorWithTags(test)
	assert.NotNil(t, tb)
	assert.Equal(t, []string{"a", "b", "c"}, tb.Get())
}

func TestHashingTagsAccumulatorAppend(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.Get())

	tb.Append("d")
	assert.Equal(t, []string{"a", "b", "c", "d"}, tb.Get())
}

func TestHashingTagsAccumulatorReset(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.Get())

	tb.Reset()
	assert.Equal(t, []TagHash{}, tb.tags)
}

func TestHashingTagsAccumulatorGet(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	result := tb.Get()
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestHashingTagsAccumulatorCopy(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	tagsCopy := tb.Copy()
	assert.Equal(t, []string{"a", "b", "c"}, tagsCopy)

	tagsCopy[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, tagsCopy)
	assert.Equal(t, []string{"a", "b", "c"}, tb.Get())
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
	for _, th := range acc.tags {
		assert.Equal(t, murmur3.StringSum64(th.Tag), th.Hash)
	}
}

func TestFilterTags(t *testing.T) {
	tests := []struct {
		name         string
		inputTags    []string
		filter       *HashedMetricTagList
		expectedTags []string
	}{
		{
			name:         "include with empty list removes all tags",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			filter:       &HashedMetricTagList{Tags: []uint64{}, Action: Include},
			expectedTags: []string{},
		},
		{
			name:         "exclude with empty list keeps all tags",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			filter:       &HashedMetricTagList{Tags: []uint64{}, Action: Exclude},
			expectedTags: []string{"env:prod", "host:server1", "version:1.0"},
		},
		{
			name:      "include keeps only matching tag names",
			inputTags: []string{"env:prod", "host:server1", "version:1.0", "region:us-east"},
			filter: &HashedMetricTagList{
				Tags:   []uint64{murmur3.StringSum64("env"), murmur3.StringSum64("version")},
				Action: Include,
			},
			expectedTags: []string{"env:prod", "version:1.0"},
		},
		{
			name:      "exclude removes matching tag names",
			inputTags: []string{"env:prod", "host:server1", "version:1.0", "region:us-east"},
			filter: &HashedMetricTagList{
				Tags:   []uint64{murmur3.StringSum64("host"), murmur3.StringSum64("region")},
				Action: Exclude,
			},
			expectedTags: []string{"env:prod", "version:1.0"},
		},
		{
			name:         "no tags to filter",
			inputTags:    []string{},
			filter:       &HashedMetricTagList{Tags: []uint64{}, Action: Exclude},
			expectedTags: []string{},
		},
		{
			name:      "multiple tags with same name are all kept or removed together",
			inputTags: []string{"env:prod", "env:staging", "host:server1"},
			filter: &HashedMetricTagList{
				Tags:   []uint64{murmur3.StringSum64("env")},
				Action: Exclude,
			},
			expectedTags: []string{"host:server1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := NewHashingTagsAccumulatorWithTags(tt.inputTags)
			removed := acc.RetainFunc(tt.filter)

			assert.ElementsMatch(t, tt.expectedTags, acc.Get())
			assert.Equal(t, len(tt.inputTags)-len(tt.expectedTags), removed)
			testTagsMatchHash(t, acc)
		})
	}
}
