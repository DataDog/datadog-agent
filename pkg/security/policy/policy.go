package policy

import (
	"fmt"
	"io"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Section struct {
	Type string
}

type MacroDefinition struct {
	ID         string
	Expression string
}

type RuleDefinition struct {
	ID         string
	Expression string
	Tags       map[string]string
}

// GetTags - Returns the tags of the rule
func (rd *RuleDefinition) GetTags() []string {
	tags := []string{}
	for k, v := range rd.Tags {
		tags = append(
			tags,
			fmt.Sprintf("%s:%s", k, v))
	}
	return tags
}

type Policy struct {
	Rules  []*RuleDefinition
	Macros []*MacroDefinition
}

// PolicySet - Regroups the macros and the rules of multiple policies in a usable format by the ruleset
type PolicySet struct {
	// Rules holds the list of merged rule definitions, indexed by their IDs
	Rules map[string]*RuleDefinition
	// Macros holds the list of merged macro asts, indexed by their ID
	Macros map[string]*MacroDefinition
}

// AddPolicy - Includes a policy in a policy set
func (ps *PolicySet) AddPolicy(policy *Policy) error {
	// Merge macros
	if ps.Macros == nil {
		ps.Macros = make(map[string]*MacroDefinition)
	}

	// Make sure that there is only one definition of the macro
	for _, newMacro := range policy.Macros {
		if _, exists := ps.Macros[newMacro.ID]; exists {
			return fmt.Errorf("found multiple definition of the macro %s", newMacro.ID)
		}
		ps.Macros[newMacro.ID] = newMacro
	}

	// Merge rules
	if ps.Rules == nil {
		ps.Rules = make(map[string]*RuleDefinition)
	}

	// Make sure that the new rules do not have conflicting IDs
	for _, newRule := range policy.Rules {
		if _, exists := ps.Rules[newRule.ID]; exists {
			return fmt.Errorf("found multiple definition of the rule %s", newRule.ID)
		}
		ps.Rules[newRule.ID] = newRule
	}
	return nil
}

func LoadPolicy(r io.Reader) (*Policy, error) {
	var mapSlice []map[string]interface{}

	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&mapSlice); err != nil {
		return nil, errors.Wrap(err, "failed to load policy")
	}

	policy := &Policy{}
	for _, m := range mapSlice {
		if len(m) != 1 {
			return nil, errors.New("invalid item in policy")
		}

		for key, value := range m {
			switch key {
			case "rule":
				ruleDef := &RuleDefinition{
					Tags: make(map[string]string),
				}
				if err := mapstructure.Decode(value, ruleDef); err != nil {
					return nil, errors.Wrap(err, "invalid policy")
				}

				if ruleDef.ID == "" {
					return nil, errors.New("rule has no name")
				}

				if ruleDef.Expression == "" {
					return nil, errors.New("rule has no expression")
				}

				policy.Rules = append(policy.Rules, ruleDef)

			case "macro":
				macroDef := &MacroDefinition{}

				if err := mapstructure.Decode(value, macroDef); err != nil {
					return nil, errors.Wrap(err, "invalid policy")
				}

				if macroDef.ID == "" {
					return nil, errors.New("macro has no name")
				}

				if macroDef.Expression == "" {
					return nil, errors.New("macro has no expression")
				}

				policy.Macros = append(policy.Macros, macroDef)

			default:
				return nil, fmt.Errorf("invalid policy item '%s'", key)
			}
		}
	}

	return policy, nil
}
