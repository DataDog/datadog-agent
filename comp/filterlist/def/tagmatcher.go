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

	// ShouldStripTagsByNameHash is a faster variant of ShouldStripTags that operates on
	// pre-computed xxh3 hashes of tag names instead of raw tag strings. The returned
	// closure performs only a binary search with no string operations. The second return
	// value is true when this metric has a filter configured.
	ShouldStripTagsByNameHash(metricName string) (func(nameHash uint64) bool, bool)
}
