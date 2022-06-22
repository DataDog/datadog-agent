// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"io"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
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

func parsePolicyDef(name string, source string, def *PolicyDef, agentVersion *semver.Version) (*Policy, *multierror.Error) {
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
		if !checkRuleID(macroDef.ID) {
			errs = multierror.Append(errs, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		policy.AddMacro(macroDef)
	}

	for _, ruleDef := range def.Rules {
		constraintSatisfied, err := checkAgentVersionConstraint(ruleDef.AgentVersionConstraint, agentVersion)
		if err != nil {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("failed to parse agent version constraint `%s`", ruleDef.AgentVersionConstraint)})
			continue
		}

		if !constraintSatisfied {
			continue
		}

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

// LoadPolicy load a policy
func LoadPolicy(name string, source string, reader io.Reader, agentVersion *semver.Version) (*Policy, error) {
	var def PolicyDef

	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	policy, errs := parsePolicyDef(name, source, &def, agentVersion)
	if errs.ErrorOrNil() != nil {
		return nil, errs.ErrorOrNil()
	}

	return policy, nil
}

func checkAgentVersionConstraint(constraint string, agentVersion *semver.Version) (bool, error) {
	if agentVersion == nil {
		return true, nil
	}

	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return true, nil
	}

	semverConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, err
	}

	return semverConstraint.Check(agentVersion), nil
}
