package util

import (
	"sort"
)

// SortUniqInPlace sorts and remove duplicates from elements in place
// The returned slice is a subslice of elements
func SortUniqInPlace(elements []string) []string {
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
	return UniqSorted(elements)
}

// DedupInPlace deduplicates the string re-using the given slice.
func DedupInPlace(elements []string) []string {
	if len(elements) < 2 {
		return elements
	}

	idx := 1
OUTER:
	for i := 1; i < len(elements); i++ {
		el := elements[i]

		for j := 0; j < idx; j++ {
			if el == elements[j] {
				continue OUTER
			}
		}

		elements[idx] = el
		idx++
	}

	return elements[:idx]
}

// UniqSorted removes duplicate elements from the given slice
// the given slice needs to be sorted
func UniqSorted(elements []string) []string {
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
