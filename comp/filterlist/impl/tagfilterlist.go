// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"slices"

	"github.com/twmb/murmur3"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/DataDog/datadog-agent/pkg/metrics/tagname"
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
	MetricName string   `mapstructure:"metric_name" yaml:"metric_name" json:"metric_name"`
	Action     string   `mapstructure:"action" yaml:"action" json:"action"`
	Tags       []string `mapstructure:"tags" yaml:"tags" json:"tags"`
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

func newHashedMetricTagList(action action, tags []uint64) hashedMetricTagList {
	// The tags must be sorted as we do a binary search to test membership.
	slices.Sort(tags)
	tags = slices.Compact(tags)

	return hashedMetricTagList{
		action: action,
		tags:   tags,
	}
}

func NewEmptyTagMatcher() filterlist.TagMatcher {
	return tagMatcher{
		MetricTags: map[string]hashedMetricTagList{},
	}
}

// NewTagMatcher creates a TagMatcher from a map of metric names to tag configuration.
func NewTagMatcher(metrics map[string]MetricTagList, log log.Component) filterlist.TagMatcher {
	return newTagMatcher(metrics, log)
}

func newTagMatcher(metrics map[string]MetricTagList, log log.Component) tagMatcher {
	entries := make([]MetricTagListEntry, 0, len(metrics))
	for name, list := range metrics {
		entries = append(entries, MetricTagListEntry{MetricName: name, Action: list.Action, Tags: list.Tags})
	}
	return compileTagFilterList(entries, log)
}

// compileTagFilterList compiles tag filter list entries, from any source, into
// a tagMatcher.
//
// The Agent sees metric names and tags as they were submitted, but the intake
// normalizes both on ingest, and users configure rules against what they see
// in Datadog. So metric names are compiled in their intake form (see
// pkg/metrics/metricname), and tag keys as the intake stores them (see
// pkg/metrics/tagname/SPEC.md), and ShouldStripTags normalizes the samples it is
// given the same way. Rules that are already normalized are unchanged.
//
// Entries naming the same metric, once normalized, are merged:
//   - same action: the tag lists are merged.
//   - different actions: only the exclude tags are kept.
//
// The result does not depend on the order of the entries.
func compileTagFilterList(entries []MetricTagListEntry, log log.Component) tagMatcher {
	type rule struct {
		action action
		keys   []string
	}
	rules := make(map[string]*rule, len(entries))

	var nameBuf [metricname.MaxLength]byte
	for _, entry := range entries {
		if entry.MetricName == "" {
			log.Warn("skipping metric_tag_filterlist entry with empty metric_name")
			continue
		}

		act := parseAction(entry, log)

		normalized, ok := metricname.NormalizeAppend(nameBuf[:0], entry.MetricName)
		if !ok {
			log.Warnf("skipping metric_tag_filterlist entry for metric %q: the Datadog intake rejects this metric name", entry.MetricName)
			continue
		}
		name := string(normalized)

		keys := make([]string, 0, len(entry.Tags))
		for _, tag := range entry.Tags {
			key, ok := tagname.RuleKey(tag)
			if !ok {
				log.Warnf("ignoring tag %q in metric_tag_filterlist entry for metric %q: tag keys cannot contain ':'", tag, entry.MetricName)
				continue
			}
			keys = append(keys, key)
		}

		current, exists := rules[name]
		switch {
		case !exists:
			rules[name] = &rule{action: act, keys: keys}
		case current.action == act:
			current.keys = append(current.keys, keys...)
		case act == exclude:
			log.Debugf("metric_tag_filterlist configures conflicting actions for metric %q, keeping exclude", name)
			*current = rule{action: exclude, keys: keys}
		default:
			log.Debugf("metric_tag_filterlist configures conflicting actions for metric %q, keeping exclude", name)
		}
	}

	hashed := make(map[string]hashedMetricTagList, len(rules))
	for name, r := range rules {
		hashed[name] = newHashedMetricTagList(r.action, hashTags(r.keys))
	}

	return tagMatcher{
		MetricTags: hashed,
	}
}

// parseAction returns the action of a tag filter list entry. An empty action
// means exclude.
func parseAction(entry MetricTagListEntry, log log.Component) action {
	switch entry.Action {
	case "include":
		return include
	case "exclude", "":
		return exclude
	default:
		log.Warnf("`metric_tag_filterlist.%s.action` configuration value %q should be either `include` or `exclude`. Defaulting to `exclude`.", entry.MetricName, entry.Action)
		return exclude
	}
}

func hashTags(tags []string) []uint64 {
	hashed := make([]uint64, 0, len(tags))
	for _, tag := range tags {
		hashed = append(hashed, murmur3.StringSum64(tag))
	}

	return hashed
}

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name. The returned function reports whether a tag
// should be kept.
//
// The metric name and the tag keys are compared in the form the intake stores
// them in, so a rule for `my_metric` and `env` applies to a sample named
// `my-metric` with the tag `Env:prod`.
func (m tagMatcher) ShouldStripTags(metricName string) (func(tag string) bool, bool) {
	// Checked first so that the common case of no rules at all costs nothing
	// more than it did before normalization.
	if len(m.MetricTags) == 0 {
		return nil, false
	}

	var nameBuf [metricname.MaxLength]byte
	name, ok := metricname.NormalizeAppend(nameBuf[:0], metricName)
	if !ok {
		return nil, false
	}
	// Indexing with a converted []byte does not allocate.
	tm, ok := m.MetricTags[string(name)]
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		// Key does not allocate for already-normalized ASCII tags, the
		// common case, and allocates once otherwise. Hashing a stack buffer
		// would not avoid it: murmur3's generic implementation makes its
		// input escape.
		hashedTag := murmur3.StringSum64(tagname.Key(tag))
		_, found := slices.BinarySearch(tm.tags, hashedTag)
		return found != bool(tm.action)
	}

	return keepTag, true
}
