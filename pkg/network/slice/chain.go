// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package slice implements helpers for slice operations
package slice

// Chain is a series of slices of type T that have been "chained"
// together, i.e. they can be iterated on as one slice
type Chain[T any] struct {
	slices [][]T
}

// NewChain returns a new chain composed of the passed
// in slices
func NewChain[T any](slices ...[]T) Chain[T] {
	return Chain[T]{slices: slices}
}

// Len returns the sum of the lengths of the chained slices
func (c Chain[T]) Len() int {
	l := 0
	for _, s := range c.slices {
		l += len(s)
	}

	return l
}

// Iterate iterates over the chained slices in order calling function `f` on each item
func (c Chain[T]) Iterate(f func(i int, v *T)) {
	o := 0
	for i := 0; i < len(c.slices); i++ {
		for j := 0; j < len(c.slices[i]); j++ {
			f(o+j, &c.slices[i][j])
		}

		o += len(c.slices[i])
	}
}

// Get returns a slice item at index `i`, where 0 <= `i` < c.Len()
func (c Chain[T]) Get(i int) T {
	for _, s := range c.slices {
		if i < len(s) {
			return s[i]
		}

		i -= len(s)
	}

	panic("index out of range")
}
