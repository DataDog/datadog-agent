// +build windows

package network

import "math"

const minBufferSize = 256

type driverBuffer struct {
	buf []ConnectionStats
	off int
}

func newDriverBuffer(size int) *driverBuffer {
	return &driverBuffer{
		buf: make([]ConnectionStats, int(math.Max(float64(size), minBufferSize))),
	}
}

func (d *driverBuffer) Next() *ConnectionStats {
	// double size of buffer if necessary
	if d.off >= len(d.buf) {
		d.buf = append(d.buf, make([]ConnectionStats, len(d.buf))...)
	}
	c := &d.buf[d.off]
	d.off++
	return c
}

func (d *driverBuffer) Connections() []ConnectionStats {
	return d.buf[:d.off]
}

func (d *driverBuffer) Reset() {
	// shrink buffer if less than half used
	half := len(d.buf) / 2
	if d.off <= half && half >= minBufferSize {
		d.buf = make([]ConnectionStats, half)
	}
	d.off = 0
}
