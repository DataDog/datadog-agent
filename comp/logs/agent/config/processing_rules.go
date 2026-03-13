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
	"regexp/syntax"

	re2 "github.com/DataDog/datadog-agent/pkg/logs/re2"
)

// Processing rule types
const (
	ExcludeAtMatch   = "exclude_at_match"
	IncludeAtMatch   = "include_at_match"
	MaskSequences    = "mask_sequences"
	MultiLine        = "multi_line"
	ExcludeTruncated = "exclude_truncated"
)

// ProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type ProcessingRule struct {
	Type                    string
	Name                    string
	ReplacePlaceholder      string `mapstructure:"replace_placeholder" json:"replace_placeholder" yaml:"replace_placeholder"`
	Pattern                 string
	Regex                   *regexp.Regexp
	Placeholder             []byte
	designation             string
	placeholderHasExpansion bool

	// Pre-computed literal strings extracted from the pattern at compile time.
	// Covers pure literals ("DEBUG") and alternations of literals
	// ("DEBUG|TRACE", "(?:healthcheck|readiness|liveness)"). When non-nil,
	// the processor uses bytes.Contains instead of the regex engine.
	literalContents [][]byte

	// hasLiteralPrefix is true when the compiled regex has a non-empty literal
	// prefix (e.g. "api_key=" in "api_key=[a-f0-9]{28}"). Used as a signal
	// that the pattern is structurally simple enough for stdlib's NFA to
	// handle efficiently on short content, avoiding CGo overhead.
	hasLiteralPrefix bool

	// re2Compiled holds a go-re2 compiled regex. Under the re2_cgo build
	// tag it points to an actual go-re2 Regexp backed by Google's RE2 via
	// CGo. Under the default build, re2.Compile returns nil so this field
	// stays nil and the processor falls back to stdlib.
	re2Compiled *re2.Regexp
}

// GetDesignation returns the designation of the processing rule.
func (r *ProcessingRule) GetDesignation() string {
	return r.designation
}

// HasLiteralContents reports whether this rule has pre-computed literal
// byte strings that can be checked with bytes.Contains instead of the
// regex engine.
func (r *ProcessingRule) HasLiteralContents() bool {
	return r.literalContents != nil
}

// LiteralContents returns the pre-computed literal byte strings extracted
// from the pattern at compile time, or nil if the pattern is not a pure
// literal or alternation of literals.
func (r *ProcessingRule) LiteralContents() [][]byte {
	return r.literalContents
}

// HasLiteralPrefix reports whether the compiled regex has a non-empty literal
// prefix guaranteed to start every match.
func (r *ProcessingRule) HasLiteralPrefix() bool {
	return r.hasLiteralPrefix
}

// PlaceholderHasExpansion returns true if the replacement placeholder contains
// '$' characters that require regexp.Expand for capture-group substitution.
func (r *ProcessingRule) PlaceholderHasExpansion() bool {
	return r.placeholderHasExpansion
}

// RE2Compiled returns the go-re2 compiled regex, or nil if the re2_cgo
// build tag is inactive or the pattern failed to compile.
func (r *ProcessingRule) RE2Compiled() *re2.Regexp {
	return r.re2Compiled
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
		rule.designation = rule.Type + ":" + rule.Name
		if rule.Type == ExcludeTruncated {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return err
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch:
			rule.Regex = re
			rule.literalContents = extractLiteralAlternatives(rule.Pattern)
			if rule.literalContents == nil {
				prefix, _ := re.LiteralPrefix()
				rule.hasLiteralPrefix = prefix != ""
				rule.re2Compiled, _ = re2.Compile(rule.Pattern)
			}
		case MaskSequences:
			rule.Regex = re
			rule.Placeholder = []byte(rule.ReplacePlaceholder)
			rule.placeholderHasExpansion = bytes.IndexByte(rule.Placeholder, '$') >= 0
			prefix, _ := re.LiteralPrefix()
			rule.hasLiteralPrefix = prefix != ""
			rule.re2Compiled, _ = re2.Compile(rule.Pattern)
		case MultiLine:
			rule.Regex, err = regexp.Compile("^" + rule.Pattern)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// extractLiteralAlternatives parses a regex pattern and returns the literal
// byte strings if the pattern is a pure literal or an alternation of pure
// literals (e.g. "DEBUG", "DEBUG|TRACE", "(?:healthcheck|readiness|liveness)").
// Returns nil if the pattern contains any non-literal components such as
// character classes, quantifiers, or anchors.
func extractLiteralAlternatives(pattern string) [][]byte {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil
	}
	re = re.Simplify()
	return extractLiteralsFromTree(re)
}

func extractLiteralsFromTree(re *syntax.Regexp) [][]byte {
	switch re.Op {
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			return nil
		}
		return [][]byte{[]byte(string(re.Rune))}
	case syntax.OpAlternate:
		var result [][]byte
		for _, sub := range re.Sub {
			lits := extractLiteralsFromTree(sub)
			if lits == nil {
				return nil
			}
			result = append(result, lits...)
		}
		return result
	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			return extractLiteralsFromTree(re.Sub[0])
		}
	}
	return nil
}
