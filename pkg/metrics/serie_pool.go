// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "sync"

// seriePool recycles *Serie structs across flush cycles to reduce heap pressure
// in the hot TimeSampler flush path.  Each pooled Serie is returned with a
// pre-allocated Points slice of capacity 1, which covers the common single-point
// case (gauge, count, rate, counter, monotonic_count, set) without an extra
// allocation.
var seriePool = sync.Pool{
	New: func() interface{} {
		s := &Serie{}
		s.Points = make([]Point, 0, 1)
		return s
	},
}

// GetSerie returns a zeroed *Serie from the pool.
// The Points field is pre-allocated with capacity ≥ 1.
// Call PutSerie when the Serie is no longer referenced.
func GetSerie() *Serie {
	s := seriePool.Get().(*Serie)
	// Preserve the pre-allocated backing array, zero everything else.
	savedPoints := s.Points[:0]
	*s = Serie{}
	s.Points = savedPoints
	return s
}

// PutSerie resets s and returns it to the pool.
// The caller must not use s after this call, and must not call PutSerie
// on a Serie that is still referenced by a downstream consumer (serialiser,
// forwarder, etc.).
func PutSerie(s *Serie) {
	if s == nil {
		return
	}
	// Preserve the Points backing array so the next Get() can reuse it.
	savedPoints := s.Points[:0]
	*s = Serie{}
	s.Points = savedPoints
	seriePool.Put(s)
}
