// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"sort"
)

// HashingTagsAccumulator allows to build a slice of tags, including the hashes
// of each tag.
//
// This type implements TagsAccumulator.
type HashingTagsAccumulator struct {
	hashedTags
}

// RetainFunc filters tags in place using an O(n) sort-merge against the given
// HashedMetricTagList. Both the accumulator (sorted by NameHash) and the filter's
// Tags slice are sorted as a side effect. Returns the number of tags removed.
func (h *HashingTagsAccumulator) RetainFunc(filter *HashedMetricTagList) int {
	oldLen := len(h.tags)
	if oldLen == 0 {
		return 0
	}

	// Sort our tags by NameHash for the O(n) merge.
	h.sortByName()

	write := 0
	j := 0 // index into filter.Tags
	for i := 0; i < len(h.tags); i++ {
		nameHash := h.tags[i].NameHash

		// Advance j until filter.Tags[j] >= nameHash.
		for j < len(filter.Tags) && filter.Tags[j] < nameHash {
			j++
		}

		matched := j < len(filter.Tags) && filter.Tags[j] == nameHash

		// Keep the tag when:
		//   Exclude: matched == false  (name is NOT in the remove list)
		//   Include: matched == true   (name IS in the keep list)
		if matched != bool(filter.Action) {
			h.tags[write] = h.tags[i]
			write++
		}
	}

	h.tags = h.tags[0:write]
	return oldLen - write
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
func (h *HashingTagsAccumulator) Append(tags ...string) {
	for _, t := range tags {
		h.tags = append(h.tags, newTagHash(t))
	}
}

// AppendHashed appends tags and corresponding hashes to the builder
func (h *HashingTagsAccumulator) AppendHashed(src HashedTags) {
	h.tags = append(h.tags, src.tags...)
}

// SortUniq sorts and remove duplicate in place
func (h *HashingTagsAccumulator) SortUniq() {
	if h.Len() < 2 {
		return
	}

	sort.Sort(h)

	j := 0
	for i := 1; i < len(h.tags); i++ {
		if h.tags[i].Tag == h.tags[j].Tag {
			continue
		}
		j++
		h.tags[j] = h.tags[i]
	}

	h.Truncate(j + 1)
}

// Get returns the tag strings as a slice
func (h *HashingTagsAccumulator) Get() []string {
	result := make([]string, len(h.tags))
	for i, th := range h.tags {
		result[i] = th.Tag
	}
	return result
}

// Hashes returns the tag hashes as a slice
func (h *HashingTagsAccumulator) Hashes() []uint64 {
	result := make([]uint64, len(h.tags))
	for i, th := range h.tags {
		result[i] = th.Hash
	}
	return result
}

// Reset resets the size of the builder to 0 without discaring the internal
// buffer
func (h *HashingTagsAccumulator) Reset() {
	// we keep the internal buffer but reset size
	h.tags = h.tags[0:0]
}

// Truncate retains first n tags in the buffer without discarding the internal buffer
func (h *HashingTagsAccumulator) Truncate(len int) {
	h.tags = h.tags[0:len]
}

// Less implements sort.Interface.Less
func (h *HashingTagsAccumulator) Less(i, j int) bool {
	return h.tags[i].Tag < h.tags[j].Tag
}

// Swap implements sort.Interface.Swap
func (h *HashingTagsAccumulator) Swap(i, j int) {
	h.tags[i], h.tags[j] = h.tags[j], h.tags[i]
}

// Dup returns a complete copy of HashingTagsAccumulator
func (h *HashingTagsAccumulator) Dup() *HashingTagsAccumulator {
	return &HashingTagsAccumulator{h.dup()}
}

// Hash returns combined hashes of all tags in the accumulator.
//
// Does not account for possibility of duplicates. Must be called after a call to Dedup2 or SortUniq
// first.
func (h *HashingTagsAccumulator) Hash() uint64 {
	var hash uint64
	for _, th := range h.tags {
		hash ^= th.Hash
	}
	return hash
}

// removeSorted removes tags contained in l from r. Both accumulators must be SortUniq first.
//
// h is not sorted after this function. Does not modify o.
func (h *HashingTagsAccumulator) removeSorted(o *HashingTagsAccumulator) {
	// A sentinel string and NOT matching hash (an impossible combination outside this function)
	const holeTag = ""
	const holeHash = 42

	hlen, olen := len(h.tags), len(o.tags)

	for i, j := 0, 0; i < hlen && j < olen; {
		switch {
		case h.tags[i].Tag == o.tags[j].Tag:
			h.tags[i] = TagHash{Tag: holeTag, Hash: holeHash}
			i++
		case h.tags[i].Tag < o.tags[j].Tag:
			i++
		case h.tags[i].Tag > o.tags[j].Tag:
			j++
		}
	}

	for i := 0; i < hlen; {
		if h.tags[i].Tag == holeTag && h.tags[i].Hash == holeHash {
			hlen--
			h.tags[i] = h.tags[hlen]
		} else {
			i++
		}
	}

	h.Truncate(hlen)
}
