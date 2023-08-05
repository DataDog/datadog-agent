// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slice

type Chain[T any] struct {
	slices [][]T
}

func NewChain[T any](slices ...[]T) Chain[T] {
	return Chain[T]{slices: slices}
}

func (c Chain[T]) Len() int {
	l := 0
	for _, s := range c.slices {
		l += len(s)
	}

	return l
}

func (c Chain[T]) Iterate(f func(i int, v *T)) {
	o := 0
	for i := 0; i < len(c.slices); i++ {
		for j := 0; j < len(c.slices[i]); j++ {
			f(o+j, &c.slices[i][j])
		}

		o += len(c.slices[i])
	}
}

func (c Chain[T]) Get(i int) T {
	for _, s := range c.slices {
		if i < len(s) {
			return s[i]
		}

		i -= len(s)
	}

	panic("index out of range")
}
