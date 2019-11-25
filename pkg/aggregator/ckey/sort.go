// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package ckey

// selectionSort is a in-place sorting algorithm used as
// an alternative to the sort package to avoid heap allocations
// in the metric intake section. It is benchmarked faster than
// sort.Strings up to 20 elements
func selectionSort(array []string) {
	var min int
	var tmp string

	for i := 0; i < len(array); i++ {
		min = i
		for j := i + 1; j < len(array); j++ {
			if array[j] < array[min] {
				min = j
			}
		}

		tmp = array[i]
		array[i] = array[min]
		array[min] = tmp
	}
}
