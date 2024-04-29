// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

// ConcatenateTags is a fast way to concatenate multiple tag
// arrays in a single one.
func ConcatenateTags(slices ...[]string) []string {
	if len(slices) == 1 {
		return slices[0]
	}
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	result := make([]string, totalLen)
	var i int
	for _, s := range slices {
		i += copy(result[i:], s)
	}
	return result
}

// ConcatenateStringTags adds string tags to existing tag array
func ConcatenateStringTags(tags []string, extraTags ...string) []string {
	finalTags := make([]string, 0, len(tags)+len(extraTags))
	finalTags = append(finalTags, tags...)
	finalTags = append(finalTags, extraTags...)
	return finalTags
}
