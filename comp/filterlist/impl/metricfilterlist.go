// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"fmt"
	"strings"

	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// Field names of the object form of a metric_filterlist_prefix entry.
const (
	nameField         = "name"
	exceptPrefixField = "except_prefix"
	exceptExactField  = "except_exact"
)

// MetricFilterListEntry is a single metric_filterlist_prefix entry.
// Represents one prefix with either exact or prefix exceptions.
type MetricFilterListEntry struct {
	Name         string   `mapstructure:"name" yaml:"name" json:"name"`
	ExceptPrefix []string `mapstructure:"except_prefix" yaml:"except_prefix,omitempty" json:"except_prefix,omitempty"`
	ExceptExact  []string `mapstructure:"except_exact" yaml:"except_exact,omitempty" json:"except_exact,omitempty"`
}

// loadMetricFilterList reads the metric filterlist stored at `key` and compiles
// it into matcher rules. A malformed entry is reported and skipped rather than
// failing the whole list, so that one bad line does not silently disable the
// filtering of everything else.
func loadMetricFilterList(cfg config.Component, logger log.Component, key string) []metricname.Rule {
	raw := cfg.Get(key)
	if raw == nil {
		return nil
	}

	// A `[]string` comes from the environment variable parser, from remote
	// configuration and from the tests. The YAML and JSON loaders produce a
	// `[]interface{}` whose elements are either a string or a map.
	if names, ok := raw.([]string); ok {
		rules := make([]metricname.Rule, 0, len(names))
		for _, name := range names {
			rules = append(rules, metricname.Rule{Pattern: name})
		}
		return rules
	}

	entries, ok := raw.([]interface{})
	if !ok {
		logger.Errorf("ignoring %s: expected a list of metric names, got %T", key, raw)
		return nil
	}

	rules := make([]metricname.Rule, 0, len(entries))
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
func parseMetricFilterListEntry(entry interface{}) (metricname.Rule, error) {
	if name, ok := entry.(string); ok {
		return metricname.Rule{Pattern: name}, nil
	}

	// The YAML and JSON loaders do not agree on the key type of a map, and a
	// value coming through `agent config` has been round-tripped once more:
	// normalise instead of listing the map types.
	fields, err := cast.ToStringMapE(entry)
	if err != nil {
		return metricname.Rule{}, fmt.Errorf("expected a metric name or a %q object, got %T", nameField, entry)
	}

	for field := range fields {
		if field != nameField && field != exceptPrefixField && field != exceptExactField {
			return metricname.Rule{}, fmt.Errorf("unknown field %q, only %q, %q and %q are supported", field, nameField, exceptPrefixField, exceptExactField)
		}
	}

	name, err := cast.ToStringE(fields[nameField])
	if err != nil {
		return metricname.Rule{}, fmt.Errorf("invalid %q: %s", nameField, err)
	}
	if name == "" {
		return metricname.Rule{}, fmt.Errorf("missing %q", nameField)
	}

	var exceptPrefix, exceptExact []string
	if raw := fields[exceptPrefixField]; raw != nil {
		if exceptPrefix, err = cast.ToStringSliceE(raw); err != nil {
			return metricname.Rule{}, fmt.Errorf("invalid %q for %q: %s", exceptPrefixField, name, err)
		}
	}
	if raw := fields[exceptExactField]; raw != nil {
		if exceptExact, err = cast.ToStringSliceE(raw); err != nil {
			return metricname.Rule{}, fmt.Errorf("invalid %q for %q: %s", exceptExactField, name, err)
		}
	}

	return metricname.Rule{Pattern: name, Except: combineExceptions(exceptPrefix, exceptExact)}, nil
}

// ensurePrefixPattern returns pattern as a prefix pattern for the internal
// `*`-suffix convention `metricname` uses, adding the trailing marker if it
// is not already present. Every entry of metric_filterlist_prefix -- and
// every one of its except_prefix exceptions -- is a prefix regardless of
// that marker, per the RFC ("RFC - metric prefix filtering configuration"),
// so both the configuration file loader and the RC loader route their
// entries through this before compiling them.
func ensurePrefixPattern(pattern string) string {
	if strings.HasSuffix(pattern, metricname.PrefixSuffix) {
		return pattern
	}
	return pattern + metricname.PrefixSuffix
}

// combineExceptions builds the []string exceptions metricname.Rule expects,
// following its internal `*`-suffix convention, from the except_prefix and
// except_exact lists the metric_filterlist_prefix schema stores them as.
func combineExceptions(exceptPrefix, exceptExact []string) []string {
	if len(exceptPrefix) == 0 && len(exceptExact) == 0 {
		return nil
	}
	except := make([]string, 0, len(exceptPrefix)+len(exceptExact))
	for _, prefix := range exceptPrefix {
		except = append(except, ensurePrefixPattern(prefix))
	}
	except = append(except, exceptExact...)
	return except
}

// splitExceptions is the inverse of combineExceptions: it splits a rule's
// exceptions -- following metricname's internal `*`-suffix convention -- back
// into the except_prefix/except_exact lists the metric_filterlist_prefix
// schema stores them as, stripping the marker from each prefix exception.
func splitExceptions(except []string) (exceptPrefix, exceptExact []string) {
	for _, e := range except {
		if prefix, ok := strings.CutSuffix(e, metricname.PrefixSuffix); ok {
			exceptPrefix = append(exceptPrefix, prefix)
			continue
		}
		exceptExact = append(exceptExact, e)
	}
	return exceptPrefix, exceptExact
}

// metricFilterListEntries renders the rules back into the shape the
// configuration holds them in, so that setting the list from remote
// configuration keeps `agent config` readable and re-parseable. Every rule
// passed in belongs to metric_filterlist_prefix (see partitionMetricFilterRules
// in rc.go), so its pattern always carries the internal `*`-suffix marker;
// that marker is stripped from the rendered name, which never carries it in
// this schema.
func metricFilterListEntries(rules []metricname.Rule) []interface{} {
	entries := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		if len(rule.Except) == 0 {
			entries = append(entries, rule.Pattern)
			continue
		}
		exceptPrefix, exceptExact := splitExceptions(rule.Except)
		entry := map[string]interface{}{
			nameField: strings.TrimSuffix(rule.Pattern, metricname.PrefixSuffix),
		}
		if len(exceptPrefix) > 0 {
			entry[exceptPrefixField] = exceptPrefix
		}
		if len(exceptExact) > 0 {
			entry[exceptExactField] = exceptExact
		}
		entries = append(entries, entry)
	}
	return entries
}

// loadMetricFilterPrefixRules loads and normalizes metric_filterlist_prefix.
// Every entry is a prefix, whether or not it is written with a trailing `*`,
// and may carry the exceptions that only this key's schema supports (see
// core_schema.yaml): metric_filterlist itself stays a plain list of names so
// that a component which only understands that shape is unaffected by this
// key's object form.
func loadMetricFilterPrefixRules(cfg config.Component, logger log.Component) []metricname.Rule {
	rules := loadMetricFilterList(cfg, logger, "metric_filterlist_prefix")
	for i := range rules {
		rules[i].Pattern = ensurePrefixPattern(rules[i].Pattern)
	}
	// Every pattern above now carries `*`, so `hasStar` inside NormalizeEntries
	// is what makes the entry a prefix; matchPrefix would be redundant.
	return normalizeMetricRules("metric_filterlist_prefix", rules, false, logger)
}
