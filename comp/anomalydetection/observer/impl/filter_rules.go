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
//             host: noisy-host
//             tags: ["env:dev"]
//
//   code:
//     if rules.isAllowedWithHost(sample.GetName(), source, sample.GetHost(), sample.GetTags()) { ... }

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
	Host        string   `mapstructure:"host"`
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
	host       string
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
			host:       strings.TrimSpace(rule.Host),
		})
	}

	return &metricsFilterRules{rules: compiled}, nil
}

// implicitMetricsProcessingRules are evaluated before user-configured rules.
// They protect the observer from ingesting its own Agent telemetry and must not
// be overridable by an include_at_match rule.
func implicitMetricsProcessingRules() []metricsProcessingRule {
	return []metricsProcessingRule{
		{
			Type:        excludeAtMatch,
			Name:        "drop_observer_telemetry",
			NamePattern: observerTelemetryMetricPrefix + "*",
			Source:      observerdef.AgentNamespace,
		},
	}
}

func newDefaultMetricsFilterRules() (*metricsFilterRules, error) {
	return newMetricsFilterRules(implicitMetricsProcessingRules())
}

func loadMetricFilter(cfg config.Component) (*metricsFilterRules, error) {
	var rules []metricsProcessingRule
	if cfg != nil && cfg.IsConfigured(metricProcessingRulesConfigKey) {
		if err := structure.UnmarshalKey(cfg, metricProcessingRulesConfigKey, &rules); err != nil {
			return nil, fmt.Errorf("%s: decode failed: %w", metricProcessingRulesConfigKey, err)
		}
	}

	rules = append(implicitMetricsProcessingRules(), rules...)

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

type metricFilterPrecheck struct {
	reject         bool
	needsTags      bool
	firstCandidate int
}

// precheck evaluates the portions of the ordered rule list that do not depend
// on tags.
//
// This lets the high-volume rejected path avoid copying and sorting tags. It
// deliberately never admits a metric early: admitted metrics still need their
// tags for the mute check and storage. If the first name/source candidate has
// tag conditions, firstCandidate lets the tag-aware pass resume there without
// rescanning rules that cannot match.
func (f *metricsFilterRules) precheck(name, source, host string) metricFilterPrecheck {
	if f == nil || source == LogMetricsExtractorName {
		return metricFilterPrecheck{}
	}

	for i, rule := range f.rules {
		if !rule.matchesNameSourceAndHost(name, source, host) {
			continue
		}
		if len(rule.tags) > 0 {
			return metricFilterPrecheck{needsTags: true, firstCandidate: i}
		}
		return metricFilterPrecheck{reject: rule.exclude}
	}

	return metricFilterPrecheck{}
}

// isAllowed returns true if the metric should be ingested.
// tags must be sorted so the mute hash matches seriesKeyHash in storage.
func (f *metricsFilterRules) isAllowed(name, source string, tags []string) bool {
	return f.isAllowedWithHost(name, source, "", tags)
}

func (f *metricsFilterRules) isAllowedWithHost(name, source, host string, tags []string) bool {
	if f == nil {
		return true
	}

	if source == LogMetricsExtractorName {
		return true
	}

	if f.isMutedWithHost(name, source, host, tags) {
		return false
	}

	return f.isAllowedByRulesFromWithHost(name, source, host, tags, 0)
}

func (f *metricsFilterRules) isMutedWithHost(name, source, host string, tags []string) bool {
	if f == nil || source == LogMetricsExtractorName {
		return false
	}

	if m := f.muted.Load(); m != nil {
		if _, ok := (*m)[seriesKeyHash(source, name, host, tags)]; ok {
			return true
		}
	}
	return false
}

func (f *metricsFilterRules) isAllowedByRulesFromWithHost(name, source, host string, tags []string, start int) bool {
	for _, rule := range f.rules[start:] {
		if rule.matchesWithHost(name, source, host, tags) {
			return !rule.exclude
		}
	}

	return true
}

// publishMutedSnapshot atomically publishes an immutable baseline mute union.
// The engine owns constructing this copy-on-write snapshot; callers and
// readers must never mutate m after publication.
func (f *metricsFilterRules) publishMutedSnapshot(m map[uint64]struct{}) {
	f.muted.Store(&m)
}

func (r metricsCompiledRule) matchesWithHost(name, source, host string, tags []string) bool {
	return r.matchesNameSourceAndHost(name, source, host) && containsAllTagsSorted(tags, r.tags)
}

func (r metricsCompiledRule) matchesNameSourceAndHost(name, source, host string) bool {
	if r.source != "" && source != r.source {
		return false
	}
	if r.host != "" && host != r.host {
		return false
	}

	if r.namePrefix != "" && !strings.HasPrefix(name, r.namePrefix) {
		return false
	}

	return true
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
