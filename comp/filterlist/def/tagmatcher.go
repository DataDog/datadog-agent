// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlist defines a component to handle the metric and tag filterlist
// including any updates from RC.
package filterlist

import (
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// TagMatcher manages removing tags from metrics with a given name.
type TagMatcher interface {
	// ShouldStripTags returns the HashedMetricTagList for the given metric name,
	// or nil if the metric has not been configured to strip tags.
	// The second return value is true when the metric is configured to strip tags.
	ShouldStripTags(metricName string) (*tagset.HashedMetricTagList, bool)
}
