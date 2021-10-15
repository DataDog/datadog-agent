// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"sort"

	"github.com/twmb/murmur3"
)

// TagsBuilder allows to build a slice of tags to generate the context while
// reusing the same internal slice.
type TagsBuilder struct {
	data []string
}

// NewTagsBuilder returns a new empty TagsBuilder.
func NewTagsBuilder() *TagsBuilder {
	return &TagsBuilder{
		// Slice will grow as more tags are added to it. 128 tags
		// should be enough for most metrics.
		data: make([]string, 0, 128),
	}
}

// NewTagsBuilderFromSlice return a new TagsBuilder with the input slice for
// it's internal buffer.
func NewTagsBuilderFromSlice(data []string) *TagsBuilder {
	return &TagsBuilder{
		data: data,
	}
}

// Append appends tags to the builder
func (tb *TagsBuilder) Append(tags ...string) {
	tb.data = append(tb.data, tags...)
}

// AppendHashed appends tags and corresponding hashes to the builder
func (tb *TagsBuilder) AppendHashed(src HashedTags) {
	tb.data = append(tb.data, src.data...)
}

// Get returns the internal slice
func (tb *TagsBuilder) Get() []string {
	return tb.data
}

// SortUniq sorts and remove duplicate in place
func (tb *TagsBuilder) SortUniq() {
	tb.data = SortUniqInPlace(tb.data)
}

// Reset resets the size of the builder to 0 without discaring the internal
// buffer
func (tb *TagsBuilder) Reset() {
	// we keep the internal buffer but reset size
	tb.data = tb.data[0:0]
}

// hashedTags is the base type for HashingTagsBuilder and HashedTags
type hashedTags struct {
	data []string
	hash []uint64
}

func newHashedTagsWithCapacity(cap int) hashedTags {
	return hashedTags{
		data: make([]string, 0, cap),
		hash: make([]uint64, 0, cap),
	}
}

func newHashedTagsFromSlice(tags []string) hashedTags {
	hash := make([]uint64, 0, len(tags))
	for _, t := range tags {
		hash = append(hash, murmur3.StringSum64(t))
	}
	return hashedTags{
		data: tags,
		hash: hash,
	}
}

// Copy returns a new slice with the copy of the tags
func (h hashedTags) Copy() []string {
	return append(make([]string, 0, len(h.data)), h.data...)
}

// Len returns number of tags
func (h hashedTags) Len() int {
	return len(h.data)
}

// Get returns the internal slice
func (h hashedTags) Get() []string {
	return h.data
}

// Hashes returns the internal slice of tag hashes
func (h hashedTags) Hashes() []uint64 {
	return h.hash
}

// dup returns a full copy of hashedTags
func (h hashedTags) dup() hashedTags {
	return hashedTags{
		data: append([]string{}, h.data...),
		hash: append([]uint64{}, h.hash...),
	}
}

// HashingTagsBuilder allows to build a slice of tags to generate the context while
// reusing the same internal slice.
type HashingTagsBuilder struct {
	hashedTags
}

// NewHashingTagsBuilder returns a new empty TagsBuilder.
func NewHashingTagsBuilder() *HashingTagsBuilder {
	return &HashingTagsBuilder{
		hashedTags: newHashedTagsWithCapacity(128),
	}
}

// NewHashingTagsBuilderWithTags return a new HashingTagsBuilder, initialized with tags.
func NewHashingTagsBuilderWithTags(tags []string) *HashingTagsBuilder {
	tb := NewHashingTagsBuilder()
	tb.Append(tags...)
	return tb
}

// Append appends tags to the builder
func (tb *HashingTagsBuilder) Append(tags ...string) {
	for _, t := range tags {
		tb.data = append(tb.data, t)
		tb.hash = append(tb.hash, murmur3.StringSum64(t))
	}
}

// AppendHashed appends tags and corresponding hashes to the builder
func (tb *HashingTagsBuilder) AppendHashed(src HashedTags) {
	tb.data = append(tb.data, src.data...)
	tb.hash = append(tb.hash, src.hash...)
}

// SortUniq sorts and remove duplicate in place
func (tb *HashingTagsBuilder) SortUniq() {
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

// Reset resets the size of the builder to 0 without discaring the internal
// buffer
func (tb *HashingTagsBuilder) Reset() {
	// we keep the internal buffer but reset size
	tb.data = tb.data[0:0]
	tb.hash = tb.hash[0:0]
}

// Truncate retains first n tags in the buffer without discarding the internal buffer
func (tb *HashingTagsBuilder) Truncate(len int) {
	tb.data = tb.data[0:len]
	tb.hash = tb.hash[0:len]
}

// Less implements sort.Interface.Less
func (tb *HashingTagsBuilder) Less(i, j int) bool {
	// FIXME(vickenty): could sort using hashes, which is faster, but a lot of tests check for order.
	return tb.data[i] < tb.data[j]
}

// Swap implements sort.Interface.Swap
func (tb *HashingTagsBuilder) Swap(i, j int) {
	tb.hash[i], tb.hash[j] = tb.hash[j], tb.hash[i]
	tb.data[i], tb.data[j] = tb.data[j], tb.data[i]
}

// Dup returns a complete copy of HashingTagsBuilder
func (tb *HashingTagsBuilder) Dup() *HashingTagsBuilder {
	return &HashingTagsBuilder{tb.dup()}
}

// HashedTags is an immutable slice of pre-hashed tags.
type HashedTags struct {
	hashedTags
}

// NewHashedTagsFromSlice creates a new instance, re-using tags as the internal slice.
func NewHashedTagsFromSlice(tags []string) HashedTags {
	return HashedTags{newHashedTagsFromSlice(tags)}
}

// Slice returns a shared sub-slice of tags from t.
func (t HashedTags) Slice(i, j int) HashedTags {
	return HashedTags{
		hashedTags{
			data: t.data[i:j],
			hash: t.hash[i:j],
		},
	}
}
