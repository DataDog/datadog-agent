// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"github.com/cespare/xxhash"
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
	intb uint64
	//	h    hash.Hash64
	//	buf  []byte
	//	hashCache *hashCache
}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{
		//		h:         murmur3.New64(),
		//		buf:       make([]byte, binary.MaxVarintLen64),
		//		hashCache: newHashCache(1024),
	}
}

//type hashCache struct {
//	cache   map[string]uint64
//	maxSize int
//}
//
//func newHashCache(maxSize int) *hashCache {
//	return &hashCache{
//		cache:   make(map[string]uint64),
//		maxSize: maxSize,
//	}
//}
//
//func (c *hashCache) LoadOrStore(key string) uint64 {
//	if h, found := c.cache[key]; found {
//		return h
//	}
//	if len(c.cache) >= c.maxSize {
//		c.cache = make(map[string]uint64)
//	}
//	c.cache[key] = murmur3.StringSum64(key)
//	return c.cache[key]
//}

// Generate returns the ContextKey hash for the given parameters.
// The tags array is sorted in place to avoid heap allocations.
func (g *KeyGenerator) Generate(name, hostname string, tags []string) ContextKey {
	g.intb = 0xc6a4a7935bd1e995
	g.intb = g.intb ^ xxhash.Sum64String(name)
	g.intb = g.intb ^ xxhash.Sum64String(hostname)

	// no tags, avoid doing any math if there is no tags
	//	if len(tags) > 0 {
	//		for i := range g.buf { // reset every byte of this buffer
	//			g.buf[i] = 0
	//		}
	for i := range tags {
		//			intb = intb ^ g.hashCache.LoadOrStore(tags[i]) // NOTE(remy): we can maybe use a faster hash here (even if it has more collisions than murmur3)
		//                                                         // XXX(remy): it seems that using a cache, which itself uses some hashing for its map, is not making things better
		g.intb = g.intb ^ xxhash.Sum64String(tags[i])
	}
	//	}
	//
	//	binary.PutUvarint(g.buf, g.intb)
	//	g.h.Write([]byte(g.buf))
	//
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
