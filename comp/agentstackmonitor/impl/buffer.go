// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

// bufferSize gives a 10-minute observation window at the default 60s tick.
const bufferSize = 10

type ring[T any] struct {
	buf    [bufferSize]T
	idx    int
	filled int
}

func (r *ring[T]) push(v T) {
	r.buf[r.idx] = v
	r.idx = (r.idx + 1) % bufferSize
	if r.filled < bufferSize {
		r.filled++
	}
}

// values returns the buffered samples in chronological order.
func (r *ring[T]) values() []T {
	if r.filled == 0 {
		return nil
	}
	out := make([]T, r.filled)
	if r.filled < bufferSize {
		copy(out, r.buf[:r.filled])
		return out
	}
	copy(out, r.buf[r.idx:])
	copy(out[bufferSize-r.idx:], r.buf[:r.idx])
	return out
}

func (r *ring[T]) countMatching(match func(T) bool) int {
	n := 0
	for i := 0; i < r.filled; i++ {
		if match(r.buf[i]) {
			n++
		}
	}
	return n
}

func sumInt(r *ring[int32]) int32 {
	var s int32
	for i := 0; i < r.filled; i++ {
		s += r.buf[i]
	}
	return s
}
