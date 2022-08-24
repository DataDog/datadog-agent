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
	Name    string
	Source  string
	Version string
	Rules   []*RuleDefinition
	Macros  []*MacroDefinition
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

func parsePolicyDef(name string, source string, def *PolicyDef, filters []RuleFilter) (*Policy, *multierror.Error) {
	var errs *multierror.Error

	policy := &Policy{
		Name:    name,
		Source:  source,
		Version: def.Version,
	}

	for _, macroDef := range def.Macros {
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

LOOP:
	for _, ruleDef := range def.Rules {
		for _, filter := range filters {
			isAccepted, err := filter.IsAccepted(ruleDef)
			if err != nil {
				errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("error when evaluating one of the rules filters: %w", err)})
			}
			if !isAccepted {
				continue LOOP
			}
		}

		if ruleDef.ID == "" {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("no ID defined for rule with expression `%s`", ruleDef.Expression)})
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

	return policy, errs
}

// LoadPolicy load a policy
func LoadPolicy(name string, source string, reader io.Reader, filters []RuleFilter) (*Policy, error) {
	var def PolicyDef

	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	policy, errs := parsePolicyDef(name, source, &def, filters)
	if errs.ErrorOrNil() != nil {
		return nil, errs.ErrorOrNil()
	}

	return policy, nil
}
