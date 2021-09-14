// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"io"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Profile represents a profile file which is composed of a list of rules and macros
type Profile struct {
	Name     string
	Version  string `yaml:"version"`
	Selector string
	Rules    []*RuleDefinition  `yaml:"rules"`
	Macros   []*MacroDefinition `yaml:"macros"`
}

// GetValidMacroAndRules returns valid macro, rules definitions
func (p *Profile) GetValidMacroAndRules() ([]*MacroDefinition, []*RuleDefinition, *multierror.Error) {
	var result *multierror.Error
	var macros []*MacroDefinition
	var rules []*RuleDefinition

	for _, macroDef := range p.Macros {
		if macroDef.ID == "" {
			result = multierror.Append(result, &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)})
			continue
		}
		if !checkRuleID(macroDef.ID) {
			result = multierror.Append(result, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		if macroDef.Expression == "" {
			result = multierror.Append(result, &ErrMacroLoad{Definition: macroDef, Err: errors.New("no expression defined")})
			continue
		}
		macros = append(macros, macroDef)
	}

	for _, ruleDef := range p.Rules {
		// ruleDef.Policy = p

		if ruleDef.ID == "" {
			result = multierror.Append(result, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("no ID defined for rule with expression `%s`", ruleDef.Expression)})
			continue
		}
		if !checkRuleID(ruleDef.ID) {
			result = multierror.Append(result, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		if ruleDef.Expression == "" {
			result = multierror.Append(result, &ErrRuleLoad{Definition: ruleDef, Err: errors.New("no expression defined")})
			continue
		}

		rules = append(rules, ruleDef)
	}

	return macros, rules, result
}

// LoadProfile loads a YAML file and returns a new profile
func LoadProfile(r io.Reader, name string) (*Profile, error) {
	profile := &Profile{Name: name}

	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(profile); err != nil {
		return nil, &ErrProfileLoad{Name: name, Err: err}
	}

	return profile, nil
}
