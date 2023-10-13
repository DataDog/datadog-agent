// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// removeDuplicatesAndSort returns a new array, sorted without duplicates.
func removeDuplicatesAndSort(elements []string) []string {
	// isolate unique elements
	found := make(map[string]bool)
	unique := []string{}

	for v := range elements {
		if !found[elements[v]] {
			unique = append(unique, elements[v])
			found[elements[v]] = true
		}
	}

	// sort the array
	sort.Strings(unique)

	// copying the array with exactly enough capacity should make it more resilient
	// against cases where `append` mutates the original array
	res := make([]string, len(unique))
	copy(res, unique)
	return res
}

// GetConfiguredTags returns list of tags from a configuration, based on
// `tags` (DD_TAGS) and `extra_tags“ (DD_EXTRA_TAGS), with `dogstatsd_tags` (DD_DOGSTATSD_TAGS)
// if includeDogdstatsd is true.
func GetConfiguredTags(c config.Reader, includeDogstatsd bool) []string {
	tags := c.GetStringSlice("tags")
	extraTags := c.GetStringSlice("extra_tags")

	var dsdTags []string
	if includeDogstatsd {
		dsdTags = c.GetStringSlice("dogstatsd_tags")
	}

	combined := make([]string, 0, len(tags)+len(extraTags)+len(dsdTags))
	combined = append(combined, tags...)
	combined = append(combined, extraTags...)
	combined = append(combined, dsdTags...)

	// The aggregator should sort and remove duplicates in place. Pre-sorting part of the tags should
	// improve the performances of the insertion sort in the aggregators.
	combined = removeDuplicatesAndSort(combined)
	return combined
}
