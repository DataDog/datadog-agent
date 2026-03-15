// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"slices"
	"strings"

	"github.com/twmb/murmur3"
)

// TagHash holds a tag string and its precomputed hashes.
type TagHash struct {
	Tag      string
	Hash     uint64
	NameHash uint64
}

// newTagHash creates a TagHash, computing the hash of the full tag and the
// hash of the tag name (the part before the first ':').
func newTagHash(tag string) TagHash {
	var nameHash uint64
	if i := strings.IndexByte(tag, ':'); i >= 0 {
		nameHash = murmur3.StringSum64(tag[:i])
	} else {
		nameHash = murmur3.StringSum64(tag)
	}
	return TagHash{Tag: tag, Hash: murmur3.StringSum64(tag), NameHash: nameHash}
}

// hashedTags is the base type for HashingTagsAccumulator and HashedTags
type hashedTags struct {
	tags []TagHash
}

func newHashedTagsWithCapacity(cap int) hashedTags {
	return hashedTags{
		tags: make([]TagHash, 0, cap),
	}
}

func newHashedTagsFromSlice(strs []string) hashedTags {
	tags := make([]TagHash, 0, len(strs))
	for _, t := range strs {
		tags = append(tags, newTagHash(t))
	}
	return hashedTags{tags: tags}
}

// Copy returns a new slice with the copy of the tags
func (h hashedTags) Copy() []string {
	result := make([]string, len(h.tags))
	for i, th := range h.tags {
		result[i] = th.Tag
	}
	return result
}

// Len returns number of tags
func (h hashedTags) Len() int {
	return len(h.tags)
}

// sortByName sorts tags in place by NameHash.
func (h *hashedTags) sortByName() {
	slices.SortFunc(h.tags, func(a, b TagHash) int {
		if a.NameHash < b.NameHash {
			return -1
		}
		if a.NameHash > b.NameHash {
			return 1
		}
		return 0
	})
}

// dup returns a full copy of hashedTags
func (h hashedTags) dup() hashedTags {
	return hashedTags{
		tags: slices.Clone(h.tags),
	}
}
