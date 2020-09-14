// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package policy

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Policy represents a policy file which is composed of a list of rules and macros
type Policy struct {
	Version string                   `yaml:"version"`
	Rules   []*rules.RuleDefinition  `yaml:"rules"`
	Macros  []*rules.MacroDefinition `yaml:"macros"`
}

var ruleIDPattern = `^([a-zA-Z0-9]*_*)*$`

func checkRuleID(ruleID string) bool {
	pattern := regexp.MustCompile(ruleIDPattern)
	return pattern.MatchString(ruleID)
}

// LoadPolicy loads a YAML file and returns a new policy
func LoadPolicy(r io.Reader) (*Policy, error) {
	policy := &Policy{}

	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&policy); err != nil {
		return nil, errors.Wrap(err, "failed to load policy")
	}

	for _, macroDef := range policy.Macros {
		if macroDef.ID == "" {
			return nil, errors.New("macro has no name")
		}
		if !checkRuleID(macroDef.ID) {
			return nil, fmt.Errorf("macro ID does not match pattern %s", ruleIDPattern)
		}

		if macroDef.Expression == "" {
			return nil, errors.New("macro has no expression")
		}
	}

	for _, ruleDef := range policy.Rules {
		if ruleDef.ID == "" {
			return nil, errors.New("rule has no name")
		}
		if !checkRuleID(ruleDef.ID) {
			return nil, fmt.Errorf("rule ID does not match pattern %s", ruleIDPattern)
		}

		if ruleDef.Expression == "" {
			return nil, errors.New("rule has no expression")
		}
	}

	return policy, nil
}

// LoadPolicies loads the policies listed in the configuration and apply them to the given ruleset
func LoadPolicies(config *config.Config, ruleSet *rules.RuleSet) error {
	var result *multierror.Error

	policyFiles, err := ioutil.ReadDir(config.PoliciesDir)
	if err != nil {
		return err
	}

	// Load and parse policies
	for _, policyPath := range policyFiles {
		filename := policyPath.Name()

		// policy path extension check
		if filepath.Ext(filename) != ".policy" {
			log.Debugf("ignoring file `%s` wrong extension `%s`", policyPath.Name(), filepath.Ext(filename))
			continue
		}

		// Open policy path
		f, err := os.Open(filepath.Join(config.PoliciesDir, filename))
		if err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "failed to load policy `%s`", policyPath))
			continue
		}

		// Parse policy file
		policy, err := LoadPolicy(f)
		if err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "failed to load policy `%s`", policyPath))
			continue
		}

		// Add the macros to the ruleset and generate macros evaluators
		if err := ruleSet.AddMacros(policy.Macros); err != nil {
			result = multierror.Append(result, err)
		}

		// Add rules to the ruleset and generate rules evaluators
		if err := ruleSet.AddRules(policy.Rules); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}
