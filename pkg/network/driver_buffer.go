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

func NewDriverBuffer(size int) *DriverBuffer {
	return &DriverBuffer{
		buf: make([]ConnectionStats, int(math.Max(float64(size), minBufferSize))),
	}
}

func (d *DriverBuffer) Next() *ConnectionStats {
	// double size of buffer if necessary
	if d.off >= len(d.buf) {
		d.buf = append(d.buf, make([]ConnectionStats, len(d.buf))...)
	}
	c := &d.buf[d.off]
	d.off++
	return c
}

func (d *DriverBuffer) Connections() []ConnectionStats {
	return d.buf[:d.off]
}

func (d *DriverBuffer) Len() int {
	return d.off
}

func (d *DriverBuffer) Reset() {
	// shrink buffer if less than half used
	half := len(d.buf) / 2
	if d.off <= half && half >= minBufferSize {
		d.buf = make([]ConnectionStats, half)
	}
	d.off = 0
}
