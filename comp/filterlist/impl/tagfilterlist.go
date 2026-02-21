// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"strings"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/twmb/murmur3"
)

// TagMatcher manages removing tags from metrics with a given name.
type tagMatcher struct {
	MetricTags map[string]hashedMetricTagList
}

// MetricTagList is for loading the data from the configuration.
type MetricTagList struct {
	Tags   []string `yaml:"tags"`
	Action string   `yaml:"action"`
}

// MetricTagListEntry is for loading the new list-based configuration format.
type MetricTagListEntry struct {
	MetricName string   `mapstructure:"metric_name"`
	Action     string   `mapstructure:"action"`
	Tags       []string `mapstructure:"tags"`
}

type action bool

const (
	Exclude action = true
	Include action = false
)

// HashedMetricTagList contains the list of tags hashed using murmur3.
type hashedMetricTagList struct {
	tags   []uint64            // kept for backwards compatibility if needed
	tagMap map[uint64]struct{} // O(1) lookup map
	action action
}

func NewEmptyTagMatcher() filterlist.TagMatcher {
	matcher := tagMatcher{
		MetricTags: map[string]hashedMetricTagList{},
	}
	return &matcher
}

func NewTagMatcher(metrics map[string]MetricTagList) filterlist.TagMatcher {
	matcher := newTagMatcher(metrics)
	return &matcher
}

// NewTagMatcher creates a new instance of TagMatcher. The function takes
// a list of metric names and tags. Those tags are hashed using murmur3.
// The hashed value is then used to query whether a tag should be removed
// from a given metric.
func newTagMatcher(metrics map[string]MetricTagList) tagMatcher {
	// Store a hashed version of the tag list since that will take up
	// less space and be faster to query.
	hashed := make(map[string]hashedMetricTagList, len(metrics))
	for k, v := range metrics {
		tags := make([]uint64, 0, len(v.Tags))
		tagMap := make(map[uint64]struct{}, len(v.Tags))
		for _, tag := range v.Tags {
			h := murmur3.StringSum64(tag)
			tags = append(tags, h)
			tagMap[h] = struct{}{}
		}

		var action action
		switch v.Action {
		case "include":
			action = Include
		case "exclude":
			action = Exclude
		case "":
			action = Exclude
		default:
			log.Warnf("`metric_tag_filterlist.%s.action` configuration should be either `include` or `exclude`. Defaulting to `exclude`.", v.Action)
			action = Exclude
		}
		hashed[k] = hashedMetricTagList{
			tags:   tags,
			tagMap: tagMap,
			action: action,
		}
	}

	return tagMatcher{
		MetricTags: hashed,
	}
}

// tagName extracts the tag name portion from the tag.
func tagName(tag string) string {
	tagNamePos := strings.Index(tag, ":")
	if tagNamePos < 0 {
		tagNamePos = len(tag)
	}

	return tag[:tagNamePos]
}

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name. The returned tag list will be used to query
// the tag.
func (m *tagMatcher) ShouldStripTags(metricName string) (func(tag string) bool, bool) {
	tm, ok := m.MetricTags[metricName]
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		hashedTag := murmur3.StringSum64(tagName(tag))
		_, found := tm.tagMap[hashedTag]
		return found != bool(tm.action)
	}

	return keepTag, ok
}

// GetTagNameFilter returns the tag name hash map and exclude flag for optimized filtering.
func (m *tagMatcher) GetTagNameFilter(metricName string) (map[uint64]struct{}, bool, bool) {
	tm, ok := m.MetricTags[metricName]
	if !ok {
		return nil, false, false
	}
	return tm.tagMap, bool(tm.action), true
}
