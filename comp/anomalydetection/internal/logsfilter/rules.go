// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Usage:
//   datadog.yaml:
//     anomaly_detection:
//       logs:
//         processing_rules:
//           - type: exclude_at_match
//             name: drop_dev_containers
//             tags: ["env:dev"]
//
//           - type: exclude_at_match
//             name: drop_containerd_runtime
//             source: containerd
//
//   code:
//     if rules.IsAllowed(source, msg.Tags()) { ... }

package logsfilter

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

const (
	excludeAtMatch = "exclude_at_match"
	includeAtMatch = "include_at_match"
)

// ProcessingRule is one entry in anomaly_detection.logs.processing_rules.
type ProcessingRule struct {
	Type   string   `mapstructure:"type"`
	Name   string   `mapstructure:"name"`
	Source string   `mapstructure:"source"`
	Tags   []string `mapstructure:"tags"`
}

// Rules evaluates the ordered rule list against incoming log sources.
type Rules struct {
	rules       []compiledRule
	hasTagRules bool // true if any rule has tag predicates, requiring sorted input
}

type compiledRule struct {
	exclude bool
	name    string
	source  string
	tags    []string // sorted, trimmed
}

// NewRules parses, validates, and compiles the given rule list.
func NewRules(rules []ProcessingRule) (*Rules, error) {
	compiled := make([]compiledRule, 0, len(rules))
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

		tags, err := compileRuleTags(rule.Tags)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", name, err)
		}

		compiled = append(compiled, compiledRule{
			exclude: exclude,
			name:    name,
			source:  strings.TrimSpace(rule.Source),
			tags:    tags,
		})
	}
	hasTagRules := false
	for _, r := range compiled {
		if len(r.tags) > 0 {
			hasTagRules = true
			break
		}
	}
	return &Rules{rules: compiled, hasTagRules: hasTagRules}, nil
}

// LoadRules reads processing rules from the given config key and compiles them.
// Returns an empty (allow-all) Rules when the key is absent.
// EnableStringUnmarshal allows the key to be set as a JSON array via environment
// variable (e.g. DD_ANOMALY_DETECTION_LOGS_PROCESSING_RULES).
func LoadRules(cfg config.Component, key string) (*Rules, error) {
	if !cfg.IsConfigured(key) {
		return &Rules{}, nil
	}
	var raw []ProcessingRule
	if err := structure.UnmarshalKey(cfg, key, &raw, structure.EnableStringUnmarshal); err != nil {
		return nil, fmt.Errorf("%s: decode failed: %w", key, err)
	}
	return NewRules(raw)
}

// NeedsSortedTags reports whether any rule has tag predicates requiring sorted input to IsAllowed.
func (r *Rules) NeedsSortedTags() bool {
	return r != nil && r.hasTagRules
}

// IsAllowed returns true if the log should be ingested.
// tags must be sorted when NeedsSortedTags returns true.
// A nil receiver always allows.
func (r *Rules) IsAllowed(source string, tags []string) bool {
	if r == nil {
		return true
	}
	for _, rule := range r.rules {
		if rule.matches(source, tags) {
			return !rule.exclude
		}
	}
	return true
}

// matches reports whether the rule applies to the given log.
// tags must be sorted in ascending order (guaranteed by callers of IsAllowed).
func (r compiledRule) matches(source string, tags []string) bool {
	if r.source != "" && r.source != source {
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
