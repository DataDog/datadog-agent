// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"fmt"

	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// Field names of the object form of a metric filterlist entry.
const (
	metricNameField = "metric_name"
	exceptField     = "except"
)

// MetricFilterListEntry is the object form of a metric filterlist entry: a
// metric name, which is a prefix when it ends with `*`, and the exceptions
// that are kept even though the name matches.
//
// An entry that has no exception is written as a plain string instead, which is
// how the majority of a filterlist looks; both forms can be mixed in the same
// list.
type MetricFilterListEntry struct {
	MetricName string   `mapstructure:"metric_name" yaml:"metric_name" json:"metric_name"`
	Except     []string `mapstructure:"except" yaml:"except" json:"except,omitempty"`
}

// loadMetricFilterList reads the metric filterlist stored at `key` and compiles
// it into matcher rules. A malformed entry is reported and skipped rather than
// failing the whole list, so that one bad line does not silently disable the
// filtering of everything else.
func loadMetricFilterList(cfg config.Component, logger log.Component, key string) []utilstrings.Rule {
	raw := cfg.Get(key)
	if raw == nil {
		return nil
	}

	// A `[]string` comes from the environment variable parser, from remote
	// configuration and from the tests. The YAML and JSON loaders produce a
	// `[]interface{}` whose elements are either a string or a map.
	if names, ok := raw.([]string); ok {
		rules := make([]utilstrings.Rule, 0, len(names))
		for _, name := range names {
			rules = append(rules, utilstrings.Rule{Pattern: name})
		}
		return rules
	}

	entries, ok := raw.([]interface{})
	if !ok {
		logger.Errorf("ignoring %s: expected a list of metric names, got %T", key, raw)
		return nil
	}

	rules := make([]utilstrings.Rule, 0, len(entries))
	for i, entry := range entries {
		rule, err := parseMetricFilterListEntry(entry)
		if err != nil {
			logger.Errorf("ignoring %s entry %d: %s", key, i, err)
			continue
		}
		rules = append(rules, rule)
	}

	return rules
}

// parseMetricFilterListEntry reads one entry of a metric filterlist, in either
// its plain metric name or its object form.
func parseMetricFilterListEntry(entry interface{}) (utilstrings.Rule, error) {
	if name, ok := entry.(string); ok {
		return utilstrings.Rule{Pattern: name}, nil
	}

	// The YAML and JSON loaders do not agree on the key type of a map, and a
	// value coming through `agent config` has been round-tripped once more:
	// normalise instead of listing the map types.
	fields, err := cast.ToStringMapE(entry)
	if err != nil {
		return utilstrings.Rule{}, fmt.Errorf("expected a metric name or a %q object, got %T", metricNameField, entry)
	}

	for field := range fields {
		if field != metricNameField && field != exceptField {
			return utilstrings.Rule{}, fmt.Errorf("unknown field %q, only %q and %q are supported", field, metricNameField, exceptField)
		}
	}

	name, err := cast.ToStringE(fields[metricNameField])
	if err != nil {
		return utilstrings.Rule{}, fmt.Errorf("invalid %q: %s", metricNameField, err)
	}
	if name == "" {
		return utilstrings.Rule{}, fmt.Errorf("missing %q", metricNameField)
	}

	var except []string
	if raw := fields[exceptField]; raw != nil {
		if except, err = cast.ToStringSliceE(raw); err != nil {
			return utilstrings.Rule{}, fmt.Errorf("invalid %q for %q: %s", exceptField, name, err)
		}
	}

	return utilstrings.Rule{Pattern: name, Except: except}, nil
}

// metricFilterListEntries renders the rules back into the shape the
// configuration holds them in, so that setting the list from remote
// configuration keeps `agent config` readable and re-parseable.
func metricFilterListEntries(rules []utilstrings.Rule) []interface{} {
	entries := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		if len(rule.Except) == 0 {
			entries = append(entries, rule.Pattern)
			continue
		}
		entries = append(entries, map[string]interface{}{
			metricNameField: rule.Pattern,
			exceptField:     rule.Except,
		})
	}
	return entries
}
