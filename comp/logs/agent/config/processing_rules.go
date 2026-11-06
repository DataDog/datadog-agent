// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
)

// Processing rule types
const (
	// ExcludeAtMatchOTTL drops messages for which the OTTL condition is true.
	ExcludeAtMatchOTTL = "exclude_at_match_ottl"
	// IncludeAtMatchOTTL keeps messages for which the OTTL condition is true.
	IncludeAtMatchOTTL = "include_at_match_ottl"
	// MaskOTTLTransform edits log content using an OTTL masking statement.
	MaskOTTLTransform = "mask_ottl"

	ExcludeAtMatch   = "exclude_at_match"
	IncludeAtMatch   = "include_at_match"
	MaskSequences    = "mask_sequences"
	MultiLine        = "multi_line"
	ExcludeTruncated = "exclude_truncated"
	RemapSource      = "remap_source"
)

// SourceMatchEntry defines a single attribute-value-to-source match
// used by the RemapSource processing rule type.
type SourceMatchEntry struct {
	Attribute string `mapstructure:"attribute" json:"attribute" yaml:"attribute"`
	Value     string `mapstructure:"value" json:"value" yaml:"value"`
	NewSource string `mapstructure:"new_source" json:"new_source" yaml:"new_source"`
}

// ProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type ProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string `mapstructure:"replace_placeholder" json:"replace_placeholder" yaml:"replace_placeholder"`
	Pattern            string
	OTTL               CompiledOTTLRule `mapstructure:"-" json:"-" yaml:"-"`
	Regex              *regexp.Regexp
	Placeholder        []byte
	Matching           []*SourceMatchEntry `mapstructure:"matching" json:"matching" yaml:"matching"`
}

// ValidateProcessingRules validates the rules and raises an error if one is misconfigured.
// Each processing rule must have:
// - a valid name
// - a valid type
// - a valid regex pattern, or a nonempty OTTL expression
// OTTL syntax is validated by the logs runtime before the configuration is used.
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
		case ExcludeAtMatchOTTL, IncludeAtMatchOTTL, MaskOTTLTransform:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			// The logs runtime validates OTTL syntax before accepting the source.
		case ExcludeTruncated:
			break
		case RemapSource:
			if len(rule.Matching) == 0 {
				return fmt.Errorf("no matching entries provided for processing rule: %s", rule.Name)
			}
			for i, m := range rule.Matching {
				if m.Attribute == "" {
					return fmt.Errorf("match %d has empty attribute in processing rule: %s", i, rule.Name)
				}
				if m.Value == "" {
					return fmt.Errorf("match %d has empty value in processing rule: %s", i, rule.Name)
				}
				if m.NewSource == "" {
					return fmt.Errorf("match %d has empty new_source in processing rule: %s", i, rule.Name)
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

// CompileProcessingRules compiles regular expressions. OTTL compilation belongs to the logs runtime.
func CompileProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case ExcludeAtMatchOTTL, IncludeAtMatchOTTL, MaskOTTLTransform, ExcludeTruncated, RemapSource:
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

// ApplyMaskSequence applies a compiled mask_sequences rule to content.
func ApplyMaskSequence(content []byte, rule *ProcessingRule) ([]byte, bool) {
	if rule.Type != MaskSequences {
		return content, false
	}

	prefix, _ := rule.Regex.LiteralPrefix()
	if prefix != "" && !bytes.Contains(content, []byte(prefix)) {
		return content, false
	}

	masked := rule.Regex.ReplaceAll(content, rule.Placeholder)
	return masked, !bytes.Equal(content, masked)
}

// OTTLContext is the message-local state shared by compiled OTTL rules. Keeping
// this interface here lets config consumers avoid linking the OTTL engine.
type OTTLContext interface {
	Content() []byte
	Reset([]byte)
	Release()
}

// CompiledOTTLRule is attached by the logs runtime during setup, before rules are
// published to processors. Evaluate returns a filter match or a masking change.
// Implementations must support concurrent evaluation with separate contexts.
type CompiledOTTLRule interface {
	NewContext([]byte) OTTLContext
	Evaluate(OTTLContext) (bool, error)
}
