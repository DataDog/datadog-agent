// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"fmt"
	"regexp"
)

// ProcessingRules defines at global level or integration level which apply
// operations on logs.
type ProcessingRules struct {
	Rules []*ProcessingRule
}

// ProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type ProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string `mapstructure:"replace_placeholder" json:"replace_placeholder"`
	Pattern            string
	// computed properties
	Regex       *regexp.Regexp
	Placeholder []byte
}

// Validate validates the rules and raises an error if one is misconfigured.
// Each processing rule must have:
// - a valid name
// - a valid type
// - a valid pattern that compiles
func (pr *ProcessingRules) Validate() error {
	for _, rule := range pr.Rules {
		if rule.Name == "" {
			return fmt.Errorf("all processing rules must have a name")
		}

		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			break
		case "":
			return fmt.Errorf("type must be set for processing rule `%s`", rule.Name)
		default:
			return fmt.Errorf("type %s is not supported for processing rule `%s`", rule.Type, rule.Name)
		}

		if rule.Pattern == "" {
			return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
		}
		_, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern %s for processing rule: %s", rule.Pattern, rule.Name)
		}
	}
	return nil
}

// Compile compiles all processing rule regular expressions.
func (pr *ProcessingRules) Compile() error {
	for _, rule := range pr.Rules {
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
