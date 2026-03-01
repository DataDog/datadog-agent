// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlist defines a component to handle the metric and tag filterlist
// including any updates from RC.
package filterlist

type TagMatcher interface {
	// ShouldStripTags returns a closure that can be called to check if a tag should be
	// removed from the given name. The second return will be true if this metric name
	// has been configured to strip tags.
	ShouldStripTags(metricName string) (func(tag string) bool, bool)

	// GetTagNameFilter returns the tag name hash map and exclude flag for optimized filtering.
	// If the metric name has no filter rules, returns (nil, false, false).
	// Otherwise returns (tagNameHashes, exclude, true) where:
	// - tagNameHashes: map of tag name hashes to filter
	// - exclude: if true, remove tags in the map; if false, keep only tags in the map
	// - found: true if filter rules exist for this metric
	GetTagNameFilter(metricName string) (tagNameHashes map[uint64]struct{}, exclude bool, found bool)
}
