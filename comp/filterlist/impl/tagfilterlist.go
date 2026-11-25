// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/DataDog/datadog-agent/pkg/metrics/tagname"
)

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

func NewEmptyTagMatcher() metricname.TagMatcher {
	return metricname.TagMatcher{}
}

// NewTagMatcher creates a TagMatcher from a map of metric names to tag
// configuration. See compileTagFilterList.
func NewTagMatcher(metrics map[string]MetricTagList, log log.Component) metricname.TagMatcher {
	entries := make([]MetricTagListEntry, 0, len(metrics))
	for name, list := range metrics {
		entries = append(entries, MetricTagListEntry{MetricName: name, Action: list.Action, Tags: list.Tags})
	}
	return compileTagFilterList(entries, log)
}

// compileTagFilterList compiles tag filter list entries, from any source, into
// a TagMatcher. Metric names and tag keys are normalized the way the intake
// stores them, and entries naming the same metric are then merged; see
// pkg/metrics/tagname/SPEC.md §10. The result does not depend on entry order.
//
// As in the intake, metric names keep their case and tag keys are lowercased:
// `My-Metric` compiles to `My_Metric`, and `Env` to `env`.
func compileTagFilterList(entries []MetricTagListEntry, log log.Component) metricname.TagMatcher {
	type rule struct {
		exclude bool
		keys    []string
		// For warnings only: the first configured name for this metric, and
		// whether any of its entries listed tags.
		firstName string
		hadTags   bool
	}
	rules := make(map[string]*rule, len(entries))

	var nameBuf [metricname.MaxLength]byte
	for _, entry := range entries {
		if entry.MetricName == "" {
			log.Warn("skipping metric_tag_filterlist entry with empty metric_name")
			continue
		}

		exclude := parseExclude(entry, log)

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
				log.Warnf("ignoring tag %q in metric_tag_filterlist entry for metric %q: tag keys cannot contain ':'; list the tag key only, without a value", tag, entry.MetricName)
				continue
			}
			keys = append(keys, key)
		}

		current, exists := rules[name]
		if !exists {
			rules[name] = &rule{exclude: exclude, keys: keys, firstName: entry.MetricName, hadTags: len(entry.Tags) > 0}
			continue
		}
		current.hadTags = current.hadTags || len(entry.Tags) > 0
		if current.exclude == exclude {
			current.keys = append(current.keys, keys...)
			continue
		}
		log.Warnf("metric_tag_filterlist entries %q and %q are for the same metric and configure both include and exclude; keeping only the exclude entries", current.firstName, entry.MetricName)
		if exclude {
			current.exclude, current.keys = true, keys
		}
	}

	hashed := make(map[string]metricname.HashedMetricTagList, len(rules))
	for name, r := range rules {
		if !r.exclude && len(r.keys) == 0 && r.hadTags {
			log.Warnf("metric_tag_filterlist include entry for metric %q has no usable tag keys left, so it removes every tag", r.firstName)
		}
		hashed[name] = metricname.NewHashedMetricTagList(r.exclude, r.keys)
	}

	return metricname.NewTagMatcher(hashed)
}

// parseExclude reports whether a tag filter list entry excludes its tags. An
// empty action means exclude.
func parseExclude(entry MetricTagListEntry, log log.Component) bool {
	switch entry.Action {
	case "include":
		return false
	case "exclude", "":
		return true
	default:
		log.Warnf("`metric_tag_filterlist.%s.action` configuration value %q should be either `include` or `exclude`. Defaulting to `exclude`.", entry.MetricName, entry.Action)
		return true
	}
}
