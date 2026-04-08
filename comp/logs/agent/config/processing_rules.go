// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"regexp"
)

// Processing rule types
const (
	ExcludeAtMatch         = "exclude_at_match"
	IncludeAtMatch         = "include_at_match"
	MaskSequences          = "mask_sequences"
	MultiLine              = "multi_line"
	ExcludeTruncated       = "exclude_truncated"
	RemapAttributeToSource = "remap_attribute_to_source"
)

// SourceMappingEntry defines a single attribute-value-to-source mapping
// used by the RemapAttributeToSource processing rule type.
type SourceMappingEntry struct {
	Attribute     string `mapstructure:"attribute" json:"attribute" yaml:"attribute"`
	Value         string `mapstructure:"value" json:"value" yaml:"value"`
	RemapSourceTo string `mapstructure:"remap_source_to" json:"remap_source_to" yaml:"remap_source_to"`
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
	Mappings           []*SourceMappingEntry `mapstructure:"mappings" json:"mappings" yaml:"mappings"`
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
		case RemapAttributeToSource:
			if len(rule.Mappings) == 0 {
				return fmt.Errorf("no mappings provided for processing rule: %s", rule.Name)
			}
			for i, m := range rule.Mappings {
				if m.Attribute == "" {
					return fmt.Errorf("mapping %d has empty attribute in processing rule: %s", i, rule.Name)
				}
				if m.Value == "" {
					return fmt.Errorf("mapping %d has empty value in processing rule: %s", i, rule.Name)
				}
				if m.RemapSourceTo == "" {
					return fmt.Errorf("mapping %d has empty remap_source_to in processing rule: %s", i, rule.Name)
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
		if rule.Type == ExcludeTruncated || rule.Type == RemapAttributeToSource {
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
