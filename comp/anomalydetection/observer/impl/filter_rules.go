// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Usage:
//   datadog.yaml:
//     anomaly_detection:
//       metrics:
//         processing_rules:
//           - type: exclude_at_match
//             name: drop_dev_dogstatsd
//             source: dogstatsd
//             tags: ["env:dev"]
//
//   code:
//     if rules.isAllowed(sample.GetName(), source, sample.GetRawTags()) { ... }

package observerimpl

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

const (
	excludeAtMatch = "exclude_at_match"
	includeAtMatch = "include_at_match"

	metricProcessingRulesConfigKey = "anomaly_detection.metrics.processing_rules"
)

// metricsProcessingRule is one entry in anomaly_detection.metrics.processing_rules.
type metricsProcessingRule struct {
	Type        string   `mapstructure:"type"`
	Name        string   `mapstructure:"name"`
	NamePattern string   `mapstructure:"name_pattern"`
	Tags        []string `mapstructure:"tags"`
	Source      string   `mapstructure:"source"`
}

// metricsFilterRules evaluates the ordered rule list against incoming metrics.
type metricsFilterRules struct {
	rules []metricsCompiledRule

	// muted tracks metrics that are muted by baseline analysis, these metrics are totally dropped from the storage/engine.
	// It is used by the baseline analysis to reduce false positives.
	muted atomic.Pointer[map[uint64]struct{}]
}

type metricsCompiledRule struct {
	exclude    bool
	name       string
	namePrefix string
	tags       []string
	source     string
}

// newMetricsFilterRules parses, validates, and compiles rules.
func newMetricsFilterRules(rules []metricsProcessingRule) (*metricsFilterRules, error) {
	compiled := make([]metricsCompiledRule, 0, len(rules))
	for i, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return nil, fmt.Errorf("rule %d: name is required", i)
		}

		var exclude bool
		switch strings.TrimSpace(rule.Type) {
		case excludeAtMatch:
			exclude = true
		case includeAtMatch:
			exclude = false
		default:
			return nil, fmt.Errorf("rule %q: unsupported type %q", name, rule.Type)
		}

		namePrefix, err := compileNamePattern(rule.NamePattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", name, err)
		}

		tags, err := compileRuleTags(rule.Tags)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", name, err)
		}

		compiled = append(compiled, metricsCompiledRule{
			exclude:    exclude,
			name:       name,
			namePrefix: namePrefix,
			tags:       tags,
			source:     strings.TrimSpace(rule.Source),
		})
	}

	return &metricsFilterRules{rules: compiled}, nil
}

func defaultMetricsProcessingRules() []metricsProcessingRule {
	return []metricsProcessingRule{
		{
			Type:   excludeAtMatch,
			Name:   "drop_agent_metrics",
			Source: observerdef.AgentNamespace,
		},
	}
}

func newDefaultMetricsFilterRules() (*metricsFilterRules, error) {
	return newMetricsFilterRules(defaultMetricsProcessingRules())
}

func loadMetricFilter(cfg config.Component) (*metricsFilterRules, error) {
	var rules []metricsProcessingRule
	if cfg != nil && cfg.IsConfigured(metricProcessingRulesConfigKey) {
		if err := structure.UnmarshalKey(cfg, metricProcessingRulesConfigKey, &rules); err != nil {
			return nil, fmt.Errorf("%s: decode failed: %w", metricProcessingRulesConfigKey, err)
		}
	}

	rules = append(rules, defaultMetricsProcessingRules()...)

	filter, err := newMetricsFilterRules(rules)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

func compileNamePattern(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", nil
	}

	if idx := strings.IndexByte(pattern, '*'); idx >= 0 && idx != len(pattern)-1 {
		return "", errors.New("name_pattern must be a prefix with an optional trailing *")
	}

	return strings.TrimSuffix(pattern, "*"), nil
}

func compileRuleTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	compiled := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			return nil, errors.New("tags must not contain empty values")
		}
		compiled = append(compiled, trimmed)
	}
	slices.Sort(compiled)
	compiled = slices.Compact(compiled)
	return compiled, nil
}

// isAllowed returns true if the metric should be ingested.
// tags must be sorted so the mute hash matches seriesKeyHash in storage.
func (f *metricsFilterRules) isAllowed(name, source string, tags []string) bool {
	if f == nil {
		return true
	}

	if source == LogMetricsExtractorName {
		return true
	}

	if m := f.muted.Load(); m != nil {
		if _, ok := (*m)[seriesKeyHash(source, name, tags)]; ok {
			return false
		}
	}

	for _, rule := range f.rules {
		if rule.matches(name, source, tags) {
			return !rule.exclude
		}
	}

	return true
}

// setMuted publishes the baseline mute set atomically. Called once at freeze
// from the engine run goroutine; all handle goroutines observe it on next ingest.
func (f *metricsFilterRules) setMuted(m map[uint64]struct{}) {
	f.muted.Store(&m)
}

// matches reports whether the rule applies to the given metric.
// tags must be sorted in ascending order (guaranteed by canonicalizeTags in prepareMetricIngest).
func (r metricsCompiledRule) matches(name, source string, tags []string) bool {
	if r.source != "" && source != r.source {
		return false
	}

	if r.namePrefix != "" && !strings.HasPrefix(name, r.namePrefix) {
		return false
	}

	return containsAllTagsSorted(tags, r.tags)
}

// containsAllTagsSorted reports whether all ruleTags appear in sampleTags.
// Both slices must be sorted in ascending order.
func containsAllTagsSorted(sampleTags, ruleTags []string) bool {
	j := 0
	for i := 0; i < len(sampleTags) && j < len(ruleTags); i++ {
		if sampleTags[i] == ruleTags[j] {
			j++
		}
	}
	return j == len(ruleTags)
}
