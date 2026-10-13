// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "slices"

// ConcatenateTags is a fast way to concatenate multiple tag
// arrays in a single one.
func ConcatenateTags(tagSlices ...[]string) []string {
	if len(tagSlices) == 1 {
		return tagSlices[0]
	}
	var totalLen int
	for _, s := range tagSlices {
		totalLen += len(s)
	}
	result := make([]string, totalLen)
	var i int
	for _, s := range tagSlices {
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

// AppendUniqueTags adds string tags to an existing tag array, skipping values
// that the array already holds. It is meant for small tagsets: a payload that
// gathers tags from several sources can append a constant tagset without
// emitting it twice.
func AppendUniqueTags(tags []string, extraTags ...string) []string {
	if len(extraTags) == 0 {
		return tags
	}
	finalTags := make([]string, 0, len(tags)+len(extraTags))
	finalTags = append(finalTags, tags...)
	for _, extra := range extraTags {
		if !slices.Contains(finalTags, extra) {
			finalTags = append(finalTags, extra)
		}
	}
	return finalTags
}
