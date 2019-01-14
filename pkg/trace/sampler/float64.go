package sampler

import (
	"math"
	"sync/atomic"
)

// The atomic float64 is copied from: https://github.com/uber-go/atomic/blob/master/atomic.go#L267

// atomicFloat64 is an atomic wrapper around float64.
type atomicFloat64 struct {
	v uint64
}

// newFloat64 creates a atomicFloat64.
func newFloat64(f float64) *atomicFloat64 {
	return &atomicFloat64{math.Float64bits(f)}
}

// Load atomically loads the wrapped value.
func (f *atomicFloat64) Load() float64 {
	return math.Float64frombits(atomic.LoadUint64(&f.v))
}

// Store atomically stores the passed value.
func (f *atomicFloat64) Store(s float64) {
	atomic.StoreUint64(&f.v, math.Float64bits(s))
}

// Add atomically adds to the wrapped float64 and returns the new value.
func (f *atomicFloat64) Add(s float64) float64 {
	for {
		old := f.Load()
		new := old + s
		if f.CAS(old, new) {
			return new
		}
	}
}

// Sub atomically subtracts from the wrapped float64 and returns the new value.
func (f *atomicFloat64) Sub(s float64) float64 {
	return f.Add(-s)
}

// CAS is an atomic compare-and-swap.
func (f *atomicFloat64) CAS(old, new float64) bool {
	return atomic.CompareAndSwapUint64(&f.v, math.Float64bits(old), math.Float64bits(new))
}
