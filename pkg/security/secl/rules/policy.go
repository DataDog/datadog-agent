// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"
	"io"

	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
)

// PolicyDef represents a policy file definition
type PolicyDef struct {
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
}

// Policy represents a policy file which is composed of a list of rules and macros
type Policy struct {
	Name       string
	Source     string
	Version    string
	Rules      []*RuleDefinition
	Macros     []*MacroDefinition
	IsInternal bool
}

// AddMacro add a macro to the policy
func (p *Policy) AddMacro(def *MacroDefinition) {
	p.Macros = append(p.Macros, def)
}

// AddRule adds a rule to the policy
func (p *Policy) AddRule(def *RuleDefinition) {
	def.Policy = p
	p.Rules = append(p.Rules, def)
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

	var skipped []struct {
		ruleDefinition *RuleDefinition
		err            error
	}

RULES:
	for _, ruleDef := range def.Rules {
		// set the policy so that when we parse the errors we can get the policy associated
		ruleDef.Policy = policy

		for _, filter := range ruleFilters {
			isRuleAccepted, err := filter.IsRuleAccepted(ruleDef)
			if err != nil {
				errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: err})
			}

			if !isRuleAccepted {
				// we do not fail directly because one of the rules with the same id can load properly
				if _, ok := filter.(*AgentVersionFilter); ok {
					skipped = append(skipped, struct {
						ruleDefinition *RuleDefinition
						err            error
					}{ruleDefinition: ruleDef, err: ErrRuleAgentVersion})
				} else if _, ok := filter.(*SECLRuleFilter); ok {
					skipped = append(skipped, struct {
						ruleDefinition *RuleDefinition
						err            error
					}{ruleDefinition: ruleDef, err: ErrRuleAgentFilter})
				}

				continue RULES
			}
		}

		if ruleDef.ID == "" {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: ErrRuleWithoutID})
			continue
		}
		if !validators.CheckRuleID(ruleDef.ID) {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: ErrRuleIDPattern})
			continue
		}

		if ruleDef.Expression == "" && !ruleDef.Disabled {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: ErrRuleWithoutExpression})
			continue
		}

		policy.AddRule(ruleDef)
	}

LOOP:
	for _, s := range skipped {
		// For every skipped rule, if it doesn't match an ID of a policy rule, add an error.
		for _, r := range policy.Rules {
			if s.ruleDefinition.ID == r.ID {
				continue LOOP
			}
		}

		// do not report filtered rules
		if !errors.Is(s.err, ErrRuleAgentFilter) {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: s.ruleDefinition, Err: s.err})
		}
	}

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
