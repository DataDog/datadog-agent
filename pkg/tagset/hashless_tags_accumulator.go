// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "github.com/DataDog/datadog-agent/pkg/util"

// HashlessTagsAccumulator allows to build a slice of tags, in a context where the hashes for
// those tags are not useful.
//
// This type implements TagAccumulator.
type HashlessTagsAccumulator struct {
	data []string
}

// NewHashlessTagsAccumulator returns a new empty HashlessTagsAccumulator.
func NewHashlessTagsAccumulator() *HashlessTagsAccumulator {
	return &HashlessTagsAccumulator{
		// Slice will grow as more tags are added to it. 128 tags
		// should be enough for most metrics.
		data: make([]string, 0, 128),
	}
}

// NewHashlessTagsAccumulatorFromSlice return a new HashlessTagsAccumulator with the
// input slice for it's internal buffer.
func NewHashlessTagsAccumulatorFromSlice(data []string) *HashlessTagsAccumulator {
	return &HashlessTagsAccumulator{
		data: data,
	}
}

// Append appends tags to the builder
func (tb *HashlessTagsAccumulator) Append(tags ...string) {
	tb.data = append(tb.data, tags...)
}

// AppendHashed appends tags and corresponding hashes to the builder
func (tb *HashlessTagsAccumulator) AppendHashed(src HashedTags) {
	tb.data = append(tb.data, src.data...)
}

// Get returns the internal slice
func (tb *HashlessTagsAccumulator) Get() []string {
	return tb.data
}

// SortUniq sorts and remove duplicate in place
func (tb *HashlessTagsAccumulator) SortUniq() {
	tb.data = util.SortUniqInPlace(tb.data)
}

// Reset resets the size of the builder to 0 without discaring the internal
// buffer
func (tb *HashlessTagsAccumulator) Reset() {
	// we keep the internal buffer but reset size
	tb.data = tb.data[0:0]
}
