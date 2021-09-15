// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"math/bits"

	"github.com/DataDog/datadog-agent/pkg/util"
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
//
// Must be a power of two.
const hashSetSize = 512

// bruteforceSize is the threshold number of tags below which a bruteforce algorithm is
// faster than a hashset.
const bruteforceSize = 4

// blank is a marker value to indicate that hashset slot is vacant.
const blank = -1

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	g := &KeyGenerator{}

	for i := 0; i < len(g.empty); i++ {
		g.empty[i] = blank
	}

	return g
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
	seen [hashSetSize]uint64
	// seenIdx is the index of the tag stored in the hashset
	seenIdx [hashSetSize]int16
	// empty is an empty hashset with all values set to `blank`, to reset `seenIdx`
	empty [hashSetSize]int16
}

// Generate returns the ContextKey hash for the given parameters.
// tagsBuf is re-arranged in place and truncated to only contain unique tags.
func (g *KeyGenerator) Generate(name, hostname string, tagsBuf *util.HashingTagsBuilder) ContextKey {
	// between two generations, we have to set the hash to something neutral, let's
	// use this big value seed from the murmur3 implementations
	g.intb = 0xc6a4a7935bd1e995

	g.intb = g.intb ^ murmur3.StringSum64(name)
	g.intb = g.intb ^ murmur3.StringSum64(hostname)

	// There are three implementations used here to deduplicate the tags depending on how
	// many tags we have to process:
	//   -  16 < n < hashSetSize:	we use a hashset of `hashSetSize` values.
	//   -  n < 16:                 we use a simple for loops, which is faster than
	//                          	the hashset when there is less than 16 tags
	//   - n > hashSetSize:         sort

	if tagsBuf.Len() > hashSetSize {
		tagsBuf.SortUniq()
		for _, h := range tagsBuf.Hashes() {
			g.intb = g.intb ^ h
		}
	} else if tagsBuf.Len() > bruteforceSize {
		tags := tagsBuf.Get()
		hashes := tagsBuf.Hashes()

		// reset the `seen` hashset.
		// it copies `g.empty` instead of using make because it's faster

		// for smaller tag sets, initialize only a portion of the array. when len(tags) is
		// close to a power of two, size one up to keep hashset load low.
		size := 1 << bits.Len(uint(len(tags)+len(tags)/8))
		if size > hashSetSize {
			size = hashSetSize
		}
		mask := uint64(size - 1)
		copy(g.seenIdx[:size], g.empty[:size])

		ntags := len(tags)
		for i := 0; i < ntags; {
			h := hashes[i]
			j := h & mask // address this hash into the hashset
			for {
				if g.seenIdx[j] == blank {
					// not seen, we will add it to the hash
					g.seen[j] = h
					g.seenIdx[j] = int16(i)
					g.intb = g.intb ^ h // add this tag into the hash
					i++
					break
				} else if g.seen[j] == h && tags[g.seenIdx[j]] == tags[i] {
					// already seen, we do not want to xor multiple times the same tag
					tags[i] = tags[ntags-1]
					hashes[i] = hashes[ntags-1]
					ntags--
					break
				} else {
					// move 'right' in the hashset because there is already a value,
					// in this bucket, which is not the one we're dealing with right now,
					// we may have already seen this tag
					j = (j + 1) & mask
				}
			}
		}
		tagsBuf.Truncate(ntags)
	} else {
		tags := tagsBuf.Get()
		hashes := tagsBuf.Hashes()
		ntags := tagsBuf.Len()
	OUTER:
		for i := 0; i < ntags; {
			h := hashes[i]
			for j := 0; j < i; j++ {
				if g.seen[j] == h && tags[j] == tags[i] {
					tags[i] = tags[ntags-1]
					hashes[i] = hashes[ntags-1]
					ntags--
					continue OUTER // we do not want to xor multiple times the same tag
				}
			}
			g.intb = g.intb ^ h
			g.seen[i] = h
			i++
		}
		tagsBuf.Truncate(ntags)
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
