// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package ckey

import (
	"sort"

	"github.com/twmb/murmur3"
)

const byteSize8 = 2

// ContextKey is a non-cryptographic hash that allows to
// aggregate metrics from a same context together.
//
// This implementation has been designed to remove all heap
// allocations from the intake to reduce GC pressure on high
// volumes.
//
// It uses the 128bit murmur3 hash, that is already successfully
// used on other products. 128bit is probably overkill for avoiding
// collisions, but it's better to err on the safe side, as we do not
// have a collision mitigation mechanism.
type ContextKey [byteSize8]uint64

// KeyGenerator generates key
// Not safe for concurrent usage
type KeyGenerator struct {
	buf []byte
}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{
		buf: make([]byte, 0, 1024),
	}
}

// Generate returns the ContextKey hash for the given parameters.
// The tags array is sorted in place to avoid heap allocations.
func (g *KeyGenerator) Generate(name, hostname string, tags []string) ContextKey {
	g.buf = g.buf[:0]

	// Sort the tags in place. For typical tag slices, we use
	// the in-place section sort to avoid heap allocations.
	// We default to stdlib's sort package for longer slices.
	if len(tags) < 20 {
		selectionSort(tags)
	} else {
		sort.Strings(tags)
	}

	g.buf = append(g.buf, name...)
	g.buf = append(g.buf, ',')
	for i := 0; i < len(tags); i++ {
		g.buf = append(g.buf, tags[i]...)
		g.buf = append(g.buf, ',')
	}
	g.buf = append(g.buf, hostname...)

	var hash ContextKey
	hash[0], hash[1] = murmur3.Sum128(g.buf)
	return hash
}

// Compare returns an integer comparing two strings lexicographically.
// The result will be 0 if a==b, -1 if a < b, and +1 if a > b.
func Compare(a, b ContextKey) int {
	for i := 0; i < byteSize8; i++ {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		default: // equality, compare next byte
			continue
		}
	}
	return 0
}

// IsZero returns true if the key is at zero value
func (k ContextKey) IsZero() bool {
	for _, b := range k {
		if b != 0 {
			return false
		}
	}
	return true
}
