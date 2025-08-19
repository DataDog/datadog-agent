// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"unique"
)

// HashedTags is an immutable slice of pre-hashed tags.
type HashedTags struct {
	hashedTags
}

// NewHashedTagsFromSlice creates a new instance, re-using tags as the internal slice.
func NewHashedTagsFromSlice(tags []unique.Handle[string]) HashedTags {
	return HashedTags{newHashedTagsFromSlice(tags)}
}

// NewHashedTagsFromStringSlice creates a new instance, re-using tags as the internal slice.
func NewHashedTagsFromStringSlice(tags []string) HashedTags {
	return HashedTags{newHashedTagsFromStringSlice(tags)}
}

// Get returns a list of tags as a string slice.
func (h HashedTags) Get() []string {
	if h.data == nil {
		return nil
	}
	s := make([]string, 0, len(h.data))
	for _, t := range h.data {
		s = append(s, t.Value())
	}
	return s
}

// Slice returns a shared sub-slice of tags from t.
func (h HashedTags) Slice(i, j int) HashedTags {
	return HashedTags{
		hashedTags{
			data: h.data[i:j],
			hash: h.hash[i:j],
		},
	}
}
