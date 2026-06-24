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
//             namespace: dogstatsd
//             tags: ["env:dev"]
//
//   code:
//     if rules.isAllowed(sample.GetName(), source, sample.GetRawTags()) { ... }

package observerimpl

import (
	"fmt"
	"slices"
	"strings"

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
	Namespace   string   `mapstructure:"namespace"`
}

// metricsFilterRules evaluates the ordered rule list against incoming metrics.
type metricsFilterRules struct {
	rules []metricsCompiledRule
}

type metricsCompiledRule struct {
	exclude    bool
	name       string
	namePrefix string
	tags       []string
	namespace  string
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
			namespace:  strings.TrimSpace(rule.Namespace),
		})
	}

	return &metricsFilterRules{rules: compiled}, nil
}

func mustNewMetricsFilterRules(rules []metricsProcessingRule) *metricsFilterRules {
	filter, err := newMetricsFilterRules(rules)
	if err != nil {
		panic(err)
	}
	return filter
}

func newDefaultMetricsFilterRules() *metricsFilterRules {
	return mustNewMetricsFilterRules(defaultMetricsProcessingRules())
}

func defaultMetricsProcessingRules() []metricsProcessingRule {
	return []metricsProcessingRule{
		{
			Type:      excludeAtMatch,
			Name:      "drop_agent_metrics",
			Namespace: observerdef.AgentNamespace,
		},
	}
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
		return "", fmt.Errorf("name_pattern must be a prefix with an optional trailing *")
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
			return nil, fmt.Errorf("tags must not contain empty values")
		}
		compiled = append(compiled, trimmed)
	}
	slices.Sort(compiled)
	return compiled, nil
}

// isAllowed returns true if the metric should be ingested.
func (f *metricsFilterRules) isAllowed(name, namespace string, tags []string) bool {
	if f == nil {
		return true
	}

	if namespace == LogMetricsExtractorName {
		return true
	}

	for _, rule := range f.rules {
		if rule.matches(name, namespace, tags) {
			return !rule.exclude
		}
	}

	return true
}

func (r metricsCompiledRule) matches(name, namespace string, tags []string) bool {
	if r.namespace != "" && namespace != r.namespace {
		return false
	}

	if r.namePrefix != "" && !strings.HasPrefix(name, r.namePrefix) {
		return false
	}

	for _, ruleTag := range r.tags {
		if !containsRuleTag(tags, ruleTag) {
			return false
		}
	}

	return true
}

func containsRuleTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
