// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package atomic

import (
	"math"
	"sync/atomic"
)

// The atomic float64 is copied from: https://github.com/uber-go/atomic/blob/master/atomic.go#L267

// Float64 is an atomic wrapper around float64.
type Float64 struct {
	v uint64
}

// NewFloat creates a Float64.
func NewFloat(f float64) *Float64 {
	return &Float64{math.Float64bits(f)}
}

// Load atomically loads the wrapped value.
func (f *Float64) Load() float64 {
	return math.Float64frombits(atomic.LoadUint64(&f.v))
}

// Store atomically stores the passed value.
func (f *Float64) Store(s float64) {
	atomic.StoreUint64(&f.v, math.Float64bits(s))
}

// Swap atomically swaps the passed value and returns the old one.
func (f *Float64) Swap(s float64) (old float64) {
	return math.Float64frombits(atomic.SwapUint64(&f.v, math.Float64bits(s)))
}

// Add atomically adds to the wrapped float64 and returns the new value.
func (f *Float64) Add(s float64) float64 {
	for {
		old := f.Load()
		new := old + s
		if f.CAS(old, new) {
			return new
		}
	}
}

// Sub atomically subtracts from the wrapped float64 and returns the new value.
func (f *Float64) Sub(s float64) float64 {
	return f.Add(-s)
}

// CAS is an atomic compare-and-swap.
func (f *Float64) CAS(old, new float64) bool {
	return atomic.CompareAndSwapUint64(&f.v, math.Float64bits(old), math.Float64bits(new))
}
