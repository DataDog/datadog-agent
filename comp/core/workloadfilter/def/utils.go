// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

// FlattenFilterSets flattens a slice of filter sets into a single slice.
func FlattenFilterSets[T ~int](
	filterSets [][]T, // Generic filter types
) []T {
	// Flatten the filter sets into a single slice
	flattened := make([]T, 0)
	for _, set := range filterSets {
		flattened = append(flattened, set...)
	}
	return flattened
}
