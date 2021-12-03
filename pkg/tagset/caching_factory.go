// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"strings"

	"github.com/twmb/murmur3"
)

// A cachingFactory is a Factory implementation that caches Tags instances.
// See NewCachingFactory for usage.
type cachingFactory struct {
	baseFactory

	// Tags instances are cached by 64-bit cache keys that can have a range of
	// meanings; each CacheID identifies a different such meaning. Caches with
	// different CacheIDs are stored independently.
	caches [numCacheIDs]tagsCache
}

var _ Factory = (*cachingFactory)(nil)

// NewCachingFactory creates a new caching factory. A caching factory caches
// Tags instances when they are seen, and uses those cached values when possible
// to reduce CPU and memory usage.
//
// The size of the factory's cache is proportional to cacheSize: double this
// parameter and memory usage will roughly double. The cache size must be at
// least 1.
//
// For a given cache size, the cacheWidth parameter determines how well the
// cache handles eviction. If set to 1, the entire cache is thrown out when it
// is full. Larger values do a better job of holding on to
// less-frequently-referenced values, at the expense of more CPU time spent
// searching for cached instances. A value of 5 is a nice starting point. The
// cache width must be at least 1.
//
// Caching factories are not threadsafe. Wrap with a ThreadsafeFactory if
// thread safety is required.
func NewCachingFactory(cacheSize, cacheWidth int) Factory {
	if cacheSize < 1 {
		panic("cacheSize must be at least 1")
	}
	if cacheWidth < 1 {
		panic("cacheWidth must be at least 1")
	}

	// approximate the tagsCache parameters, rounding up
	insertsPerRotation := cacheSize/cacheWidth + 1
	cacheCount := cacheWidth

	var caches [numCacheIDs]tagsCache
	for i := range caches {
		caches[i] = newTagsCache(insertsPerRotation, cacheCount)
	}
	return &cachingFactory{
		caches: caches,
	}
}

// NewTags implements Factory.NewTags
func (f *cachingFactory) NewTags(tags []string) *Tags {
	tagsMap := make(map[uint64]string, len(tags))
	hash := uint64(0)
	for _, t := range tags {
		h := murmur3.StringSum64(t)
		_, seen := tagsMap[h]
		if seen {
			continue
		}
		tagsMap[h] = t
		hash ^= h
	}

	return f.getCachedTags(byTagsetHashCache, hash, func() *Tags {
		// write hashes and rewrite tags based on the map
		hashes := make([]uint64, len(tagsMap))
		tags = tags[:len(tagsMap)]
		i := 0
		for h, t := range tagsMap {
			tags[i] = t
			hashes[i] = h
			i++
		}

		return &Tags{tags, hashes, hash}
	})
}

// NewUniqueTags implements Factory.NewUniqueTags
func (f *cachingFactory) NewUniqueTags(tags ...string) *Tags {
	hashes, hash := calcHashes(tags)
	return f.getCachedTags(byTagsetHashCache, hash, func() *Tags {
		return &Tags{tags, hashes, hash}
	})
}

// NewTagsFromMap implements Factory.NewTagsFromMap
func (f *cachingFactory) NewTagsFromMap(src map[string]struct{}) *Tags {
	tags := make([]string, 0, len(src))
	for tag := range src {
		tags = append(tags, tag)
	}
	hashes, hash := calcHashes(tags)
	return f.getCachedTags(byTagsetHashCache, hash, func() *Tags {
		return &Tags{tags, hashes, hash}
	})
}

// NewTag implements Factory.NewTag
func (f *cachingFactory) NewTag(tag string) *Tags {
	hash := murmur3.StringSum64(tag)
	return f.getCachedTags(byTagsetHashCache, hash, func() *Tags {
		return &Tags{[]string{tag}, []uint64{hash}, hash}
	})
}

// NewBuilder implements Factory.NewBuilder
func (f *cachingFactory) NewBuilder(capacity int) *Builder {
	return f.baseFactory.newBuilder(f, capacity)
}

// NewSliceBuilder implements Factory.NewSliceBuilder
func (f *cachingFactory) NewSliceBuilder(levels, capacity int) *SliceBuilder {
	return f.baseFactory.newSliceBuilder(f, levels, capacity)
}

// ParseDSD implements Factory.ParseDSD
func (f *cachingFactory) ParseDSD(data []byte) (*Tags, error) {
	return f.getCachedTags(byDSDHashCache, murmur3.Sum64(data), func() *Tags {
		tags := strings.Split(string(data), ",")
		return f.NewTags(tags)
	}), nil
}

// Union implements Factory.Union
func (f *cachingFactory) Union(a, b *Tags) *Tags {
	key := unionCacheKey(a.Hash(), b.Hash())
	return f.getCachedTags(byUnionHashCache, key, func() *Tags {
		tags := make(map[string]struct{}, len(a.tags)+len(b.tags))
		for _, t := range a.tags {
			tags[t] = struct{}{}
		}
		for _, t := range b.tags {
			tags[t] = struct{}{}
		}
		return f.NewTagsFromMap(tags)
	})
}

// UnsafeDisjointUnion implements Factory.UnsafeDisjointUnion
func (f *cachingFactory) UnsafeDisjointUnion(a, b *Tags) *Tags {
	hash := a.hash ^ b.hash
	return f.getCachedTags(byTagsetHashCache, hash, func() *Tags {

		tags := make([]string, len(a.tags)+len(b.tags))
		copy(tags[:len(a.tags)], a.tags)
		copy(tags[len(a.tags):], b.tags)

		hashes := make([]uint64, len(a.hashes)+len(b.hashes))
		copy(hashes[:len(a.hashes)], a.hashes)
		copy(hashes[len(a.hashes):], b.hashes)
		return &Tags{tags, hashes, hash}
	})
}

// getCachedTags implements Factory.getCachedTags
func (f *cachingFactory) getCachedTags(cacheID cacheID, key uint64, miss func() *Tags) *Tags {
	return f.caches[cacheID].getCachedTags(key, miss)
}

// getCachedTagsErr implements Factory.getCachedTagsErr
func (f *cachingFactory) getCachedTagsErr(cacheID cacheID, key uint64, miss func() (*Tags, error)) (*Tags, error) {
	return f.caches[cacheID].getCachedTagsErr(key, miss)
}
