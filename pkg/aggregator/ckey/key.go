// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/cespare/xxhash"
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
// Note that benchmarks against fnv1a did not provide better performances (no inlining).
type ContextKey uint64

// KeyGenerator generates key
// Not safe for concurrent usage
type KeyGenerator struct {
	intb uint64
}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{}
}

// Generate returns the ContextKey hash for the given parameters.
// The tags array is sorted in place to avoid heap allocations.
func (g *KeyGenerator) Generate(name, hostname string, tags []string) ContextKey {

	// Ensure the list of tags is deduplicated the fastest way possible
	if len(tags) > util.InsertionSortThreshold {
		sort.Strings(tags)
		util.UniqSorted(tags)
	} else {
		util.DedupInPlace(tags)
	}

	g.intb = 0xc6a4a7935bd1e995
	g.intb = g.intb ^ xxhash.Sum64String(name)
	g.intb = g.intb ^ xxhash.Sum64String(hostname)
	for i := range tags {
		g.intb = g.intb ^ xxhash.Sum64String(tags[i])
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
