// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// MetricMatch holds the resolved Datadog name and optional type override for a matched metric.
type MetricMatch struct {
	Name string // Datadog metric name (after rename)
	Type string // Type override ("counter", "gauge", etc.), or "" for native/auto-detect
}

// regexMetaChars contains characters that indicate a string is a regex pattern
// rather than a plain metric name.
const regexMetaChars = `\.+*?()|[]{}^$`

// exactInclude stores an exact-match include entry.
type exactInclude struct {
	match MetricMatch
}

// regexInclude stores a compiled regex include entry.
type regexInclude struct {
	pattern *regexp.Regexp
	match   MetricMatch
}

// MetricFilter implements metric include/exclude filtering for the OpenMetrics scraper.
type MetricFilter struct {
	// Include rules
	exactIncludes map[string]exactInclude
	regexIncludes []regexInclude
	matchAll      bool // true when ".*" or "*" is in the include list

	// Exclude rules
	exactExcludes map[string]struct{}
	regexExcludes []*regexp.Regexp

	// Exclude by labels
	excludeByLabels map[string][]string // label_name → values ("*" means any)

	// Wildcard cache: raw metric name → (MetricMatch, matched bool)
	cacheWildcards bool
	cacheMu        sync.RWMutex
	cache          map[string]cachedResult
}

type cachedResult struct {
	match   MetricMatch
	matched bool
}

// NewMetricFilter builds a MetricFilter from config fields.
func NewMetricFilter(metrics []interface{}, extraMetrics []interface{}, excludeMetrics []string, excludeByLabels map[string][]string, cacheWildcards bool) (*MetricFilter, error) {
	f := &MetricFilter{
		exactIncludes:   make(map[string]exactInclude),
		exactExcludes:   make(map[string]struct{}),
		excludeByLabels: excludeByLabels,
		cacheWildcards:  cacheWildcards,
	}
	if cacheWildcards {
		f.cache = make(map[string]cachedResult)
	}

	// Parse include entries from both metrics and extra_metrics.
	allMetrics := make([]interface{}, 0, len(metrics)+len(extraMetrics))
	allMetrics = append(allMetrics, metrics...)
	allMetrics = append(allMetrics, extraMetrics...)

	for _, entry := range allMetrics {
		if err := f.parseIncludeEntry(entry); err != nil {
			return nil, err
		}
	}

	// Parse exclude entries.
	for _, pattern := range excludeMetrics {
		if isRegexPattern(pattern) {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid exclude_metrics regex %q: %w", pattern, err)
			}
			f.regexExcludes = append(f.regexExcludes, re)
		} else {
			f.exactExcludes[pattern] = struct{}{}
		}
	}

	return f, nil
}

// parseIncludeEntry processes a single entry from the metrics config list.
func (f *MetricFilter) parseIncludeEntry(entry interface{}) error {
	switch v := entry.(type) {
	case string:
		return f.parseStringInclude(v)
	case map[interface{}]interface{}:
		return f.parseMapInclude(v)
	case map[string]interface{}:
		return f.parseStringKeyMapInclude(v)
	default:
		return fmt.Errorf("unsupported metrics entry type %T", entry)
	}
}

// parseStringInclude handles a string entry in the metrics list.
func (f *MetricFilter) parseStringInclude(pattern string) error {
	// Special wildcard patterns.
	if pattern == ".*" || pattern == "*" {
		f.matchAll = true
		return nil
	}

	if isRegexPattern(pattern) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid metrics regex %q: %w", pattern, err)
		}
		f.regexIncludes = append(f.regexIncludes, regexInclude{
			pattern: re,
			match:   MetricMatch{Type: "native"},
		})
	} else {
		f.exactIncludes[pattern] = exactInclude{
			match: MetricMatch{Name: pattern, Type: "native"},
		}
	}
	return nil
}

// parseMapInclude handles a map entry in the metrics list.
// Key is the raw Prometheus metric name.
// Value is either:
//   - a string: the Datadog metric name
//   - a map with "name" and optionally "type" keys
func (f *MetricFilter) parseMapInclude(m map[interface{}]interface{}) error {
	for rawKey, rawValue := range m {
		key, ok := rawKey.(string)
		if !ok {
			return fmt.Errorf("metrics map key must be a string, got %T", rawKey)
		}

		match, err := parseMapValue(key, rawValue)
		if err != nil {
			return err
		}

		f.exactIncludes[key] = exactInclude{match: match}
	}
	return nil
}

