// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

type ConnStatsBuffer = ConnectionBuffer[ConnectionStats]
type FailedConnStatsBuffer = ConnectionBuffer[FailedConnStats]

var NewConnStatsBuf = NewConnectionBuffer[ConnectionStats]
var NewFailedConnStatsBuf = NewConnectionBuffer[FailedConnStats]

// ConnectionBuffer encapsulates a resizing buffer for ConnectionStat objects
type ConnectionBuffer[T any] struct {
	buf           []T
	off           int
	minBufferSize int
}

// NewConnectionBuffer creates a ConnectionBuffer with initial size `size`.
func NewConnectionBuffer[T any](initSize, minSize int) *ConnectionBuffer[T] {
	return &ConnectionBuffer[T]{
		buf:           make([]T, initSize),
		minBufferSize: minSize,
	}
}

// Next returns the next `ConnectionStats` object available for writing.
// It will resize the internal buffer if necessary.
func (b *ConnectionBuffer[T]) Next() *T {
	if b.off >= len(b.buf) {
		b.buf = append(b.buf, *new(T))
	}
	c := &b.buf[b.off]
	b.off++
	return c
}

// Append slice to ConnectionBuffer
func (b *ConnectionBuffer[T]) Append(slice []T) {
	b.buf = append(b.buf[:b.off], slice...)
	b.off += len(slice)
}

// Reclaim captures the last n entries for usage again.
func (b *ConnectionBuffer[T]) Reclaim(n int) {
	b.off -= n
	if b.off < 0 {
		b.off = 0
	}
}

// Connections returns a slice of all the `ConnectionStats` objects returned via `Next`
// since the last `Reset`.
func (b *ConnectionBuffer[T]) Connections() []T {
	return b.buf[:b.off]
}

// Len returns the count of the number of written `ConnectionStats` objects since last `Reset`.
func (b *ConnectionBuffer[T]) Len() int {
	return b.off
}

// Capacity returns the current capacity of the buffer
func (b *ConnectionBuffer[T]) Capacity() int {
	return cap(b.buf)
}

// Reset returns the written object count back to zero. It may resize the internal buffer based on past usage.
func (b *ConnectionBuffer[T]) Reset() {
	// shrink buffer if less than half used
	half := cap(b.buf) / 2
	if b.off <= half && half >= b.minBufferSize {
		b.buf = make([]T, half)
		b.off = 0
		return
	}

	zero := *new(T)
	for i := 0; i < b.off; i++ {
		b.buf[i] = zero
	}
	b.off = 0
}
