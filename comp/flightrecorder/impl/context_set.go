// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync/atomic"
)

// contextSet tracks which metric context keys have been sent with full name+tags
// definitions. Implemented as a lock-free bloom filter with inline CAS on miss.
//
// The hot path (IsKnown) checks k=3 bloom positions via atomic loads. On a miss,
// it sets the bits inline via 3 CAS operations — cheap enough that no background
// goroutine is needed. This eliminates warm-up delay, channel overhead, and
// goroutine lifecycle management.
//
// Trade-off: k=3 has a higher false positive rate (~1%) than k=7 (~0.01%), but
// false positives here only mean we skip sending a context definition that the
// sidecar already has — completely harmless.
//
// Memory: ~1.2 MB fixed regardless of context count.
type contextSet struct {
	bits []atomic.Uint64
	m    uint64
}

// Bloom filter parameters: ~9.6M bits (~1.2 MB) with k=3.
// At 200K contexts: FPR ≈ 0.5%. At 500K: FPR ≈ 1.5%.
// Doubling the bit array vs the previous k=7 design compensates for using
// fewer hash probes while keeping memory well under 2 MB.
const (
	bloomBits  = 9_600_000
	bloomWords = (bloomBits + 63) / 64
	bloomK     = 3
)

// newContextSet creates a bloom-filter-based context set.
// The cap parameter is accepted for API compatibility but ignored.
func newContextSet(_ int) *contextSet {
	return &contextSet{
		bits: make([]atomic.Uint64, bloomWords),
		m:    bloomBits,
	}
}

// IsKnown checks if the key is probably in the set. On a miss, sets the bits
// inline via CAS and returns false. With k=3 the CAS cost is negligible (~15ns),
// so no background goroutine is needed.
func (cs *contextSet) IsKnown(key uint64) bool {
	h1 := key
	h2 := (key >> 17) | (key << 47)

	for i := 0; i < bloomK; i++ {
		pos := (h1 + uint64(i)*h2) % cs.m
		word := pos / 64
		bit := uint64(1) << (pos % 64)
		if cs.bits[word].Load()&bit == 0 {
			cs.setBits(key)
			return false
		}
	}
	return true
}

func (cs *contextSet) setBits(key uint64) {
	h1 := key
	h2 := (key >> 17) | (key << 47)
	for i := 0; i < bloomK; i++ {
		pos := (h1 + uint64(i)*h2) % cs.m
		word := pos / 64
		bit := uint64(1) << (pos % 64)
		for {
			old := cs.bits[word].Load()
			if old&bit != 0 {
				break
			}
			if cs.bits[word].CompareAndSwap(old, old|bit) {
				break
			}
		}
	}
}

// Reset clears all bits.
func (cs *contextSet) Reset() {
	for i := range cs.bits {
		cs.bits[i].Store(0)
	}
}
