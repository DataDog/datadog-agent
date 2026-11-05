// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package slices

import "iter"

// ZipIter combines the two iterators into a single iterator.
// The combined iterator stops only when both iterators are stopped.
func ZipIter[K any, V any](x iter.Seq[K], y iter.Seq[V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		xpull, xstop := iter.Pull(x)
		defer xstop()
		ypull, ystop := iter.Pull(y)
		defer ystop()
		for {
			k, okx := xpull()
			v, oky := ypull()
			if !okx && !oky {
				return
			}
			if !yield(k, v) {
				return
			}
		}
	}
}
