// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

// FNV-1a constants for 64-bit hash.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// computeContextKey returns a 64-bit FNV-1a hash of the metric name and its
// tags. The hash is computed inline (zero allocations) to avoid GC pressure on
// the hot path (100K+ calls/s).
//
// Tags are hashed in their natural order. If two sources produce the same
// logical context with different tag orderings, they will generate different
// keys. This is benign: the sidecar just stores two context definitions
// instead of one.
func computeContextKey(name string, tags []string) uint64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= fnvPrime64
	}
	h ^= 0 // null separator between name and tags
	h *= fnvPrime64
	for _, t := range tags {
		for i := 0; i < len(t); i++ {
			h ^= uint64(t[i])
			h *= fnvPrime64
		}
		h ^= 0 // null separator between tags
		h *= fnvPrime64
	}
	return h
}
