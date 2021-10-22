// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"github.com/DataDog/datadog-agent/pkg/tagset"
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

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{
		hg: tagset.NewHashGenerator(),
	}
}

// KeyGenerator generates hash for the given name, hostname and tags.
// The tags don't have to be sorted and duplicated tags will be ignored while
// generating the hash.
// Not safe for concurrent usage.
type KeyGenerator struct {
	hg *tagset.HashGenerator
}

// Generate returns the ContextKey hash for the given parameters.
// tagsBuf is re-arranged in place and truncated to only contain unique tags.
func (g *KeyGenerator) Generate(name, hostname string, tagsBuf *tagset.HashingTagsAccumulator) ContextKey {
	hash := murmur3.StringSum64(name) ^ murmur3.StringSum64(hostname) ^ g.hg.Hash(tagsBuf)
	return ContextKey(hash)
}

// Equals returns whether the two context keys are equal or not.
func Equals(a, b ContextKey) bool {
	return a == b
}

// IsZero returns true if the key is at zero value
func (k ContextKey) IsZero() bool {
	return k == 0
}
