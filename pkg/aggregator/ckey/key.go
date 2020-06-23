// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ckey

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/twmb/murmur3"
)

// ContextKey is a non-cryptographic hash that allows to
// aggregate metrics from a same context together.
//
// This implementation has been designed to remove all heap
// allocations from the intake to reduce GC pressure on high
// volumes.
//
// Having int64/uint64 context keys mean that we will get better performances
// from the Go runtime while using them as map keys. This is thanks to the fast-path
// methods for map access and map assign with int64 keys.
// See for instance runtime.mapassign_fast64 or runtime.mapaccess2_fast64.
//
// Note that Agent <= 6.19.0 were using a 128 bits hash, we've switched
// to 64 bits for better performances (map access) and because 128 bits were overkill
// in the first place.
// Note that we've benchmarked against xxhash64 which should be slightly faster,
// but the Go compiler is not inlining xxhash sum methods whereas it is inlining
// the murmur3 implementation, providing better performances overall.
// Note that benchmarks against fnv1a did not provide better performances (no inlining).
type ContextKey uint64

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
	// the in-place insertion sort to avoid heap allocations.
	// We default to stdlib's sort package for longer slices.
	// See `pkg/util/sort.go` for info on the threshold.
	if len(tags) < util.InsertionSortThreshold {
		util.InsertionSort(tags)
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

	return ContextKey(murmur3.Sum64(g.buf))
}

// Equals returns whether the two context keys are equal or not.
func Equals(a, b ContextKey) bool {
	return a == b
}

// IsZero returns true if the key is at zero value
func (k ContextKey) IsZero() bool {
	return k == 0
}
