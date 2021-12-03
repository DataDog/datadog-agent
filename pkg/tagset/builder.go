// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "github.com/twmb/murmur3"

// Builder is used to build tagsets tag-by-tag, before "freezing" into a Tags instance. Builders are not threadsafe.
//
// A Builder goes through three stages in its lifecycle:
//
//     1. adding tags (begins on call to factory.NewBuilder).
//     2. frozen (begins on call to bldr.Freeze).
//     3. closed (begins on call to bldr.Close).
//
// The Add* methods may only be called in the "adding tags" stage. No methods may be called in the "closed" stage.
type Builder struct {
	factory Factory
	tags    []string
	hashes  []uint64
	hash    uint64
	seen    map[uint64]struct{}
	frozen  *Tags
}

// newBuilder creates a new builder. This must be reset()
// before use.
func newBuilder(factory Factory) *Builder {
	return &Builder{
		factory: factory,
		tags:    nil,
		frozen:  nil,
	}
}

// reset the builder, preparing it for re-use
func (bldr *Builder) reset(capacity int) {
	// ensure at least the requested capacity for bldr.tags
	if bldr.tags == nil || cap(bldr.tags) < capacity {
		bldr.tags = make([]string, 0, capacity)
	} else {
		bldr.tags = bldr.tags[:0]
	}

	// ensure at least the requested capacity for bldr.hashes
	if bldr.hashes == nil || cap(bldr.hashes) < capacity {
		bldr.hashes = make([]uint64, 0, capacity)
	} else {
		bldr.hashes = bldr.hashes[:0]
	}

	bldr.seen = map[uint64]struct{}{}
	bldr.frozen = nil
}

// Add adds the given tag to the builder
func (bldr *Builder) Add(tag string) {
	h := murmur3.StringSum64(tag)
	if _, seen := bldr.seen[h]; seen {
		return
	}
	bldr.tags = append(bldr.tags, tag)
	bldr.hashes = append(bldr.hashes, h)
	bldr.hash ^= h
	bldr.seen[h] = struct{}{}
}

// AddTags adds the contents of another Tags instance to this builder.
func (bldr *Builder) AddTags(tags *Tags) {
	tags.ForEach(func(t string) {
		bldr.Add(t)
	})
}

// AddKV adds the tag "k:v" to the builder
func (bldr *Builder) AddKV(k, v string) {
	tag := k + ":" + v
	bldr.Add(tag)
}

// Contains checks whether the given tag is in the builder
func (bldr *Builder) Contains(tag string) bool {
	h := murmur3.StringSum64(tag)
	_, has := bldr.seen[h]
	return has
}

// Freeze "freezes" the builder and returns the resulting tagset. The Add methods
// cannot be called after freezing.
func (bldr *Builder) Freeze() *Tags {
	if bldr.frozen == nil {
		bldr.frozen = bldr.factory.getCachedTags(byTagsetHashCache, bldr.hash, func() *Tags {

			tags, hashes, hash := bldr.tags, bldr.hashes, bldr.hash
			// the Tags instance will own the storage in these slices, so reset them
			bldr.tags = []string{}
			bldr.hashes = []uint64{}

			return &Tags{tags, hashes, hash}
		})
		bldr.seen = nil // free unnecessary memory
	}
	return bldr.frozen
}

// Close closes the builder, freeing its resources.
func (bldr *Builder) Close() {
	bldr.seen = nil // free unnecessary memory
	bldr.factory.builderClosed(bldr)
}