// parseStringKeyMapInclude handles a map[string]interface{} entry (some YAML
// decoders produce this type instead of map[interface{}]interface{}).
func (f *MetricFilter) parseStringKeyMapInclude(m map[string]interface{}) error {
	for key, rawValue := range m {
		match, err := parseMapValue(key, rawValue)
		if err != nil {
			return err
		}

		f.exactIncludes[key] = exactInclude{match: match}
	}
	return nil
}

// parseMapValue resolves the value side of a metrics map entry into a MetricMatch.
func parseMapValue(key string, rawValue interface{}) (MetricMatch, error) {
	switch val := rawValue.(type) {
	case string:
		return MetricMatch{Name: val, Type: "native"}, nil

	case map[interface{}]interface{}:
		return parseNestedMap(key, val)

	case map[string]interface{}:
		converted := make(map[interface{}]interface{}, len(val))
		for k, v := range val {
			converted[k] = v
		}
		return parseNestedMap(key, converted)

	default:
		return MetricMatch{}, fmt.Errorf("unsupported value type %T for metric %q", rawValue, key)
	}
}

// parseNestedMap extracts "name" and "type" from a nested map value.
func parseNestedMap(key string, m map[interface{}]interface{}) (MetricMatch, error) {
	match := MetricMatch{Type: "native"}

	if nameVal, ok := m["name"]; ok {
		name, ok := nameVal.(string)
		if !ok {
			return MetricMatch{}, fmt.Errorf("metric %q: \"name\" must be a string, got %T", key, nameVal)
		}
		match.Name = name
	} else {
		match.Name = key
	}

	if typeVal, ok := m["type"]; ok {
		typ, ok := typeVal.(string)
		if !ok {
			return MetricMatch{}, fmt.Errorf("metric %q: \"type\" must be a string, got %T", key, typeVal)
		}
		match.Type = typ
	}

	return match, nil
}

// MatchMetric checks if a raw Prometheus metric name should be collected.
// Returns the MetricMatch and true if it should be collected, or false if not.
func (f *MetricFilter) MatchMetric(rawName string) (MetricMatch, bool) {
	// Check excludes first.
	if f.isExcluded(rawName) {
		return MetricMatch{}, false
	}

	// Check exact includes.
	if inc, ok := f.exactIncludes[rawName]; ok {
		return inc.match, true
	}

	// Check match-all wildcard.
	if f.matchAll {
		m := MetricMatch{Name: rawName, Type: "native"}
		return m, true
	}

	// Check regex includes (with optional caching).
	if f.cacheWildcards {
		return f.matchRegexCached(rawName)
	}
	return f.matchRegex(rawName)
}

// isExcluded returns true if rawName matches any exclude rule.
func (f *MetricFilter) isExcluded(rawName string) bool {
	if _, ok := f.exactExcludes[rawName]; ok {
		return true
	}
	for _, re := range f.regexExcludes {
		if re.MatchString(rawName) {
			return true
		}
	}
	return false
}

// matchRegex checks rawName against all regex include patterns without caching.
func (f *MetricFilter) matchRegex(rawName string) (MetricMatch, bool) {
	for _, ri := range f.regexIncludes {
		if ri.pattern.MatchString(rawName) {
			m := ri.match
			// Regex matches keep the original raw name.
			m.Name = rawName
			return m, true
		}
	}
	return MetricMatch{}, false
}

// matchRegexCached checks rawName against all regex include patterns with caching.
func (f *MetricFilter) matchRegexCached(rawName string) (MetricMatch, bool) {
	// Fast path: read lock.
	f.cacheMu.RLock()
	if result, ok := f.cache[rawName]; ok {
		f.cacheMu.RUnlock()
		return result.match, result.matched
	}
	f.cacheMu.RUnlock()

	// Slow path: compute and cache.
	match, matched := f.matchRegex(rawName)

	f.cacheMu.Lock()
	f.cache[rawName] = cachedResult{match: match, matched: matched}
	f.cacheMu.Unlock()

	return match, matched
}

// ShouldExcludeSample checks if a sample should be skipped based on its labels.
func (f *MetricFilter) ShouldExcludeSample(labels map[string]string) bool {
	if len(f.excludeByLabels) == 0 {
		return false
	}

	for labelName, excludeValues := range f.excludeByLabels {
		labelValue, hasLabel := labels[labelName]
		if !hasLabel {
			continue
		}
		for _, ev := range excludeValues {
			if ev == "*" || ev == labelValue {
				return true
			}
		}
	}
	return false
}

// isRegexPattern returns true if s contains regex metacharacters, suggesting
// it should be treated as a regex rather than a plain metric name.
func isRegexPattern(s string) bool {
	return strings.ContainsAny(s, regexMetaChars)
}
