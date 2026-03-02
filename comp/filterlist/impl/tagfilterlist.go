// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"regexp"
	"slices"
	"strings"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
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

// hashedMetricTagList contains a compiled regex that matches configured tag names.
type hashedMetricTagList struct {
	tagRegex *regexp.Regexp
	action   action
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

// newTagMatcher creates a new instance of TagMatcher. The function takes
// a list of metric names and tags. Those tags are compiled into a regex
// used to match incoming tag names.
func newTagMatcher(metrics map[string]MetricTagList) tagMatcher {
	hashed := make(map[string]hashedMetricTagList, len(metrics))
	for k, v := range metrics {
		var act action
		switch v.Action {
		case "include":
			act = Include
		case "exclude":
			act = Exclude
		case "":
			act = Exclude
		default:
			log.Warnf("`metric_tag_filterlist.%s.action` configuration should be either `include` or `exclude`. Defaulting to `exclude`.", v.Action)
			act = Exclude
		}
		hashed[k] = hashedMetricTagList{
			tagRegex: buildTagRegex(v.Tags),
			action:   act,
		}
	}

	return tagMatcher{
		MetricTags: hashed,
	}
}

// buildTagRegex compiles a regex that matches any of the given tag names exactly,
// with common prefixes factored out for efficiency.
// Returns nil if the list is empty.
func buildTagRegex(tags []string) *regexp.Regexp {
	if len(tags) == 0 {
		return nil
	}
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	slices.Sort(sorted)
	return regexp.MustCompile(`^` + prefixAlternation(sorted) + `$`)
}

// prefixAlternation builds a regex alternation pattern that matches exactly the
// given sorted words, factoring out common leading characters into shared prefixes.
// For example ["env", "error", "host", "http"] → "(?:e(?:nv|rror)|h(?:ost|ttp))".
func prefixAlternation(words []string) string {
	// Deduplicate consecutive equal words (input is sorted).
	deduped := words[:0:0]
	for i, w := range words {
		if i == 0 || w != words[i-1] {
			deduped = append(deduped, w)
		}
	}
	words = deduped

	if len(words) == 0 {
		return ""
	}

	// An empty string means this position is a terminal node — the parent
	// prefix alone is a valid match.
	hasEmpty := words[0] == ""
	if hasEmpty {
		words = words[1:]
	}
	if len(words) == 0 {
		return ""
	}

	// Group consecutive words by their first character.
	// Because input is sorted, words sharing a first character are adjacent.
	type group struct {
		char     byte
		suffixes []string
	}
	var groups []group
	for _, w := range words {
		c := w[0]
		if len(groups) > 0 && groups[len(groups)-1].char == c {
			groups[len(groups)-1].suffixes = append(groups[len(groups)-1].suffixes, w[1:])
		} else {
			groups = append(groups, group{char: c, suffixes: []string{w[1:]}})
		}
	}

	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		inner := prefixAlternation(g.suffixes)
		parts = append(parts, regexp.QuoteMeta(string(g.char))+inner)
	}

	var result string
	isBareGroup := false
	if len(parts) == 1 {
		result = parts[0]
	} else {
		result = "(?:" + strings.Join(parts, "|") + ")"
		isBareGroup = true
	}

	if hasEmpty {
		if isBareGroup {
			result += "?"
		} else {
			result = "(?:" + result + ")?"
		}
	}

	return result
}

// tagName extracts the tag name portion from the tag.
func tagName(tag string) string {
	tagNamePos := strings.IndexByte(tag, ':')
	if tagNamePos < 0 {
		tagNamePos = len(tag)
	}

	return tag[:tagNamePos]
}

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name. The returned function reports whether a
// given tag should be kept.
func (m *tagMatcher) ShouldStripTags(metricName string) (func(tag string) bool, bool) {
	tm, ok := m.MetricTags[metricName]
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		matched := tm.tagRegex != nil && tm.tagRegex.MatchString(tagName(tag))
		return matched != bool(tm.action)
	}

	return keepTag, ok
}
