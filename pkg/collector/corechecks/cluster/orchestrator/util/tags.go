// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package util provides utility functions and constants for the orchestrator collectors.
package util

// ImmutableTagsJoin merges tags from multiple lists, allocating a new resulting slice. It is important to use this
// function to join tags in order to make sure we don't mutate the original slices.
func ImmutableTagsJoin(tagLists ...[]string) []string {
	allTags := []string{}
	for _, tags := range tagLists {
		allTags = append(allTags, tags...)
	}
	if len(allTags) == 0 {
		return nil
	}
	return allTags
}
