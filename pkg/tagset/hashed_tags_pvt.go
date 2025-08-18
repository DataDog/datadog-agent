// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"slices"
	"unique"

	"github.com/twmb/murmur3"
)

// hashedTags is the base type for HashingTagsAccumulator and HashedTags
type hashedTags struct {
	data []unique.Handle[string]
	hash []uint64
}

func newHashedTagsWithCapacity(cap int) hashedTags {
	return hashedTags{
		data: make([]unique.Handle[string], 0, cap),
		hash: make([]uint64, 0, cap),
	}
}

func newHashedTagsFromSlice(tags []unique.Handle[string]) hashedTags {
	hash := make([]uint64, 0, len(tags))
	for _, t := range tags {
		hash = append(hash, murmur3.StringSum64(t.Value()))
	}
	return hashedTags{
		data: tags,
		hash: hash,
	}
}

func newHashedTagsFromStringSlice(tags []string) hashedTags {
	data := make([]unique.Handle[string], 0, len(tags))
	for _, t := range tags {
		data = append(data, unique.Make(t))
	}
	return newHashedTagsFromSlice(data)
}

// Copy returns a new slice with the copy of the tags
func (h hashedTags) Copy() []unique.Handle[string] {
	return slices.Clone(h.data)
}

// Len returns number of tags
func (h hashedTags) Len() int {
	return len(h.data)
}

// dup returns a full copy of hashedTags
func (h hashedTags) dup() hashedTags {
	return hashedTags{
		data: slices.Clone(h.data),
		hash: slices.Clone(h.hash),
	}
}
