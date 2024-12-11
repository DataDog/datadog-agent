// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"github.com/DataDog/datadog-agent/pkg/util/sort"
)

// HashlessTagsAccumulator allows to build a slice of tags, in a context where the hashes for
// those tags are not useful.
//
// This type implements TagsAccumulator.
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
func (h *HashlessTagsAccumulator) Append(tags ...string) {
	h.data = append(h.data, tags...)
}

// AppendHashlessAccumulator appends tags from the given accumulator
func (h *HashlessTagsAccumulator) AppendHashlessAccumulator(src *HashlessTagsAccumulator) {
	h.data = append(h.data, src.data...)
}

// AppendHashed appends tags and corresponding hashes to the builder
func (h *HashlessTagsAccumulator) AppendHashed(src HashedTags) {
	h.data = append(h.data, src.data...)
}

// Get returns the internal slice
func (h *HashlessTagsAccumulator) Get() []string {
	return h.data
}

// Copy returns a new slice with the copy of the tags
func (h *HashlessTagsAccumulator) Copy() []string {
	return append(make([]string, 0, len(h.data)), h.data...)
}

// SortUniq sorts and remove duplicate in place
func (h *HashlessTagsAccumulator) SortUniq() {
	h.data = sort.UniqInPlace(h.data)
}

// Reset resets the size of the builder to 0 without discarding the internal
// buffer
func (h *HashlessTagsAccumulator) Reset() {
	// we keep the internal buffer but reset size
	h.data = h.data[0:0]
}
