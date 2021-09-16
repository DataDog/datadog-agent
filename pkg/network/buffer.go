package network

import "math"

const minBufferSize = 256

// Buffer encapsulates a resizing buffer for providing ConnectionStat pointers
// to read from the windows driver
type Buffer struct {
	buf []ConnectionStats
	off int
}

// NewBuffer creates a Buffer with initial size `size`.
func NewBuffer(size int) *Buffer {
	return &Buffer{
		buf: make([]ConnectionStats, int(math.Max(float64(size), minBufferSize))),
	}
}

// Next returns the next `ConnectionStats` object available for writing.
// It will resize the internal buffer if necessary.
func (b *Buffer) Next() *ConnectionStats {
	if b.off >= len(b.buf) {
		b.buf = append(b.buf, ConnectionStats{})
	}
	c := &b.buf[b.off]
	b.off++
	return c
}

// Append slice to Buffer
func (b *Buffer) Append(slice []ConnectionStats) {
	b.buf = append(b.buf[:b.off], slice...)
	b.off += len(slice)
}

// Reclaim captures the last n entries for usage again.
func (b *Buffer) Reclaim(n int) {
	b.off -= n
	if b.off < 0 {
		b.off = 0
	}
}

// Connections returns a slice of all the `ConnectionStats` objects returned via `Next`
// since the last `Reset`.
func (b *Buffer) Connections() []ConnectionStats {
	return b.buf[:b.off]
}

// Len returns the count of the number of written `ConnectionStats` objects since last `Reset`.
func (b *Buffer) Len() int {
	return b.off
}

// Capacity returns the current capacity of the buffer
func (b *Buffer) Capacity() int {
	return cap(b.buf)
}

// Reset returns the written object count back to zero. It may resize the internal buffer based on past usage.
func (b *Buffer) Reset() {
	// shrink buffer if less than half used
	half := cap(b.buf) / 2
	if b.off <= half && half >= minBufferSize {
		b.buf = make([]ConnectionStats, half)
	}
	b.off = 0
}
