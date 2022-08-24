// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"sync"
)

const defaultBufferSize = 1e9 // 1M

// safeBuffer is a thread safe buffer implementation which acts like a rolling
// buffer based on the size of the internal slice.
type safeBuffer struct {
	mu  sync.RWMutex
	b   []byte
	off int
}

// newSafeBuffer returns a new safe buffer with a default rolling size.
func newSafeBuffer() *safeBuffer {
	return newSafeBufferWithSize(defaultBufferSize)
}

// newSafeBuffer returns a new safe buffer having the given size.
func newSafeBufferWithSize(size int) *safeBuffer {
	return &safeBuffer{b: make([]byte, size)}
}

func (sb *safeBuffer) Reset() {
	sb.mu.Lock()
	sb.off = 0
	sb.mu.Unlock()
}

func (sb *safeBuffer) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return string(sb.b[:sb.off])
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	n := len(p)
	if n >= len(sb.b) {
		// p is bigger than the whole buffer; we store only
		// the last len(sb.b) bytes
		sb.off = copy(sb.b, p[n-len(sb.b):])
		return n, nil
	}
	if n > len(sb.b)-sb.off {
		// shift to make space in the buffer
		copy(sb.b, sb.b[n-(len(sb.b)-sb.off):sb.off])
		sb.off = len(sb.b) - n
	}
	sb.off += copy(sb.b[sb.off:], p)
	return n, nil
}
