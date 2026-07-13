// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/vrl"
)

// Processing rule types
const (
	ExcludeAtMatch   = "exclude_at_match"
	IncludeAtMatch   = "include_at_match"
	MaskSequences    = "mask_sequences"
	MultiLine        = "multi_line"
	ExcludeTruncated = "exclude_truncated"
	RemapSource      = "remap_source"
	// ExcludeAtVRLMatch drops log lines for which the VRL expression evaluates to true.
	// The pattern must be a valid VRL boolean expression (e.g. `.status == "debug"`).
	ExcludeAtVRLMatch = "exclude_at_vrl_match"
	// IncludeAtVRLMatch keeps only log lines for which the VRL expression evaluates to true.
	IncludeAtVRLMatch = "include_at_vrl_match"
	// MaskVRLTransform mutates the log line's content using a VRL transform
	// (e.g. `.message = redact(.message, [...])`). Unlike MaskSequences, the
	// replacement logic lives entirely in the VRL source, not in a separate
	// placeholder field.
	MaskVRLTransform = "mask_vrl"
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
	Regex              *regexp.Regexp
	Placeholder        []byte
	Matching           []*SourceMatchEntry `mapstructure:"matching" json:"matching" yaml:"matching"`
	// VRLFilter is set for ExcludeAtVRLMatch and IncludeAtVRLMatch rules after compilation.
	// It returns (true, nil) when the VRL expression matches the given input,
	// (false, nil) when it doesn't, and (false, err) on a compile/runtime error.
	VRLFilter func(input []byte) (bool, error) `json:"-" yaml:"-" mapstructure:"-"`
	// VRLTransform is set for MaskVRLTransform rules after compilation.
	// It returns the (possibly mutated) message content, or an error if the
	// VRL program failed to run.
	VRLTransform func(input []byte) ([]byte, error) `json:"-" yaml:"-" mapstructure:"-"`
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
		case ExcludeAtVRLMatch, IncludeAtVRLMatch, MaskVRLTransform:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			if _, err := vrl.Compile(rule.Pattern); err != nil {
				return fmt.Errorf("invalid VRL pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
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

// CompileProcessingRules compiles all processing rule regular expressions and VRL programs.
func CompileProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case ExcludeTruncated, RemapSource:
			continue
		case ExcludeAtVRLMatch, IncludeAtVRLMatch:
			prog, err := vrl.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid VRL pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
			rule.VRLFilter = prog.Filter
			continue
		case MaskVRLTransform:
			prog, err := vrl.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid VRL pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
			rule.VRLTransform = prog.Transform
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
