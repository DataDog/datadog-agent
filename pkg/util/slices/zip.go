// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package slices

import "iter"

// ZipIter combines the two iterators into a single iterator.
// The combined iterator stops when the first of either iterator stops.
func ZipIter[K any, V any](x iter.Seq[K], y iter.Seq[V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		pull, stop := iter.Pull(y)
		defer stop()
		for k := range x {
			v, ok := pull()
			if !ok || !yield(k, v) {
				return
			}
		}
	}
}
