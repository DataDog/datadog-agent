// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"slices"
	"strings"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/twmb/murmur3"
)

// tagMatcher manages removing tags from metrics with a given name.
type tagMatcher struct {
	MetricTags map[string]tagset.HashedMetricTagList
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

func NewEmptyTagMatcher() filterlist.TagMatcher {
	matcher := tagMatcher{
		MetricTags: map[string]tagset.HashedMetricTagList{},
	}
	return &matcher
}

func NewTagMatcher(metrics map[string]MetricTagList) filterlist.TagMatcher {
	matcher := newTagMatcher(metrics)
	return &matcher
}

// newTagMatcher creates a new instance of tagMatcher. The function takes
// a list of metric names and tags. Those tags are hashed using murmur3.
// The hashed value is then used to query whether a tag should be removed
// from a given metric.
func newTagMatcher(metrics map[string]MetricTagList) tagMatcher {
	// Store a hashed version of the tag list since that will take up
	// less space and be faster to query.
	hashed := make(map[string]tagset.HashedMetricTagList, len(metrics))
	for k, v := range metrics {
		tags := make([]uint64, 0, len(v.Tags))
		for _, tag := range v.Tags {
			tags = append(tags, murmur3.StringSum64(tag))
		}

		// Sort the filter's name hashes so we can walk both lists with two pointers.
		slices.Sort(tags)

		var act tagset.Action
		switch v.Action {
		case "include":
			act = tagset.Include
		case "exclude":
			act = tagset.Exclude
		case "":
			act = tagset.Exclude
		default:
			log.Warnf("`metric_tag_filterlist.%s.action` configuration should be either `include` or `exclude`. Defaulting to `exclude`.", v.Action)
			act = tagset.Exclude
		}
		hashed[k] = tagset.HashedMetricTagList{
			Tags:   tags,
			Action: act,
		}
	}

	return tagMatcher{
		MetricTags: hashed,
	}
}

// tagName extracts the tag name portion from the tag.
func tagName(tag string) string {
	if i := strings.IndexByte(tag, ':'); i >= 0 {
		return tag[:i]
	}
	return tag
}

// ShouldStripTags returns the HashedMetricTagList for the given metric name,
// or nil if the metric has not been configured to strip tags.
func (m *tagMatcher) ShouldStripTags(metricName string) (*tagset.HashedMetricTagList, bool) {
	tm, ok := m.MetricTags[metricName]
	if !ok {
		return nil, false
	}
	return &tm, true
}
