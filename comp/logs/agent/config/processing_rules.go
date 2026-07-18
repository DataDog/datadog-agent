// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"regexp"

	// We need to import the OTTL package to be able to parse OTTL conditions in processing rules, even if we don't use it directly in this file.
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"

	"github.com/DataDog/fastjq"

	"github.com/DataDog/datadog-agent/pkg/logs/vrl"
)

// Processing rule types
const (
	ExcludeAtMatchOTTL = "exclude_at_match_ottl"
	IncludeAtMatchOTTL = "include_at_match_ottl"

	ExcludeAtMatch   = "exclude_at_match"
	IncludeAtMatch   = "include_at_match"
	MaskSequences    = "mask_sequences"
	MultiLine        = "multi_line"
	ExcludeTruncated = "exclude_truncated"
	RemapSource      = "remap_source"
	// ExcludeAtJQMatch drops log lines for which the jq expression produces output.
	// The pattern must be a valid jq expression (e.g. `select(.level == "debug")`).
	// Non-JSON content is passed through unchanged.
	ExcludeAtJQMatch = "exclude_at_jq_match"
	// IncludeAtJQMatch keeps only log lines for which the jq expression produces output.
	// Non-JSON content is passed through unchanged.
	IncludeAtJQMatch = "include_at_jq_match"
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
	OTTLCondition      *ottl.Condition[*ottllog.TransformContext]
	Placeholder        []byte
	Matching           []*SourceMatchEntry `mapstructure:"matching" json:"matching" yaml:"matching"`
	// JQFilter is set for ExcludeAtJQMatch and IncludeAtJQMatch rules after compilation.
	// It returns (true, nil) when the jq program produces output for the given input,
	// (false, nil) when it produces no output, and (false, err) on program error.
	JQFilter func(input []byte) (bool, error) `json:"-" yaml:"-" mapstructure:"-"`
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
		case ExcludeAtMatchOTTL, IncludeAtMatchOTTL:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			err := validateOTTLPattern(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid OTTL pattern %s for processing rule: %s, error: %v", rule.Pattern, rule.Name, err)
			}
		case ExcludeAtJQMatch, IncludeAtJQMatch:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			if _, err := fastjq.Compile(rule.Pattern); err != nil {
				return fmt.Errorf("invalid jq pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
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

// CompileProcessingRules compiles all processing rule regular expressions, OTTL conditions, jq programs, and VRL programs.
func CompileProcessingRules(rules []*ProcessingRule) error {

	for _, rule := range rules {
		switch rule.Type {
		case ExcludeTruncated, RemapSource:
			continue
		case ExcludeAtJQMatch, IncludeAtJQMatch:
			prog, err := fastjq.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid jq pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
			rule.JQFilter = makeJQFilter(prog)
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
		// re, err := regexp.Compile(rule.Pattern)
		// if err != nil {
		// 	return err
		// }
		switch rule.Type {
		case ExcludeAtMatchOTTL, IncludeAtMatchOTTL:
			err := compileOTTLProcessingRule(rule)
			if err != nil {
				return err
			}

		case ExcludeAtMatch, IncludeAtMatch:
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return err
			}
			rule.Regex = re

		case MaskSequences:
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return err
			}
			rule.Regex = re
			rule.Placeholder = []byte(rule.ReplacePlaceholder)

		case MultiLine:
			re, err := regexp.Compile("^" + rule.Pattern)
			if err != nil {
				return err
			}
			rule.Regex = re
		}
	}
	return nil
}

// validateOTTLPattern checks that the given OTTL pattern is valid and can be parsed by the OTTL parser.
func validateOTTLPattern(pattern string) error {
	settings := component.TelemetrySettings{Logger: zap.NewNop()} // settings are only used for logging during parsing, so we can use a no-op logger here
	parser, err := ottllog.NewParser(ottlfuncs.StandardFuncs[*ottllog.TransformContext](), settings)
	if err != nil {
		return err
	}
	_, err = parser.ParseCondition(pattern)
	return err
}

func compileOTTLProcessingRule(rule *ProcessingRule) error {
	settings := component.TelemetrySettings{Logger: zap.NewNop()} // settings are only used for logging during parsing, so we can use a no-op logger here
	parser, err := ottllog.NewParser(ottlfuncs.StandardFuncs[*ottllog.TransformContext](), settings)
	if err != nil {
		return err
	}
	condition, err := parser.ParseCondition(rule.Pattern)
	if err != nil {
		return fmt.Errorf("error parsing OTTL condition for processing rule `%s`: %v", rule.Name, err)
	}
	rule.OTTLCondition = condition
	return nil
}

// makeJQFilter returns a function that reports whether a jq program produces output for the given input.
// Non-JSON input always returns (false, nil) — never silently dropped.
func makeJQFilter(prog *fastjq.Program) func([]byte) (bool, error) {
	buf := make([]byte, 0, 4096)
	return func(input []byte) (bool, error) {
		matched := false
		buf = buf[:0]
		err := prog.RunFunc(input, func(result []byte) error {
			if len(result) > 0 {
				matched = true
			}
			return nil
		})
		if err != nil {
			// Non-JSON or jq runtime error: treat as no match (pass-through).
			return false, err
		}
		return matched, nil
	}
}
