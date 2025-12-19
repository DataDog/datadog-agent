// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlist provides utilities for matching and filtering tags.
package filterlist

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/twmb/murmur3"
)

// TagMatcher manages removing tags from metrics with a given name.
type TagMatcher struct {
	Metrics map[string]HashedMetricTagList
}

// MetricTagList is for loading the data from the configuration.
type MetricTagList struct {
	Tags   []string `yaml:"tags"`
	Action string   `yaml:"action"`
}

type Action bool

const (
	Exclude Action = true
	Include Action = false
)

// HashedMetricTagList contains the list of tags hashed using murmur3.
type HashedMetricTagList struct {
	tags   []uint64
	action Action
}

// NewTagMatcher creates a new instance of TagMatcher. The function takes
// a list of metric names and tags. Those tags are hashed using murmur3.
// The hashed value is then used to query whether a tag should be removed
// from a given metric.
func NewTagMatcher(metrics map[string]MetricTagList) *TagMatcher {
	// Store a hashed version of the tag list since that will take up
	// less space and be faster to query.
	hashed := make(map[string]HashedMetricTagList, len(metrics))
	for k, v := range metrics {
		tags := make([]uint64, 0, len(v.Tags))
		for _, tag := range v.Tags {
			tags = append(tags, murmur3.StringSum64(tag))
		}

		var action Action
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
		hashed[k] = HashedMetricTagList{
			tags:   tags,
			action: action,
		}
	}

	return &TagMatcher{
		Metrics: hashed,
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

// KeepTagFunc is called to see if we should keep the given tag.
type KeepTagFunc = func(tag string) bool

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name. The returned tag list will be used to query
// the tag.
func (m *TagMatcher) ShouldStripTags(metricName string) (KeepTagFunc, bool) {
	tm, ok := m.Metrics[metricName]
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		hashedTag := murmur3.StringSum64(tagName(tag))
		return slices.Contains(tm.tags, hashedTag) != bool(tm.action)
	}

	return keepTag, ok
}
