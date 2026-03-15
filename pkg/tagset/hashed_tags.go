// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

// HashedTags is an immutable slice of pre-hashed tags.
type HashedTags struct {
	hashedTags
}

// NewHashedTagsFromSlice creates a new instance, re-using tags as the internal slice.
func NewHashedTagsFromSlice(tags []string) HashedTags {
	return HashedTags{newHashedTagsFromSlice(tags)}
}

// Get returns the tag strings as a slice.
func (h HashedTags) Get() []string {
	result := make([]string, len(h.tags))
	for i, th := range h.tags {
		result[i] = th.Tag
	}
	return result
}

// Slice returns a shared sub-slice of tags from t.
func (h HashedTags) Slice(i, j int) HashedTags {
	return HashedTags{
		hashedTags{
			tags: h.tags[i:j],
		},
	}
}
