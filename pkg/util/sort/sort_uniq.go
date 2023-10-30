// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sort

import "sort"

// UniqInPlace sorts and remove duplicates from elements in place
// The returned slice is a subslice of elements
func UniqInPlace(elements []string) []string {
	if len(elements) < 2 {
		return elements
	}
	size := len(elements)
	if size <= InsertionSortThreshold {
		InsertionSort(elements)
	} else {
		// this will trigger an alloc because sorts uses interface{} internaly
		// which confuses the escape analysis
		sort.Strings(elements)
	}
	return uniqSorted(elements)
}

// uniqSorted remove duplicate elements from the given slice
// the given slice needs to be sorted
func uniqSorted(elements []string) []string {
	j := 0
	for i := 1; i < len(elements); i++ {
		if elements[j] == elements[i] {
			continue
		}
		j++
		elements[j] = elements[i]
	}
	return elements[:j+1]
}

// RemoveDuplicatesAndSort sorts and removes duplicates from a slice without doing it in place.
func RemoveDuplicatesAndSort(elements []string) []string {
	res := CopyArray(elements)
	res = UniqInPlace(res)
	// copying the array with exactly enough capacity should make it more resilient
	// against cases where `append` mutates the original array
	return CopyArray(res)
}

// CopyArray returns a copied array
func CopyArray[T any](array []T) []T {
	res := make([]T, len(array))
	copy(res, array)
	return res
}
