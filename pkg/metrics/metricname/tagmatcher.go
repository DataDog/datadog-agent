// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"slices"

	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/metrics/tagname"
)

// HashedMetricTagList is the list of tag keys configured to be stripped from
// (or kept on) a metric. Tag keys are stored hashed using murmur3, in the form
// the intake stores them in (see tagname.RuleKey).
type HashedMetricTagList struct {
	// sorted, for binary search.
	tags    []uint64
	exclude bool
}

// NewHashedMetricTagList creates a tag list from tag keys in the form the
// intake stores them in. If exclude is true, the listed tags are removed;
// otherwise only the listed tags are kept.
func NewHashedMetricTagList(exclude bool, tags []string) HashedMetricTagList {
	hashed := make([]uint64, 0, len(tags))
	for _, tag := range tags {
		hashed = append(hashed, murmur3.StringSum64(tag))
	}
	slices.Sort(hashed)
	hashed = slices.Compact(hashed)

	return HashedMetricTagList{
		tags:    hashed,
		exclude: exclude,
	}
}

// KeepTag returns true if the tag should be kept on the metric. The tag's key
// is compared in the form the intake stores it in, so `Pod Name:web` matches
// the key `pod_name`. Normalizing and hashing the key does not allocate.
func (l HashedMetricTagList) KeepTag(tag string) bool {
	_, found := slices.BinarySearch(l.tags, tagname.KeyHash(tag))
	return found != l.exclude
}

// TagMatcher manages removing tags from metrics with a given name. The zero
// value matches no metric.
type TagMatcher struct {
	metricTags map[string]HashedMetricTagList
}

// NewTagMatcher creates a TagMatcher from per-metric tag lists, keyed by
// metric names in the form the intake stores them in.
func NewTagMatcher(metricTags map[string]HashedMetricTagList) TagMatcher {
	return TagMatcher{
		metricTags: metricTags,
	}
}

// ShouldStripTags returns the tag list for the given metric name, and true if
// tags have been configured to be stripped from it. The name is compared in
// the form the intake stores it in, so `my-metric` matches a list for
// `my_metric`.
func (m TagMatcher) ShouldStripTags(metricName string) (HashedMetricTagList, bool) {
	// The common case: no rules, so nothing to normalize.
	if len(m.metricTags) == 0 {
		return HashedMetricTagList{}, false
	}
	// Names are almost always already normalized, and are then looked up
	// without a copy.
	if isNormalized(metricName) {
		tl, ok := m.metricTags[metricName]
		return tl, ok
	}
	var buf [MaxLength]byte
	name, ok := NormalizeAppend(buf[:0], metricName)
	if !ok {
		return HashedMetricTagList{}, false
	}
	// Indexing with a converted []byte does not allocate.
	tl, ok := m.metricTags[string(name)]
	return tl, ok
}

// Len returns the number of metrics with a configured tag list.
func (m TagMatcher) Len() int {
	return len(m.metricTags)
}
