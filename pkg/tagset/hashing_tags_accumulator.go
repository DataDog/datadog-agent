// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"sort"

	"github.com/twmb/murmur3"
)

// HashingTagsAccumulator allows to build a slice of tags, including the hashes
// of each tag.
//
// This type implements TagAccumulator.
type HashingTagsAccumulator struct {
	hashedTags
}

// NewHashingTagsAccumulator returns a new empty HashingTagsAccumulator
func NewHashingTagsAccumulator() *HashingTagsAccumulator {
	return &HashingTagsAccumulator{
		hashedTags: newHashedTagsWithCapacity(128),
	}
}

// NewHashingTagsAccumulatorWithTags return a new HashingTagsAccumulator, initialized with tags.
func NewHashingTagsAccumulatorWithTags(tags []string) *HashingTagsAccumulator {
	tb := NewHashingTagsAccumulator()
	tb.Append(tags...)
	return tb
}

// Append appends tags to the builder
func (tb *HashingTagsAccumulator) Append(tags ...string) {
	for _, t := range tags {
		tb.data = append(tb.data, t)
		tb.hash = append(tb.hash, murmur3.StringSum64(t))
	}
}

// AppendHashed appends tags and corresponding hashes to the builder
func (tb *HashingTagsAccumulator) AppendHashed(src HashedTags) {
	tb.data = append(tb.data, src.data...)
	tb.hash = append(tb.hash, src.hash...)
}

// SortUniq sorts and remove duplicate in place
func (tb *HashingTagsAccumulator) SortUniq() {
	if tb.Len() < 2 {
		return
	}

	sort.Sort(tb)

	j := 0
	for i := 1; i < len(tb.data); i++ {
		if tb.hash[i] == tb.hash[j] && tb.data[i] == tb.data[j] {
			continue
		}
		j++
		tb.data[j] = tb.data[i]
		tb.hash[j] = tb.hash[i]
	}

	tb.Truncate(j + 1)
}

// Get returns the internal slice
func (tb *HashingTagsAccumulator) Get() []string {
	return tb.data
}

// Hashes returns the internal slice of tag hashes
func (tb *HashingTagsAccumulator) Hashes() []uint64 {
	return tb.hash
}

// Reset resets the size of the builder to 0 without discaring the internal
// buffer
func (tb *HashingTagsAccumulator) Reset() {
	// we keep the internal buffer but reset size
	tb.data = tb.data[0:0]
	tb.hash = tb.hash[0:0]
}

// Truncate retains first n tags in the buffer without discarding the internal buffer
func (tb *HashingTagsAccumulator) Truncate(len int) {
	tb.data = tb.data[0:len]
	tb.hash = tb.hash[0:len]
}

// Less implements sort.Interface.Less
func (tb *HashingTagsAccumulator) Less(i, j int) bool {
	// FIXME(vickenty): could sort using hashes, which is faster, but a lot of tests check for order.
	return tb.data[i] < tb.data[j]
}

// Swap implements sort.Interface.Swap
func (tb *HashingTagsAccumulator) Swap(i, j int) {
	tb.hash[i], tb.hash[j] = tb.hash[j], tb.hash[i]
	tb.data[i], tb.data[j] = tb.data[j], tb.data[i]
}

// Dup returns a complete copy of HashingTagsAccumulator
func (tb *HashingTagsAccumulator) Dup() *HashingTagsAccumulator {
	return &HashingTagsAccumulator{tb.dup()}
}
