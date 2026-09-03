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
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
	"github.com/twmb/murmur3"
)

// TagMatcher manages removing tags from metrics with a given name.
//
// Both halves work in normalized names, the names the intake stores and displays
// and therefore the names users configure: MetricTags is keyed on the normalized
// metric name and holds hashes of normalized tag names, and ShouldStripTags
// normalizes the metric name and each tag name it is given before looking them
// up. See metricname, and note that metric names and tag names normalize by
// different rules.
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
	// Distinct configured names can normalize to the same one, and merging two
	// lists can bring the same name in twice; either way a duplicate carries no
	// information.
	tags = slices.Compact(tags)

	return hashedMetricTagList{
		action: action,
		tags:   tags,
	}
}

// hashTags hashes the given tag names, which must already be normalized (see
// normalizeTagNames).
func hashTags(tags []string) []uint64 {
	hashed := make([]uint64, 0, len(tags))
	for _, tag := range tags {
		hashed = append(hashed, murmur3.StringSum64(tag))
	}

	return hashed
}

// normalizeTagNames normalizes each configured tag name so it is in the name space
// the intake stores tag names in, which is the one ShouldStripTags compares in.
// Names the intake would drop outright can never match and are dropped here.
//
// A configured name is a name on its own, never a whole tag, so a trailing
// underscore is kept: an entry of `my_tag_` means the name stored as `my_tag_`.
// See hashTagName for the valueless tag that loses it.
func normalizeTagNames(names []string, log log.Component) []string {
	normalized := make([]string, 0, len(names))
	// Reuse this stack buffer for normalizing each name.
	var buf [metricname.MaxNormalizedTagLength]byte
	for _, name := range names {
		if metricname.IsNormalizedASCIITagName(name) {
			normalized = append(normalized, name)
			continue
		}

		key, ok := metricname.NormalizeTagNameAppend(buf[:0], name)
		if !ok {
			log.Warnf("metric_tag_filterlist: dropping tag %q that is not a storable tag name", name)
			continue
		}
		normalized = append(normalized, string(key))
	}
	return normalized
}

// hashTagName hashes the name of the given tag the way the configured names were
// hashed, normalizing it first. The Agent sees tags exactly as they were
// submitted, so a tag submitted as `My-Tag:value` has to be found under the
// `my-tag` entry the user configured. It reports false when the intake would drop
// the tag outright, in which case no configured name can match it.
//
// It takes the whole tag rather than just its name because one rule depends on
// what follows the name: the intake strips a trailing underscore from the end of
// the tag, so `my_tag_:value` keeps it and a valueless `my_tag_` does not.
//
// It never allocates: an already normalized ASCII name is hashed as given, and the
// rest are normalized into a stack buffer.
func hashTagName(tag string) (uint64, bool) {
	name := tagName(tag)
	// The name ends the tag when the tag carries no value, and only then is its
	// trailing underscore at the end of the tag.
	valueless := len(name) == len(tag)

	if metricname.IsNormalizedASCIITagName(name) &&
		!(valueless && name[len(name)-1] == '_') {
		return murmur3.StringSum64(name), true
	}

	var buf [metricname.MaxNormalizedTagLength]byte
	key, ok := metricname.NormalizeTagNameAppend(buf[:0], name)
	if !ok {
		return 0, false
	}
	if valueless && key[len(key)-1] == '_' {
		// Normalizing never leaves a run of underscores, so dropping one is
		// enough. It cannot empty the name either, since a normalized name
		// starts with a letter.
		key = key[:len(key)-1]
	}
	// murmur3.Sum64 and murmur3.StringSum64 hash the same bytes to the same
	// value, so this matches how the configured names were hashed.
	return murmur3.Sum64(key), true
}

// mergeHashedMetricTagLists reconciles two tag configurations that apply to the
// same metric name, following the same rules as duplicate configuration entries:
// tags are merged when both agree on the action, and an exclude list always wins
// over an include one.
func mergeHashedMetricTagLists(a, b hashedMetricTagList) hashedMetricTagList {
	if a.action == b.action {
		return newHashedMetricTagList(a.action, slices.Concat(a.tags, b.tags))
	}

	if a.action == exclude {
		return a
	}
	return b
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
//
// Metric and tag names are normalized, so an entry written as the metric and its
// tags appear in Datadog matches the raw names the Agent sees on the wire. Names
// the intake would reject outright are dropped, and two metric names that
// normalize to the same stored name have their configurations merged.
func newTagMatcher(metrics map[string]MetricTagList, log log.Component) tagMatcher {
	// Store a hashed version of the tag list since that will take up
	// less space and be faster to query.
	hashed := make(map[string]hashedMetricTagList, len(metrics))
	for k, v := range metrics {
		name, storable := normalizeMetricName(k)
		if !storable {
			log.Warnf("metric_tag_filterlist: dropping entry %q that is not a storable metric name", k)
			continue
		}

		tags := hashTags(normalizeTagNames(v.Tags, log))

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

		entry := newHashedMetricTagList(act, tags)
		if existing, dup := hashed[name]; dup {
			log.Debugf("metric_tag_filterlist configures conflicting tags for metric %v, which is stored as %v", k, name)
			entry = mergeHashedMetricTagLists(existing, entry)
		}
		hashed[name] = entry
	}

	return tagMatcher{
		MetricTags: hashed,
	}
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
// from the given metric name. The returned tag list will be used to query
// the tag.
func (m tagMatcher) ShouldStripTags(metricName string) (func(tag string) bool, bool) {
	tm, ok := m.lookup(metricName)
	if !ok {
		return nil, false
	}

	keepTag := func(tag string) bool {
		// A tag name the intake drops outright cannot have been configured, so it
		// is simply not in the list, which include and exclude read differently.
		found := false
		if hashedTag, ok := hashTagName(tag); ok {
			_, found = slices.BinarySearch(tm.tags, hashedTag)
		}
		keep := found != bool(tm.action)

		return keep
	}

	return keepTag, ok
}

// lookup returns the tag configuration for the given metric name.
//
// The name is normalized before being looked up, since MetricTags is keyed on
// the name the intake stores. The Agent sees names exactly as they were
// submitted, so a metric submitted as `my metric-name` has to be found under the
// `my_metric_name` entry the user configured. Names the intake would reject are
// never found.
//
// lookup never allocates: already normalized names are used as given, and the
// rest are normalized into a stack buffer.
func (m tagMatcher) lookup(metricName string) (hashedMetricTagList, bool) {
	if metricname.IsNormalized(metricName) {
		tm, ok := m.MetricTags[metricName]
		return tm, ok
	}

	var buf [metricname.MaxLength]byte
	key, ok := metricname.NormalizeAppend(buf[:0], metricName)
	if !ok {
		return hashedMetricTagList{}, false
	}

	// The compiler elides the copy for a map lookup on a []byte to string
	// conversion, so this does not allocate.
	tm, ok := m.MetricTags[string(key)]
	return tm, ok
}
