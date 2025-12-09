// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/tokens"
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
// be applied on log lines using token-based matching
type ProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string   `mapstructure:"replace_placeholder" json:"replace_placeholder" yaml:"replace_placeholder"`
	TokenPatternStr    []string `mapstructure:"token_pattern" json:"token_pattern" yaml:"token_pattern"`                // e.g., ["D3", "Dash", "D2", "Dash", "D4"]
	PrefilterKeywords  []string `mapstructure:"prefilter_keywords" json:"prefilter_keywords" yaml:"prefilter_keywords"` // e.g., ["-"] for early exit

	// Length constraints for variable-length patterns (e.g., IPv4: \d{1,3})
	LengthConstraintsConfig []LengthConstraintConfig `mapstructure:"length_constraints" json:"length_constraints" yaml:"length_constraints"`

	// Compiled token pattern (populated by CompileProcessingRules)
	TokenPattern         []tokens.Token
	Placeholder          []byte
	PrefilterKeywordsRaw [][]byte           // Converted from PrefilterKeywords
	LengthConstraints    []LengthConstraint // Compiled from LengthConstraintsConfig
}

// LengthConstraintConfig is the YAML/JSON representation of a length constraint
type LengthConstraintConfig struct {
	TokenIndex int `mapstructure:"token_index" json:"token_index" yaml:"token_index"`
	MinLength  int `mapstructure:"min_length" json:"min_length" yaml:"min_length"`
	MaxLength  int `mapstructure:"max_length" json:"max_length" yaml:"max_length"`
}

// LengthConstraint validates the length of a token's literal value at runtime
// Useful for variable-length patterns like \d{1,3} in IPv4 addresses
type LengthConstraint struct {
	TokenIndex int // Index in TokenPattern to apply constraint
	MinLength  int // Minimum literal length (e.g., 1 for \d{1,3})
	MaxLength  int // Maximum literal length (e.g., 3 for \d{1,3})
}

// ValidateProcessingRules validates the rules and raises an error if one is misconfigured.
// Each processing rule must have:
// - a valid name
// - a valid type
// - a valid token pattern (except for exclude_truncated)
func ValidateProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		if rule.Name == "" {
			return errors.New("all processing rules must have a name")
		}

		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			if len(rule.TokenPatternStr) == 0 {
				return fmt.Errorf("no token_pattern provided for processing rule: %s", rule.Name)
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

// CompileProcessingRules compiles token patterns from string names.
func CompileProcessingRules(rules []*ProcessingRule) error {
	for _, rule := range rules {
		if rule.Type == ExcludeTruncated {
			continue
		}

		// Convert token pattern strings to actual tokens
		tokenPattern, err := compileTokenPattern(rule.TokenPatternStr)
		if err != nil {
			return fmt.Errorf("failed to compile token pattern for rule %s: %w", rule.Name, err)
		}
		rule.TokenPattern = tokenPattern

		// Convert prefilter keywords to byte slices
		if len(rule.PrefilterKeywords) > 0 {
			rule.PrefilterKeywordsRaw = make([][]byte, len(rule.PrefilterKeywords))
			for i, keyword := range rule.PrefilterKeywords {
				rule.PrefilterKeywordsRaw[i] = []byte(keyword)
			}
		}

		// Convert length constraints from config format to runtime format
		if len(rule.LengthConstraintsConfig) > 0 {
			rule.LengthConstraints = make([]LengthConstraint, len(rule.LengthConstraintsConfig))
			for i, lc := range rule.LengthConstraintsConfig {
				rule.LengthConstraints[i] = LengthConstraint{
					TokenIndex: lc.TokenIndex,
					MinLength:  lc.MinLength,
					MaxLength:  lc.MaxLength,
				}
			}
		}

		// Set placeholder for mask_sequences
		if rule.Type == MaskSequences {
			rule.Placeholder = []byte(rule.ReplacePlaceholder)
		}
	}
	return nil
}

// compileTokenPattern converts token name strings to tokens.Token objects
func compileTokenPattern(tokenNames []string) ([]tokens.Token, error) {
	result := make([]tokens.Token, 0, len(tokenNames))

	for _, name := range tokenNames {
		tokenKind, err := tokenNameToKind(name)
		if err != nil {
			return nil, err
		}
		result = append(result, tokens.NewSimpleToken(tokenKind))
	}

	return result, nil
}

// tokenNameToKind converts a token name string to tokens.TokenKind
func tokenNameToKind(name string) (tokens.TokenKind, error) {
	switch name {
	// Digit runs
	case "D1":
		return tokens.D1, nil
	case "D2":
		return tokens.D2, nil
	case "D3":
		return tokens.D3, nil
	case "D4":
		return tokens.D4, nil
	case "D5":
		return tokens.D5, nil
	case "D6":
		return tokens.D6, nil
	case "D7":
		return tokens.D7, nil
	case "D8":
		return tokens.D8, nil
	case "D9":
		return tokens.D9, nil
	case "D10":
		return tokens.D10, nil

	// Character runs
	case "C1":
		return tokens.C1, nil
	case "C2":
		return tokens.C2, nil
	case "C3":
		return tokens.C3, nil
	case "C4":
		return tokens.C4, nil
	case "C5":
		return tokens.C5, nil
	case "C6":
		return tokens.C6, nil
	case "C7":
		return tokens.C7, nil
	case "C8":
		return tokens.C8, nil
	case "C9":
		return tokens.C9, nil
	case "C10":
		return tokens.C10, nil

	// Special characters
	case "Dash":
		return tokens.Dash, nil
	case "Period":
		return tokens.Period, nil
	case "Colon":
		return tokens.Colon, nil
	case "Underscore":
		return tokens.Underscore, nil
	case "Fslash":
		return tokens.Fslash, nil
	case "Bslash":
		return tokens.Bslash, nil
	case "Comma":
		return tokens.Comma, nil
	case "At":
		return tokens.At, nil
	case "Space":
		return tokens.Space, nil
	case "Plus":
		return tokens.Plus, nil
	case "Equal":
		return tokens.Equal, nil
	case "Parenopen":
		return tokens.Parenopen, nil
	case "Parenclose":
		return tokens.Parenclose, nil
	case "Bracketopen":
		return tokens.Bracketopen, nil
	case "Bracketclose":
		return tokens.Bracketclose, nil
	case "Braceopen":
		return tokens.Braceopen, nil
	case "Braceclose":
		return tokens.Braceclose, nil

	// Special tokens
	case "T":
		return tokens.T, nil
	case "Zone":
		return tokens.Zone, nil
	case "Month":
		return tokens.Month, nil
	case "Day":
		return tokens.Day, nil
	case "Apm":
		return tokens.Apm, nil

	// Wildcard tokens
	case "DAny":
		return tokens.DAny, nil
	case "CAny":
		return tokens.CAny, nil

	default:
		return 0, fmt.Errorf("unknown token name: %s", name)
	}
}
