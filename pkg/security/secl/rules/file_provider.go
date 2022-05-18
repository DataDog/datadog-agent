// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const defaultPolicyFile = "default.policy"

// Policy represents a policy file definition
type PolicyDef struct {
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
}

// PolicyFileProvider defines a file provider
type PolicyFileProvider struct {
	Filename          string
	onPolicyChangedCb func(_ *Policy)
}

// GetValidMacroAndRules returns valid macro, rules definitions
func (p *PolicyFileProvider) parseDef(name string, def *PolicyDef) (*Policy, *multierror.Error) {
	var errs *multierror.Error

	policy := &Policy{
		Name:    name,
		Source:  "file",
		Version: def.Version,
	}

	for _, macroDef := range def.Macros {
		if macroDef.ID == "" {
			errs = multierror.Append(errs, &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)})
			continue
		}
		if !checkRuleID(macroDef.ID) {
			errs = multierror.Append(errs, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		policy.AddMacro(macroDef)
	}

	for _, ruleDef := range def.Rules {
		if ruleDef.ID == "" {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("no ID defined for rule with expression `%s`", ruleDef.Expression)})
			continue
		}
		if !checkRuleID(ruleDef.ID) {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
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

// LoadPolicy loads a YAML file and returns a new policy
func (p *PolicyFileProvider) LoadPolicy() (*Policy, error) {
	f, err := os.Open(p.Filename)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: p.Filename, Err: err}
	}
	defer f.Close()

	var def PolicyDef

	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: p.Filename, Err: err}
	}

	policy, errs := p.parseDef(filepath.Base(p.Filename), &def)
	if errs.ErrorOrNil() != nil {
		return nil, errs
	}

	if p.onPolicyChangedCb != nil {
		p.onPolicyChangedCb(policy)
	}

	return policy, nil
}

func (p *PolicyFileProvider) SetOnPolicyChangedCb(cb func(*Policy)) {
	p.onPolicyChangedCb = cb
}

func NewPolicyFileProvider(filename string) *PolicyFileProvider {
	return &PolicyFileProvider{
		Filename: filename,
	}
}
