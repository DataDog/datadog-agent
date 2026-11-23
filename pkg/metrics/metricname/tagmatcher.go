// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"slices"
	"strings"

	"github.com/twmb/murmur3"
)

// HashedMetricTagList is the list of tag names configured to be stripped from
// (or kept on) a metric. Tag names are stored hashed using murmur3.
type HashedMetricTagList struct {
	// sorted, for binary search.
	tags    []uint64
	exclude bool
}

// NewHashedMetricTagList creates a tag list. If exclude is true, the listed
// tags are removed; otherwise only the listed tags are kept.
func NewHashedMetricTagList(exclude bool, tags []string) HashedMetricTagList {
	hashed := make([]uint64, 0, len(tags))
	for _, tag := range tags {
		hashed = append(hashed, murmur3.StringSum64(tag))
	}
	slices.Sort(hashed)

	return HashedMetricTagList{
		tags:    hashed,
		exclude: exclude,
	}
}

// KeepTag returns true if the tag should be kept on the metric.
func (l HashedMetricTagList) KeepTag(tag string) bool {
	_, found := slices.BinarySearch(l.tags, murmur3.StringSum64(tagName(tag)))
	return found != l.exclude
}

// tagName extracts the tag name portion from the tag.
func tagName(tag string) string {
	tagNamePos := strings.IndexByte(tag, ':')
	if tagNamePos < 0 {
		tagNamePos = len(tag)
	}

	return tag[:tagNamePos]
}

// TagMatcher manages removing tags from metrics with a given name. The zero
// value matches no metric.
type TagMatcher struct {
	metricTags map[string]HashedMetricTagList
}

// NewTagMatcher creates a TagMatcher from per-metric tag lists.
func NewTagMatcher(metricTags map[string]HashedMetricTagList) TagMatcher {
	return TagMatcher{
		metricTags: metricTags,
	}
}

// ShouldStripTags returns the tag list for the given metric name, and true if
// tags have been configured to be stripped from it.
func (m TagMatcher) ShouldStripTags(metricName string) (HashedMetricTagList, bool) {
	tl, ok := m.metricTags[metricName]
	return tl, ok
}

// Len returns the number of metrics with a configured tag list.
func (m TagMatcher) Len() int {
	return len(m.metricTags)
}
