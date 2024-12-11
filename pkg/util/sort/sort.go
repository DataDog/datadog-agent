// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sort has sort functions used by Agent.
package sort

// InsertionSortThreshold is the slice size after which we should consider
// using the stdlib sort method instead of the InsertionSort implemented below.
const InsertionSortThreshold = 40

// InsertionSort sorts in-place the given elements, not doing any allocation.
// It is very efficient for on slices but if memory allocation is not an issue,
// consider using the stdlib `sort.Sort` method on slices having a size > InsertionSortThreshold.
// See `pkg/util/sort_benchmarks_note.md` for more details.
func InsertionSort(elements []string) {
	for i := 1; i < len(elements); i++ {
		temp := elements[i]
		j := i
		for j > 0 && temp <= elements[j-1] {
			elements[j] = elements[j-1]
			j--
		}
		elements[j] = temp
	}
}
