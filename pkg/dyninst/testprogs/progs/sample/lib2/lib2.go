// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lib2 uses generics from lib and defines its own, exercising
// cross-package generic displacement in DWARF. When main calls lib2's generic
// functions with different shapes than lib2's own usage, the Go compiler
// generates shape functions in main's compile unit that belong to lib2.
package lib2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib"
)

// --- Generic types defined in lib2 ---

// Wrapper wraps a lib.Box and adds a label. Tests a generic type in one
// package that contains a generic type from another package.
type Wrapper[T any] struct {
	Label string
	Inner lib.Box[T]
}

// Unwrap returns the inner Box's value.
//
//go:noinline
func (w Wrapper[T]) Unwrap() T {
	return w.Inner.Get()
}

// SetInner replaces the inner Box value.
//
//go:noinline
func (w *Wrapper[T]) SetInner(val T) {
	w.Inner.Set(val)
}

// --- Generic functions in lib2 ---

// WrapInPair takes two values and puts them in a lib.Pair. Tests a generic
// function in lib2 that instantiates a generic type from lib.
//
//go:noinline
func WrapInPair[K, V any](k K, v V) lib.Pair[K, V] {
	return lib.Pair[K, V]{First: k, Second: v}
}

// BoxAndReduce creates a Box from each element, then reduces using lib.Reduce.
// The double-generic case: this is generic over T and R, it creates lib.Box[T]
// values internally and calls lib.Box[T].Get() on them, then uses lib.Reduce
// to fold the results. The compiler must generate shape functions for
// lib.Box[T].Get and lib.Reduce within lib2's (or the caller's) compile unit.
//
//go:noinline
func BoxAndReduce[T, R any](items []T, init R, f func(R, T) R) R {
	boxes := make([]lib.Box[T], len(items))
	for i, item := range items {
		boxes[i] = lib.Box[T]{Value: item}
	}
	// Call Get() on each box to unbox, then reduce.
	return lib.Reduce(items, init, f)
}

// SwapPair creates a lib.Pair and calls Swap on it. Tests calling a method
// on a generic type from lib within a generic function in lib2.
//
//go:noinline
func SwapPair[K, V any](k K, v V) lib.Pair[V, K] {
	p := lib.Pair[K, V]{First: k, Second: v}
	return p.Swap()
}

// --- Non-generic function using generics with concrete types ---
// This creates shape functions in lib2's own compile unit.

var sink any

//go:noinline
func UseGenericsWithFloat64() {
	// lib2 uses float64 shapes — main will use int/string shapes, ensuring
	// different shape functions for the same generic definitions.
	box := lib.Box[float64]{Value: 3.14}
	box.Set(2.72)
	sink = box.Get()

	w := Wrapper[float64]{Label: "pi", Inner: box}
	sink = w.Unwrap()

	p := WrapInPair(1.5, 2.5)
	sink = p.Swap()

	sink = BoxAndReduce([]float64{1.0, 2.0, 3.0}, 0.0, func(acc, x float64) float64 {
		return acc + x
	})

	sink = lib.Filter([]float64{1.1, 2.2, 3.3}, func(x float64) bool {
		return x > 2.0
	})

	sink = lib.Reduce([]float64{1.0, 2.0, 3.0}, "", func(acc string, x float64) string {
		return acc + fmt.Sprintf("%.0f", x)
	})
}
