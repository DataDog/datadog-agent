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

// Get returns the internal slice.
//
// NOTE: this returns a mutable reference to data in this immutable data structure.
// It is still used by pkg/tagger/tagstore, but new uses should not be added.
func (t HashedTags) Get() []string {
	return t.data
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
