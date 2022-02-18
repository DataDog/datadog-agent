// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"encoding/binary"

	"github.com/twmb/murmur3"
)

// unionCacheKey calculates a cache key that can be used to memoize Union(a, b)
// so that an existing tagset can be discovered in the cache and we can avoid
// performing the expensive union operation.
//
// To memoize `Union(a, b)`, we need a 64-bit cache key based on the hashes of the
// input tagsets, `cachekey(a.Hash, b.Hash)`, that will uniquely identify those
// two inputs with a collision probability close to 1/2**64. Note that this cachekey
// is _not_ the hash of the union. In other words,
//
//     Union(a, b).Hash != cachekey(a.Hash, b.Hash)
//
// The concern about collisions is not with random choices of input sets, but for
// similar sets. For example, given
//
//     Union(["abc"], ["ghi"])                = ["abc", "ghi"]
//     Union(["abc", "def"], ["def", "ghi"])  = ["abc", "def", "ghi"]
//
// The unions differ, so the cache keys must also differ
//
//     cachekey(["abc"], ["ghi"]) != cachekey(["abc", "def"], ["def", "ghi"])
//
// More formally, given two 64-bit tagset hashes a and b, and an additional
// nonzero tag hash t, we want to ensure that the pairwise probability of equality
// between any of
//
//     cachekey(a, b)     for Union(["a", "aa"],        ["b", "bb"])
//     cachekey(a^t, b)   for Union(["a", "aa", "t"],   ["b", "bb"])
//     cachekey(a, b^t)   for Union(["a", "aa"],        ["b", "bb", "t"])
//     cachekey(a^t, b^t) for Union(["a", "aa", "t"],   ["b", "bb", "t"])
//
// is close to 1/2**64.
//
// XOR (`cachekey(a, b) := a ^ b`) has a 33% collision rate, so that's no good.
//
// Empirically, addition (`cachekey(a, b) := (a + b) & MAXUINT64`) performs better
// than XOR, but still quite poorly.
//
// A well-distributed hash function, using `a.Hash` and `b.Hash` as inputs, can
// get to baseline probability.
//
// We already use murmur3 elsewhere, and in this case it performs about 4x faster
// than hash/fnv, so it's used here.
func unionCacheKey(aHash, bHash uint64) uint64 {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], aHash)
	binary.LittleEndian.PutUint64(buf[8:], bHash)
	return murmur3.Sum64(buf[:])
}

// union performs a union operation over two tagsets.  The "easy" cases have already
// been handled, and the cache has missed.
func union(a *Tags, b *Tags) *Tags {
	la := len(a.tags)
	lb := len(b.tags)

	// ensure a is the larger set
	if la < lb {
		a, b = b, a
		la, lb = lb, la
	}

	tags := make([]string, la, la+lb)
	hashes := make([]uint64, la, la+lb)
	seen := make(map[string]struct{}, la)

	// copy a to tags
	copy(tags[:la], a.tags)
	copy(hashes[:la], a.hashes)
	hash := a.hash

	// update seen with the content of a
	for _, t := range tags {
		seen[t] = struct{}{}
	}

	// iterate over b, adding what has not been seen.  b contains no duplicates,
	// so it is not necessary to insert into seen again here
	btags := b.tags
	bhashes := b.hashes
	for i, t := range btags {
		if _, s := seen[t]; !s {
			h := bhashes[i]
			tags = append(tags, t)
			hashes = append(hashes, h)
			hash ^= h
		}
	}

	return &Tags{tags, hashes, hash}
}
