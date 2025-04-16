// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "math/bits"

// A HashGenerator generates hashes for tag sets, with the property that
// the hash is invariant over re-ordering of tags and duplicate tags.
//
// This type holds storage for hash operations that can be re-used between
// operations.  It is not threadsafe and the caller must ensure that an
// instance's Hash method is not called concurrently.
type HashGenerator struct {
	// seen is used as a hashset to deduplicate the tags when there is more than
	// 16 and less than 512 tags.
	seen [hashSetSize]uint64
	// seenIdx is the index of the tag stored in the hashset
	seenIdx [hashSetSize]int16
	// empty is an empty hashset with all values set to `blank`, to reset `seenIdx`.
	// because this value is nonzero, bulk-copying an array is faster than setting
	// all values to blank in a loop
	empty [hashSetSize]int16
}

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

// NewHashGenerator creates a new HashGenerator
func NewHashGenerator() *HashGenerator {
	g := &HashGenerator{}
	for i := 0; i < len(g.empty); i++ {
		g.empty[i] = blank
	}
	return g
}

// Hash calculates the cumulative XOR of all unique tags in the builder.  As a side-effect,
// it sorts and deduplicates the hashes contained in the builder.
func (g *HashGenerator) Hash(tb *HashingTagsAccumulator) uint64 {
	var hash uint64

	// This implementation has been designed to remove all heap
	// allocations from the intake in order to reduce GC pressure on high volumes.
	//
	// There are three implementations used here to deduplicate the tags
	// depending on how many tags we have to process:
	//  - 16 < n < hashSetSize: // we use a hashset of `hashSetSize` values.
	//  - n < 16: we use a simple for loop, which is faster than the hashset when there is
	//    less than 16 tags
	//  - n > hashSetSize: sort
	if tb.Len() > hashSetSize {
		tb.SortUniq()
		for _, h := range tb.hash {
			hash ^= h
		}
	} else if tb.Len() > bruteforceSize {
		tags := tb.data
		hashes := tb.hash

		// reset the `seen` hashset.
		// it copies `g.empty` instead of using make because it's faster

		// for smaller tag sets, initialize only a portion of the array. when len(tags) is
		// close to a power of two, size one up to keep hashset load low.
		size := min(1<<bits.Len(uint(len(tags)+len(tags)/8)), hashSetSize)
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
					hash ^= h // add this tag into the hash
					i++
					break
				} else if g.seen[j] == h && tags[g.seenIdx[j]] == tags[i] {
					// already seen, we do not want to xor multiple times the same tag
					tags[i] = tags[ntags-1]
					hashes[i] = hashes[ntags-1]
					ntags--
					break
				}
				// move 'right' in the hashset because there is already a value,
				// in this bucket, which is not the one we're dealing with right now,
				// we may have already seen this tag
				j = (j + 1) & mask
			}
		}
		tb.Truncate(ntags)
	} else {
		tags := tb.data
		hashes := tb.hash
		ntags := tb.Len()
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
			hash ^= h
			g.seen[i] = h
			i++
		}
		tb.Truncate(ntags)
	}

	return hash
}

// Dedup2 removes duplicates from two tags accumulators. Duplicate tags are removed, so at the end
// tag each tag is present once in either l or r, but not both at the same time.
//
// First, duplicates are removed from l. Then duplicates are removed from r, including any tags that
// are already present in l.
func (g *HashGenerator) Dedup2(l *HashingTagsAccumulator, r *HashingTagsAccumulator) {
	ntags := l.Len() + r.Len()

	// This implementation has been designed to remove all heap
	// allocations from the intake in order to reduce GC pressure on high volumes.
	//
	// There are three implementations used here to deduplicate the tags
	// depending on how many tags we have to process:
	//  - 16 < n < hashSetSize: // we use a hashset of `hashSetSize` values.
	//  - n < 16: we use a simple for loop, which is faster than the hashset when there is
	//    less than 16 tags
	//  - n > hashSetSize: sort
	if ntags > hashSetSize {
		l.SortUniq()
		r.SortUniq()
		r.removeSorted(l)
	} else if ntags > bruteforceSize {
		// reset the `seen` hashset.
		// it copies `g.empty` instead of using make because it's faster

		// for smaller tag sets, initialize only a portion of the array. when len(tags) is
		// close to a power of two, size one up to keep hashset load low.
		size := min(1<<bits.Len(uint(ntags+ntags/8)), hashSetSize)
		mask := uint64(size - 1)
		copy(g.seenIdx[:size], g.empty[:size])

		ibase := int16(0)
		for _, tb := range [2]*HashingTagsAccumulator{l, r} {
			tags := tb.data
			hashes := tb.hash
			ntags := len(hashes)

			for i := 0; i < ntags; {
				h := hashes[i]
				j := h & mask
				for {
					if g.seenIdx[j] == blank {
						g.seen[j] = h
						g.seenIdx[j] = int16(i) + ibase
						i++
						break
					} else if g.seen[j] == h {
						idx := g.seenIdx[j]
						if (idx >= ibase && tags[idx-ibase] == tags[i]) ||
							(idx < ibase && l.data[idx] == tags[i]) {
							tags[i] = tags[ntags-1]
							hashes[i] = hashes[ntags-1]
							ntags--
							break
						}
					}
					j = (j + 1) & mask
				}
			}
			ibase = int16(ntags)
			tb.Truncate(ntags)
		}
	} else { // ntags <= bruteforceSize
		ldata := l.data
		lhash := l.hash
		lsize := len(ldata)

	L:
		for i := 0; i < lsize; {
			h := lhash[i]
			for j := 0; j < i; j++ {
				if g.seen[j] == h && ldata[j] == ldata[i] {
					lsize--
					ldata[i] = ldata[lsize]
					lhash[i] = lhash[lsize]
					continue L
				}
			}
			g.seen[i] = h
			i++
		}
		l.Truncate(lsize)

		rdata := r.data
		rhash := r.hash
		rsize := len(rdata)
	R:
		for i := 0; i < rsize; {
			h := rhash[i]
			for j := 0; j < lsize; j++ {
				if g.seen[j] == h && ldata[j] == rdata[i] {
					rsize--
					rdata[i] = rdata[rsize]
					rhash[i] = rhash[rsize]
					continue R
				}
			}
			for j := 0; j < i; j++ {
				if g.seen[lsize+j] == h && rdata[j] == rdata[i] {
					rsize--
					rdata[i] = rdata[rsize]
					rhash[i] = rhash[rsize]
					continue R
				}
			}
			g.seen[lsize+i] = h
			i++
		}
		r.Truncate(rsize)

	}
}
