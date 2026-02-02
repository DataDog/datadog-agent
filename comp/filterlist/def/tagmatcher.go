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
}
