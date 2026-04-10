// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Processing rule types
const (
	ExcludeAtMatch      = "exclude_at_match"
	IncludeAtMatch      = "include_at_match"
	MaskSequences       = "mask_sequences"
	MultiLine           = "multi_line"
	ExcludeTruncated    = "exclude_truncated"
	EdgeOnly            = "edge_only"
	ExcludeFromObserver = "exclude_from_observer"
)

// validSeverities is the set of known log severity values.
var validSeverities = map[string]bool{
	"trace":     true,
	"debug":     true,
	"info":      true,
	"warn":      true,
	"warning":   true,
	"error":     true,
	"critical":  true,
	"off":       true,
	"emergency": true,
	"alert":     true,
	"notice":    true,
}

// RoutingMatch defines the criteria for edge_only and exclude_from_observer rules.
// Within each field values are OR'd; across fields they are AND'd.
// An omitted (nil/empty) field matches all messages.
type RoutingMatch struct {
	Services   []string `mapstructure:"services" json:"services" yaml:"services"`
	Sources    []string `mapstructure:"sources" json:"sources" yaml:"sources"`
	Severities []string `mapstructure:"severities" json:"severities" yaml:"severities"`
}

// CompiledRoutingMatch holds pre-compiled matchers for a RoutingMatch.
type CompiledRoutingMatch struct {
	servicePatterns []string
	sourcePatterns  []string
	severitySet     map[string]bool
}

// Matches reports whether the given service, source, and status satisfy all criteria.
func (c *CompiledRoutingMatch) Matches(service, source, status string) bool {
	if len(c.servicePatterns) > 0 && !matchesAnyGlob(c.servicePatterns, service) {
		return false
	}
	if len(c.sourcePatterns) > 0 && !matchesAnyGlob(c.sourcePatterns, source) {
		return false
	}
	if len(c.severitySet) > 0 {
		// Normalize "warning" → "warn" to match common agent status strings
		s := strings.ToLower(status)
		if s == "warning" {
			s = "warn"
		}
		if !c.severitySet[s] {
			return false
		}
	}
	return true
}

// matchesAnyGlob returns true if value matches at least one of the glob patterns.
// path.Match is used for glob syntax (* and ?). Pattern errors are ignored at
// match time because patterns are validated at compile time.
func matchesAnyGlob(patterns []string, value string) bool {
	for _, p := range patterns {
		if matched, err := path.Match(p, value); err == nil && matched {
			return true
		}
	}
	return false
}

// ProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type ProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string `mapstructure:"replace_placeholder" json:"replace_placeholder" yaml:"replace_placeholder"`
	Pattern            string
	Regex              *regexp.Regexp
	Placeholder        []byte
	// Match is used by edge_only and exclude_from_observer rule types.
	Match         *RoutingMatch         `mapstructure:"match" json:"match" yaml:"match"`
	CompiledMatch *CompiledRoutingMatch `mapstructure:"-" json:"-" yaml:"-"`
}

// ValidateProcessingRules validates the rules and raises an error if one is misconfigured.
// Each processing rule must have:
// - a valid name
// - a valid type
// - a valid pattern that compiles
func ValidateProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		if rule.Name == "" {
			return errors.New("all processing rules must have a name")
		}

		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			_, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid pattern %s for processing rule: %s", rule.Pattern, rule.Name)
			}
		case ExcludeTruncated:
			break
		case EdgeOnly, ExcludeFromObserver:
			if rule.Match == nil {
				return fmt.Errorf("processing rule %q of type %q requires a 'match' block", rule.Name, rule.Type)
			}
			for _, sev := range rule.Match.Severities {
				normalized := strings.ToLower(sev)
				if normalized == "warning" {
					normalized = "warn"
				}
				if !validSeverities[normalized] {
					return fmt.Errorf("invalid severity %q in processing rule %q", sev, rule.Name)
				}
			}
			for _, p := range rule.Match.Services {
				if _, err := path.Match(p, ""); err != nil {
					return fmt.Errorf("invalid service pattern %q in processing rule %q: %v", p, rule.Name, err)
				}
			}
			for _, p := range rule.Match.Sources {
				if _, err := path.Match(p, ""); err != nil {
					return fmt.Errorf("invalid source pattern %q in processing rule %q: %v", p, rule.Name, err)
				}
			}
		case "":
			return fmt.Errorf("type must be set for processing rule `%s`", rule.Name)
		default:
			return fmt.Errorf("type %s is not supported for processing rule `%s`", rule.Type, rule.Name)
		}
	}
	return nil
}

// CompileProcessingRules compiles all processing rule regular expressions.
func CompileProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case ExcludeTruncated:
			continue
		case EdgeOnly, ExcludeFromObserver:
			if rule.Match != nil {
				rule.CompiledMatch = compileRoutingMatch(rule.Match)
			}
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return err
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch:
			rule.Regex = re
		case MaskSequences:
			rule.Regex = re
			rule.Placeholder = []byte(rule.ReplacePlaceholder)
		case MultiLine:
			rule.Regex, err = regexp.Compile("^" + rule.Pattern)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// compileRoutingMatch builds a CompiledRoutingMatch from a RoutingMatch.
func compileRoutingMatch(m *RoutingMatch) *CompiledRoutingMatch {
	c := &CompiledRoutingMatch{
		servicePatterns: m.Services,
		sourcePatterns:  m.Sources,
	}
	if len(m.Severities) > 0 {
		c.severitySet = make(map[string]bool, len(m.Severities))
		for _, sev := range m.Severities {
			normalized := strings.ToLower(sev)
			if normalized == "warning" {
				normalized = "warn"
			}
			c.severitySet[normalized] = true
		}
	}
	return c
}
