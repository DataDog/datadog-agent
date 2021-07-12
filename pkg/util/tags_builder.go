// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

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
		data: make([]string, 0, 128)}
}

// NewTagsBuilderFromSlice return a new TagsBuilder with the input slice for
// it's internal buffer.
func NewTagsBuilderFromSlice(tags []string) *TagsBuilder {
	return &TagsBuilder{
		data: tags,
	}
}

// Append appends tags to the builder
func (tb *TagsBuilder) Append(tags ...string) {
	tb.data = append(tb.data, tags...)
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

// Get returns the internal slice
func (tb *TagsBuilder) Get() []string {
	return tb.data
}

// Copy makes a copy of the internal slice
func (tb *TagsBuilder) Copy() []string {
	return append(make([]string, 0, len(tb.data)), tb.data...)
}
