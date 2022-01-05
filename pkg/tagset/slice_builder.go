// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"github.com/twmb/murmur3"
)

// SliceBuilder is used to build tagsets tag-by-tag, before "freezing" into a
// Tags instance.
//
// SliceBuilder is different from Builder because it associates a "level" with
// each tag, and allows the creation of Tags instances that include tags with
// specific levels, and those Tags instances can share storage. For example,
// this is useful when generating tags of different cardinalities (mapping
// cardinalities to levels).  Then the low-cardinality tags can be sliced off
// separately from the low-and-medium cardinality tags, sharing storage for
// those low-cardinality tags.
//
// A SliceBuilder goes through three stages in its lifecycle:
//
//     1. adding tags (begins on call to factory.NewSliceBuilder).
//     2. frozen (begins on call to bldr.FreezeSlice).
//     3. closed (begins on call to bldr.Close).
//
// The Add* methods may only be called in the "adding tags" stage. No methods
// may be called in the "closed" stage.
//
// In general, the easiest way to ensure these stages are followed (and allow reviewers
// to verify this) is to use a builder in a single method, as shown in the example. Avoid
// storing builders in structs.
//
// SliceBuilders are not thread-safe.
type SliceBuilder struct {
	factory Factory

	// Number of levels this builder supports
	levels int

	// Offsets into the tags/hashes arrays, such that level L is represented as
	// tags[offsets[L]:offsets[L+1]]. This slice has length (levels+1).
	offsets []int

	// The built slice of tags
	tags []string

	// Hashes of individual tags, with each element corresponding to the hash
	// of that tag. INVARIANT: len(tags) == len(hashes), cap(tags) ==
	// cap(hashes)
	hashes []uint64

	// Hashes of all tags at each level. This slice has length levels.
	levelHashes []uint64

	// all seen tags (regardless of level)
	seen map[uint64]struct{}
}

// newSliceBuilder creates a new builder. This must be reset()
// before use.
func newSliceBuilder(factory Factory) *SliceBuilder {
	return &SliceBuilder{
		factory: factory,
	}
}

// reset the builder, preparing it for re-use
func (bldr *SliceBuilder) reset(levels, capacity int) {
	bldr.levels = levels

	// expand bldr.offsets to at least length levels+1
	if bldr.offsets == nil || cap(bldr.offsets) < levels+1 {
		bldr.offsets = make([]int, levels+1)
	} else {
		bldr.offsets = bldr.offsets[:levels+1]
	}

	for i := 0; i < levels+1; i++ {
		bldr.offsets[i] = 0
	}

	// expand bldr.offsets to at least length levels
	if bldr.levelHashes == nil || cap(bldr.levelHashes) < levels {
		bldr.levelHashes = make([]uint64, levels)
	} else {
		bldr.levelHashes = bldr.levelHashes[:levels]
	}

	for i := 0; i < levels; i++ {
		bldr.levelHashes[i] = 0
	}

	bldr.tags = make([]string, 0, capacity)
	bldr.hashes = make([]uint64, 0, capacity)
	bldr.seen = map[uint64]struct{}{}
}

// Add adds the given tag to the builder at the given level. If the tag is
// already in the builder, it is not added again (regardless of the existing
// tag's level).
func (bldr *SliceBuilder) Add(level int, tag string) {
	h := murmur3.StringSum64(tag)
	if _, seen := bldr.seen[h]; seen {
		return
	}
	bldr.seen[h] = struct{}{}

	newLen := len(bldr.tags) + 1
	insertAt := bldr.offsets[level+1]

	var newTags []string
	var newHashes []uint64

	// reallocate the storage if there is not enough room in the existing
	// array, and copy the existing data into the new array
	if cap(bldr.tags) < newLen {
		newTags = make([]string, newLen, newLen*2)
		newHashes = make([]uint64, newLen, newLen*2)

		copy(newTags[:insertAt], bldr.tags[:insertAt])
		copy(newTags[insertAt+1:], bldr.tags[insertAt:])

		copy(newHashes[:insertAt], bldr.hashes[:insertAt])
		copy(newHashes[insertAt+1:], bldr.hashes[insertAt:])
	} else {
		newTags = bldr.tags[:newLen]
		newHashes = bldr.hashes[:newLen]

		// make space for the new element with the minimum number of copies,
		// moving the first element of each level's slice to the end of that
		// slice
		for l := bldr.levels - 1; l > level; l-- {
			s := bldr.offsets[l]
			d := bldr.offsets[l+1]
			newTags[d], newHashes[d] = newTags[s], newHashes[s]
		}
	}

	newTags[insertAt] = tag
	newHashes[insertAt] = h

	bldr.tags = newTags
	bldr.hashes = newHashes

	bldr.levelHashes[level] ^= h
	for l := level + 1; l < bldr.levels+1; l++ {
		bldr.offsets[l]++
	}
}

// AddKV adds the tag "k:v" to the builder at the given level. If the tag is
// already in the builder, it is not added again (regardless of the existing
// tag's level).
func (bldr *SliceBuilder) AddKV(level int, k, v string) {
	tag := k + ":" + v
	bldr.Add(level, tag)
}

// FreezeSlice "freezes" the builder and returns the requested slice of levels.
// The Add methods cannot be called after freezing.
func (bldr *SliceBuilder) FreezeSlice(a, b int) *Tags {
	// free unnecessary memory
	bldr.seen = nil

	hash := uint64(0)
	for _, lh := range bldr.levelHashes[a:b] {
		hash ^= lh
	}

	// Whether to use the byTagsetHashCache here is a judgement call. The
	// downside is that it may end up caching a Tags that refers to a slice
	// larger than necessary. It's also an additional hash-table lookup when
	// we already have all of the data necessary to create a new Tags instance.
	// That said, in cases where only low-cardinality tags are used, it may
	// result in significant memory savings.
	//
	// Another option to explore is to cache the entire built set of tags, and
	// query that cached value on the first call to FreezeSlice, borrowing the
	// cached tags and hashes values if there is a match.

	return bldr.factory.getCachedTags(byTagsetHashCache, hash, func() *Tags {
		tags := bldr.tags[bldr.offsets[a]:bldr.offsets[b]]
		hashes := bldr.hashes[bldr.offsets[a]:bldr.offsets[b]]
		return &Tags{tags, hashes, hash}
	})
}

// Close closes the builder, freeing its resources.
func (bldr *SliceBuilder) Close() {
	bldr.seen = nil
	bldr.hashes = nil
	bldr.tags = nil
	bldr.factory.sliceBuilderClosed(bldr)
}
