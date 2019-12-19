package util

import "sort"

const insertionSortThreshold = 20

// SortUniqInPlace sorts and remove duplicates from src and puts the result in dst
// The returned slice is a subslice of dst
func SortUniqInPlace(elements []string) []string {
	if len(elements) < 2 {
		return elements
	}
	size := len(elements)
	if size <= insertionSortThreshold {
		insertionSort(elements)
	} else {
		// this will trigger an alloc because sorts uses interface{} internaly
		// which confuses the escape analysis
		sort.Strings(elements)
	}
	return uniqSorted(elements)
}

func insertionSort(elements []string) {
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
