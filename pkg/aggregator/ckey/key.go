// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"github.com/twmb/murmur3"
)

// ContextKey is a non-cryptographic hash that allows to
// aggregate metrics from a same context together.
//
// This implementation has been designed to remove all heap
// allocations from the intake in order to reduce GC pressure on high volumes.
//
// Having int64/uint64 context keys mean that we will get better performances
// from the Go runtime while using them as map keys. This is thanks to the fast-path
// methods for map access and map assign with int64 keys.
// See for instance runtime.mapassign_fast64 or runtime.mapaccess2_fast64.
//
// Note that Agent <= 6.19.0 were using a 128 bits hash, we've switched
// to 64 bits for better performances (map access) and because 128 bits were overkill
// in the first place.
// Note that benchmarks against fnv1a did not provide better performances (no inlining)
// nor did benchmarks with xxhash (slightly slower).
type ContextKey uint64

// hashSetSize is the size selected for hashset used to deduplicate the tags
// while generating the hash. This size has been selected to have space for
// approximately 500 tags since it's not impacting much the performances,
// even if the backend is truncating after 100 tags.
const hashSetSize = 512

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{
		seen:  make([]uint64, 512, 512),
		empty: make([]uint64, 512, 512),
	}
}

// KeyGenerator generates hash for the given name, hostname and tags.
// The tags don't have to be sorted and duplicated tags will be ignored while
// generating the hash.
// Not safe for concurrent usage.
type KeyGenerator struct {
	// reused buffer to not create a uint64 on the stack every key generation
	intb uint64

	// seen is used as a hashset to deduplicate the tags when there is more than
	// 16 and less than 512 tags.
	seen []uint64
	// empty is an empty hashset with all values set to 0, to reset `seen`
	empty []uint64

	// idx is used to deduplicate tags when there is less than 16 tags (faster than the
	// hashset) or more than 512 tags (hashset has been allocated with 512 values max)
	idx int
}

// Generate returns the ContextKey hash for the given parameters.
// The tags array is sorted in place to avoid heap allocations.
func (g *KeyGenerator) Generate(name, hostname string, tags []string) ContextKey {
	// between two generations, we have to set the hash to something neutral, let's
	// use this big value seed from the murmur3 implementations
	g.intb = 0xc6a4a7935bd1e995

	g.intb = g.intb ^ murmur3.StringSum64(name)
	g.intb = g.intb ^ murmur3.StringSum64(hostname)

	// there is two implementations used here to deduplicate the tags depending on how
	// many tags we have to process:
	//   -  16 < n < hashSetSize:	we use a hashset of `hashSetSize` values.
	//   -  n < 16 or n > hashSetSize: we use a simple for loops, which is faster than
	//                         	the hashset when there is less than 16 tags, and
	//                         	we use it as fallback when there is more than `hashSetSize`
	//                         	because it is the maximum size the allocated
	//                         	hashset can handle.
	if len(tags) > 16 && len(tags) < hashSetSize {
		// reset the `seen` hashset.
		// it copies `g.empty` instead of using make because it's faster
		copy(g.seen, g.empty)
		for i := range tags {
			h := murmur3.StringSum64(tags[i])
			j := h & (hashSetSize - 1) // address this hash into the hashset
			for {
				if g.seen[j] == 0 {
					// not seen, we will add it to the hash
					// TODO(remy): we may want to store the original bytes instead
					// of the hash, even if the comparison would be slower, we would
					// be able to avoid collisions here.
					// See https://github.com/DataDog/datadog-agent/pull/8529#discussion_r661493647
					g.seen[j] = h
					g.intb = g.intb ^ h // add this tag into the hash
					break
				} else if g.seen[j] == h {
					// already seen, we do not want to xor multiple times the same tag
					break
				} else {
					// move 'right' in the hashset because there is already a value,
					// in this bucket, which is not the one we're dealing with right now,
					// we may have already seen this tag
					j = (j + 1) & (hashSetSize - 1)
				}
			}
		}
	} else {
		g.idx = 0
	OUTER:
		for i := range tags {
			h := murmur3.StringSum64(tags[i])
			for j := 0; j < g.idx; j++ {
				if g.seen[j] == h {
					continue OUTER // we do not want to xor multiple times the same tag
				}
			}
			g.intb = g.intb ^ h
			g.seen[g.idx] = h
			g.idx++
		}
	}

	return ContextKey(g.intb)
}

// Equals returns whether the two context keys are equal or not.
func Equals(a, b ContextKey) bool {
	return a == b
}

// IsZero returns true if the key is at zero value
func (k ContextKey) IsZero() bool {
	return k == 0
}
