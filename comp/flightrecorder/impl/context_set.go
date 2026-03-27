// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync/atomic"
)

// contextSet tracks which metric context keys have been sent with full name+tags
// definitions. Implemented as a lock-free bloom filter using atomic bit operations.
//
// Memory: ~586 KB fixed (4.8M bits) regardless of context count — 10× less than
// the sharded map it replaces (~8 MB at 500K contexts).
//
// Trade-off: bloom filters do not support deletion. Evicted context definitions
// are not re-sent. The sidecar handles unknown context_keys gracefully.
// Reset on reconnect clears all bits.
type contextSet struct {
	bits []atomic.Uint64 // bit array, each word is 64 bits
	m    uint64          // total bits
	k    int             // number of hash functions
}

// Bloom filter parameters for ~500K elements at 1% FPR:
//   m = 4_795_200 bits (74925 uint64 words = ~585 KB)
//   k = 7 hash functions
const (
	bloomBits  = 4_795_200
	bloomWords = (bloomBits + 63) / 64 // 74925
	bloomK     = 7
)

// newContextSet creates a bloom-filter-based context set.
// The cap parameter is accepted for API compatibility but ignored.
func newContextSet(_ int) *contextSet {
	return &contextSet{
		bits: make([]atomic.Uint64, bloomWords),
		m:    bloomBits,
		k:    bloomK,
	}
}

// IsKnown checks if the key is probably in the set. If not, it adds it and
// returns false. Lock-free using atomic CompareAndSwap on each bit word.
//
// False positives are possible (~1% FPR) but false negatives are not.
func (cs *contextSet) IsKnown(key uint64) bool {
	h1 := key
	h2 := (key >> 17) | (key << 47)

	// Phase 1: check all k positions (read-only, no atomic RMW).
	allSet := true
	for i := 0; i < cs.k; i++ {
		pos := (h1 + uint64(i)*h2) % cs.m
		word := pos / 64
		bit := uint64(1) << (pos % 64)
		if cs.bits[word].Load()&bit == 0 {
			allSet = false
			break
		}
	}
	if allSet {
		return true
	}

	// Phase 2: set all k bits using atomic OR (CAS loop).
	for i := 0; i < cs.k; i++ {
		pos := (h1 + uint64(i)*h2) % cs.m
		word := pos / 64
		bit := uint64(1) << (pos % 64)
		for {
			old := cs.bits[word].Load()
			if old&bit != 0 {
				break // already set
			}
			if cs.bits[word].CompareAndSwap(old, old|bit) {
				break
			}
		}
	}
	return false
}

// Remove is a no-op. Bloom filters do not support deletion.
func (cs *contextSet) Remove(_ uint64) {}

// Reset clears all bits, forcing all context definitions to be re-sent.
// Not lock-free (called rarely — only on reconnect).
func (cs *contextSet) Reset() {
	for i := range cs.bits {
		cs.bits[i].Store(0)
	}
}

// CheckCap is a no-op — the bloom filter has fixed size.
func (cs *contextSet) CheckCap() bool {
	return false
}

// Len returns 0. The bloom filter does not track exact count.
func (cs *contextSet) Len() int64 {
	return 0
}
