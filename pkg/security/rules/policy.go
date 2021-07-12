// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Policy represents a policy file which is composed of a list of rules and macros
type Policy struct {
	Name    string
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
}

var ruleIDPattern = `^([a-zA-Z0-9]*_*)*$`

func checkRuleID(ruleID string) bool {
	pattern := regexp.MustCompile(ruleIDPattern)
	return pattern.MatchString(ruleID)
}

// GetValidMacroAndRules returns valid macro, rules definitions
func (p *Policy) GetValidMacroAndRules() ([]*MacroDefinition, []*RuleDefinition, *multierror.Error) {
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
		ruleDef.Policy = p

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

// LoadPolicy loads a YAML file and returns a new policy
func LoadPolicy(r io.Reader, name string) (*Policy, error) {
	policy := &Policy{Name: name}

	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(policy); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	return policy, nil
}

// LoadPolicies loads the policies listed in the configuration and apply them to the given ruleset
func LoadPolicies(policiesDir string, ruleSet *RuleSet) *multierror.Error {
	var (
		result   *multierror.Error
		allRules []*RuleDefinition
	)

	policyFiles, err := ioutil.ReadDir(policiesDir)
	if err != nil {
		return multierror.Append(result, ErrPoliciesLoad{Name: policiesDir, Err: err})
	}
	sort.Slice(policyFiles, func(i, j int) bool { return policyFiles[i].Name() < policyFiles[j].Name() })

	// Load and parse policies
	for _, policyPath := range policyFiles {
		filename := policyPath.Name()

		// policy path extension check
		if filepath.Ext(filename) != ".policy" {
			ruleSet.logger.Debugf("ignoring file `%s` wrong extension `%s`", policyPath.Name(), filepath.Ext(filename))
			continue
		}

		// Open policy path
		f, err := os.Open(filepath.Join(policiesDir, filename))
		if err != nil {
			result = multierror.Append(result, &ErrPolicyLoad{Name: filename, Err: err})
			continue
		}
		defer f.Close()

		// Parse policy file
		policy, err := LoadPolicy(f, filepath.Base(filename))
		if err != nil {
			result = multierror.Append(result, err)
			continue
		}

		// Add policy version for logging purposes
		ruleSet.AddPolicyVersion(filename, policy.Version)

		macros, rules, mErr := policy.GetValidMacroAndRules()
		if mErr.ErrorOrNil() != nil {
			result = multierror.Append(result, mErr)
		}

		if len(macros) > 0 {
			// Add the macros to the ruleset and generate macros evaluators
			if err := ruleSet.AddMacros(macros); err != nil {
				result = multierror.Append(result, err)
			}
		}

		// aggregates them as we may need to have all the macro before compiling
		if len(rules) > 0 {
			allRules = append(allRules, rules...)
		}
	}

	// Add rules to the ruleset and generate rules evaluators
	if err := ruleSet.AddRules(allRules); err.ErrorOrNil() != nil {
		result = multierror.Append(result, err)
	}

	return result
}
