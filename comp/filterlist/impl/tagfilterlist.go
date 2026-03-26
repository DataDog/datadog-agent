// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"slices"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
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
	exclude action = true
	include action = false
)

// hashedMetricTagList contains the list of tags hashed using murmur3.
type hashedMetricTagList struct {
	tags   []uint64
	action action
}

func NewEmptyTagMatcher() filterlist.TagMatcher {
	return tagMatcher{
		MetricTags: map[string]hashedMetricTagList{},
	}
}

func NewTagMatcher(metrics map[string]MetricTagList, log log.Component) filterlist.TagMatcher {
	return newTagMatcher(metrics, log)
}

// NewTagMatcher creates a new instance of TagMatcher. The function takes
// a list of metric names and tags. Those tags are hashed using murmur3.
// The hashed value is then used to query whether a tag should be removed
// from a given metric.
func newTagMatcher(metrics map[string]MetricTagList, log log.Component) tagMatcher {
	// Store a hashed version of the tag list since that will take up
	// less space and be faster to query.
	hashed := make(map[string]hashedMetricTagList, len(metrics))
	for k, v := range metrics {
		tags := make([]uint64, 0, len(v.Tags))
		for _, tag := range v.Tags {
			tags = append(tags, murmur3.StringSum64(tag))
		}

		var act action
		switch v.Action {
		case "include":
			act = include
		case "exclude":
			act = exclude
		case "":
			act = exclude
		default:
			log.Warnf("`metric_tag_filterlist.%s.action` configuration value %q should be either `include` or `exclude`. Defaulting to `exclude`.", k, v.Action)
			act = exclude
		}
		hashed[k] = hashedMetricTagList{
			tags:   tags,
			action: act,
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
func (m tagMatcher) ShouldStripTags(metricName string) (func(tag string) bool, bool) {
	tm, ok := m.MetricTags[metricName]
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		hashedTag := murmur3.StringSum64(tagName(tag))
		return slices.Contains(tm.tags, hashedTag) != bool(tm.action)
	}

	return keepTag, ok
}
