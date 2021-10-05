package tagset

import (
	"sort"

	"github.com/twmb/murmur3"
)

// HashingTagsBuilder allows to build a slice of tags, including the hashes
// of each tag.
//
// This type implements TagAccumulator.
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
