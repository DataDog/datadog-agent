// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"strings"

	"github.com/zeebo/xxh3"
)

// HashedTags is an immutable slice of pre-hashed tags.
type HashedTags struct {
	hashedTags
	// nameHash holds the xxh3 hash of the tag name (the portion before ':') for
	// each tag. It is pre-computed once at construction time to allow fast,
	// allocation-free tag filtering in the hot path.
	nameHash []uint64
}

// NewHashedTagsFromSlice creates a new instance, re-using tags as the internal slice.
func NewHashedTagsFromSlice(tags []string) HashedTags {
	nameHash := make([]uint64, len(tags))
	for i, tag := range tags {
		pos := strings.IndexByte(tag, ':')
		if pos < 0 {
			pos = len(tag)
		}
		nameHash[i] = xxh3.HashString(tag[:pos])
	}
	return HashedTags{
		hashedTags: newHashedTagsFromSlice(tags),
		nameHash:   nameHash,
	}
}

// Get returns the internal slice.
//
// NOTE: this returns a mutable reference to data in this immutable data structure.
// It is still used by comp/core/tagger/tagstore, but new uses should not be added.
func (h HashedTags) Get() []string {
	return h.data
}

// Slice returns a shared sub-slice of tags from t.
func (h HashedTags) Slice(i, j int) HashedTags {
	result := HashedTags{
		hashedTags: hashedTags{
			data: h.data[i:j],
			hash: h.hash[i:j],
		},
	}
	if h.nameHash != nil {
		result.nameHash = h.nameHash[i:j]
	}
	return result
}
