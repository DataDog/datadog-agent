// +build windows

package network

import "math"

const minBufferSize = 256

// DriverBuffer encapsulates a resizing buffer for providing ConnectionStat pointers
// to read from the windows driver
type DriverBuffer struct {
	buf []ConnectionStats
	off int
}

// NewDriverBuffer creates a DriverBuffer with initial size `size`.
func NewDriverBuffer(size int) *DriverBuffer {
	return &DriverBuffer{
		buf: make([]ConnectionStats, int(math.Max(float64(size), minBufferSize))),
	}
}

// Next returns the next `ConnectionStats` object available for writing.
// It will resize the internal buffer if necessary.
func (d *DriverBuffer) Next() *ConnectionStats {
	// double size of buffer if necessary
	if d.off >= len(d.buf) {
		d.buf = append(d.buf, make([]ConnectionStats, len(d.buf))...)
	}
	c := &d.buf[d.off]
	d.off++
	return c
}

// Reclaim captures the last n entries for usage again.
func (d *DriverBuffer) Reclaim(n int) {
	d.off -= n
	if d.off < 0 {
		d.off = 0
	}
}

// Connections returns a slice of all the `ConnectionStats` objects returned via `Next`
// since the last `Reset`.
func (d *DriverBuffer) Connections() []ConnectionStats {
	return d.buf[:d.off]
}

// Len returns the count of the number of written `ConnectionStats` objects since last `Reset`.
func (d *DriverBuffer) Len() int {
	return d.off
}

// Capacity returns the current capacity of the buffer
func (d *DriverBuffer) Capacity() int {
	return len(d.buf)
}

// Reset returns the written object count back to zero. It may resize the internal buffer based on past usage.
func (d *DriverBuffer) Reset() {
	// shrink buffer if less than half used
	half := len(d.buf) / 2
	if d.off <= half && half >= minBufferSize {
		d.buf = make([]ConnectionStats, half)
	}
	d.off = 0
}
