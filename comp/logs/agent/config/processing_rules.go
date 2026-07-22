// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/itchyny/gojq"

	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
)

// Processing rule types
const (
	ExcludeAtMatch   = "exclude_at_match"
	IncludeAtMatch   = "include_at_match"
	MaskSequences    = "mask_sequences"
	MultiLine        = "multi_line"
	ExcludeTruncated = "exclude_truncated"
	RemapSource      = "remap_source"
	// ExcludeAtJQMatch drops log lines for which the jq expression produces output.
	// The pattern must be a valid jq expression (e.g. `select(.level == "debug")`); a
	// bare boolean expression like `.level == "debug"` always produces output and thus
	// always "matches" — use `select(...)` for real filtering. Non-JSON content, and
	// any jq runtime error, is passed through unchanged (fail-open).
	ExcludeAtJQMatch = "exclude_at_jq_match"
	// IncludeAtJQMatch keeps only log lines for which the jq expression produces output.
	// Same matching convention and fail-open behavior as ExcludeAtJQMatch.
	IncludeAtJQMatch = "include_at_jq_match"
	// MaskJQTransform mutates the log line's content using a jq program that
	// transforms the whole parsed JSON document (e.g.
	// `.message |= gsub("(?<n>[0-9]{4,})"; "[REDACTED-\(.n)]")`), and re-serializes the
	// result as the new content. Unlike the filter rules above, this rule type is
	// fail-closed: a non-JSON message, a jq runtime error, or a program producing no
	// output all result in the message being dropped rather than risk leaking
	// unredacted content. Re-serialization sorts object keys alphabetically, so masked
	// output may not preserve the original field order.
	MaskJQTransform = "mask_jq"
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
	// JQFilter is set for ExcludeAtJQMatch and IncludeAtJQMatch rules after compilation.
	// It returns (true, nil) when the jq program produces output for the given input,
	// (false, nil) when it produces no output, and (false, err) on a runtime error
	// (including non-JSON input, which gojq cannot parse).
	JQFilter func(input []byte) (bool, error) `json:"-" yaml:"-" mapstructure:"-"`
	// JQTransform is set for MaskJQTransform rules after compilation. It returns the
	// mutated message content, or an error if the jq program failed to run or produced
	// no output.
	JQTransform func(input []byte) ([]byte, error) `json:"-" yaml:"-" mapstructure:"-"`
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
		case ExcludeAtJQMatch, IncludeAtJQMatch, MaskJQTransform:
			if rule.Pattern == "" {
				return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
			}
			if _, err := jsonquery.Parse(rule.Pattern); err != nil {
				return fmt.Errorf("invalid jq pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
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

// CompileProcessingRules compiles all processing rule regular expressions and jq programs.
func CompileProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case ExcludeTruncated, RemapSource:
			continue
		case ExcludeAtJQMatch, IncludeAtJQMatch:
			code, err := jsonquery.Parse(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid jq pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
			rule.JQFilter = makeJQFilter(code)
			continue
		case MaskJQTransform:
			code, err := jsonquery.Parse(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid jq pattern %q for processing rule %s: %w", rule.Pattern, rule.Name, err)
			}
			rule.JQTransform = makeJQTransform(code)
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

// runJQ decodes input as JSON and runs the compiled jq program against it,
// returning the first result. ok is false when the program produced no output.
func runJQ(code *gojq.Code, input []byte) (result any, ok bool, err error) {
	var v any
	if err := json.Unmarshal(input, &v); err != nil {
		return nil, false, err
	}
	iter := code.Run(v)
	res, hasResult := iter.Next()
	if !hasResult {
		return nil, false, nil
	}
	if runErr, isErr := res.(error); isErr {
		return nil, false, runErr
	}
	return res, true, nil
}

// makeJQFilter builds the boolean-match function for ExcludeAtJQMatch/IncludeAtJQMatch
// rules: any output produced by the jq program counts as a match.
func makeJQFilter(code *gojq.Code) func(input []byte) (bool, error) {
	return func(input []byte) (bool, error) {
		_, ok, err := runJQ(code, input)
		if err != nil {
			return false, err
		}
		return ok, nil
	}
}

// makeJQTransform builds the content-mutation function for MaskJQTransform rules. The
// jq program is expected to output a full, transformed JSON document; the result is
// re-serialized as the new message content.
func makeJQTransform(code *gojq.Code) func(input []byte) ([]byte, error) {
	return func(input []byte) ([]byte, error) {
		result, ok, err := runJQ(code, input)
		if err != nil {
			return nil, fmt.Errorf("jq mask runtime error: %w", err)
		}
		if !ok {
			return nil, errors.New("jq mask transform produced no output")
		}
		out, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("jq mask transform: failed to encode output: %w", err)
		}
		return out, nil
	}
}
