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
)

// Processing rule types
const (
	ExcludeAtMatch      = "exclude_at_match"
	IncludeAtMatch      = "include_at_match"
	MaskSequences       = "mask_sequences"
	MultiLine           = "multi_line"
	ExcludeTruncated    = "exclude_truncated"
	LocalProcessingOnly = "local_processing_only"
)

// RoutingMatch defines the matching criteria for routing rules (local_processing_only, remote_processing_only).
// Values within services are OR'd. An empty list matches all.
type RoutingMatch struct {
	Services []string `mapstructure:"services" json:"services" yaml:"services"`
}

// CompiledRoutingMatch holds validated service glob patterns ready for runtime matching.
// Glob syntax is validated at compile time; matching uses path.Match at runtime.
type CompiledRoutingMatch struct {
	ServiceGlobs []string
}

// Matches reports whether the given service satisfies the routing match.
// An empty ServiceGlobs list means "match all".
func (m *CompiledRoutingMatch) Matches(service string) bool {
	if len(m.ServiceGlobs) == 0 {
		return true
	}
	for _, g := range m.ServiceGlobs {
		if matched, _ := path.Match(g, service); matched {
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
	Match              *RoutingMatch         `mapstructure:"match" json:"match,omitempty" yaml:"match,omitempty"`
	CompiledMatch      *CompiledRoutingMatch `mapstructure:"-" json:"-" yaml:"-"`
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
		case LocalProcessingOnly:
			if rule.Match == nil || len(rule.Match.Services) == 0 {
				return fmt.Errorf("processing rule `%s` of type %s requires a non-empty match.services list", rule.Name, rule.Type)
			}
			for _, g := range rule.Match.Services {
				if _, err := path.Match(g, ""); err != nil {
					return fmt.Errorf("invalid services glob pattern %q in processing rule `%s`: %v", g, rule.Name, err)
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
		case LocalProcessingOnly:
			if rule.Match == nil {
				continue
			}
			rule.CompiledMatch = &CompiledRoutingMatch{
				ServiceGlobs: rule.Match.Services,
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
