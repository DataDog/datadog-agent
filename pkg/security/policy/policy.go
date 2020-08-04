// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package policy

import (
	"fmt"
	"io"
	"regexp"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// MacroID represents the ID of a macro
type MacroID = string

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID         MacroID `yaml:"id"`
	Expression string  `yaml:"expression"`
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID         RuleID            `yaml:"id"`
	Expression string            `yaml:"expression"`
	Tags       map[string]string `yaml:"tags"`
}

// GetTags returns the tags associated to a rule
func (rd *RuleDefinition) GetTags() []string {
	tags := []string{}
	for k, v := range rd.Tags {
		tags = append(
			tags,
			fmt.Sprintf("%s:%s", k, v))
	}
	return tags
}

// Policy represents a policy file which is composed of a list of rules and macros
type Policy struct {
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
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
