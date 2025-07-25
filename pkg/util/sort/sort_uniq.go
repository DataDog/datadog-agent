// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sort

import (
	"sort"

	"unique"
)

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
func uniqSorted[T comparable](elements []T) []T {
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

// UniqInPlace2 sorts and deduplicates a list of interned strings
func UniqInPlace2(elements []unique.Handle[string]) []unique.Handle[string] {
	if len(elements) < 2 {
		return elements
	}
	sort.Slice(elements, func(i, j int) bool { return elements[i].Value() < elements[j].Value() })
	return uniqSorted(elements)
}
