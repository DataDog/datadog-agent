// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"errors"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"
)

// PolicyDef represents a policy file definition
type PolicyDef struct {
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
}

// Policy represents a policy file which is composed of a list of rules and macros
type Policy struct {
	Name        string
	Source      string
	Version     string
	Rules       []*RuleDefinition
	Macros      []*MacroDefinition
	RuleSkipped []*RuleDefinition
}

// AddMacro add a macro to the policy
func (p *Policy) AddMacro(def *MacroDefinition) {
	p.Macros = append(p.Macros, def)
}

// AddRule add a rule to the policy
func (p *Policy) AddRule(def *RuleDefinition) {
	def.Policy = p
	p.Rules = append(p.Rules, def)
}

// AddRule add a rule to the policy
func (p *Policy) SkipRule(def *RuleDefinition) {
	def.Policy = p
	p.RuleSkipped = append(p.RuleSkipped, def)
}

// remove rules that have a valid version from the skipped list
func cleanupRuleSkipped(policy *Policy) {
	var skipped []*RuleDefinition

LOOP:
	for _, s := range policy.RuleSkipped {
		for _, r := range policy.Rules {
			if s.ID == r.ID {
				continue LOOP
			}
		}
		skipped = append(skipped, s)
	}

	policy.RuleSkipped = skipped
}

func parsePolicyDef(name string, source string, def *PolicyDef, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	var errs *multierror.Error

	policy := &Policy{
		Name:    name,
		Source:  source,
		Version: def.Version,
	}

MACROS:
	for _, macroDef := range def.Macros {
		for _, filter := range macroFilters {
			isMacroAccepted, err := filter.IsMacroAccepted(macroDef)
			if err != nil {
				errs = multierror.Append(errs, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("error when evaluating one of the macro filters: %w", err)})
			}
			if !isMacroAccepted {
				continue MACROS
			}
		}

		if macroDef.ID == "" {
			errs = multierror.Append(errs, &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)})
			continue
		}
		if !validators.CheckRuleID(macroDef.ID) {
			errs = multierror.Append(errs, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", validators.RuleIDPattern)})
			continue
		}

		policy.AddMacro(macroDef)
	}

RULES:
	for _, ruleDef := range def.Rules {
		for _, filter := range ruleFilters {
			isRuleAccepted, err := filter.IsRuleAccepted(ruleDef)
			if err != nil {
				errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("error when evaluating one of the rule filters: %w", err)})
			}
			if !isRuleAccepted {
				policy.SkipRule(ruleDef)
				continue RULES
			}
		}

		if ruleDef.ID == "" {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: errors.New("no ID defined for rule with expression")})
			continue
		}
		if !validators.CheckRuleID(ruleDef.ID) {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("ID does not match pattern `%s`", validators.RuleIDPattern)})
			continue
		}

		if ruleDef.Expression == "" && !ruleDef.Disabled {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: errors.New("no expression defined")})
			continue
		}

		policy.AddRule(ruleDef)
	}

	cleanupRuleSkipped(policy)

	return policy, errs.ErrorOrNil()
}

// LoadPolicy load a policy
func LoadPolicy(name string, source string, reader io.Reader, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	var def PolicyDef

	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	return parsePolicyDef(name, source, &def, macroFilters, ruleFilters)
}
