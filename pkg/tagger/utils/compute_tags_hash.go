// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

// ComputeTagsHash returns a hash of the supplied tags.
func ComputeTagsHash(tags []string) string {
	panic("not called")
}

// copyArray makes sure the tagger does not return internal slices
// that could be modified by others, by explicitly copying the slice
// contents to a new slice. As strings are references, the size of
// the new array is small enough.
func copyArray(source []string) []string {
	panic("not called")
}
