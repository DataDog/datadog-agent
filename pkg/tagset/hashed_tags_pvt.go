// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "github.com/twmb/murmur3"

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
