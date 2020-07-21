package policy

import (
	"fmt"
	"io"
	"regexp"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Section describes a section of a policy file
type Section struct {
	Type string
}

// MacroID represents the ID of a macro
type MacroID = string

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID         MacroID
	Expression string
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID         RuleID
	Expression string
	Tags       map[string]string
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
	Rules  []*RuleDefinition
	Macros []*MacroDefinition
}

var ruleIDPattern = `^([a-zA-Z0-9]*_*)*$`

func checkRuleID(ruleID string) bool {
	pattern := regexp.MustCompile(ruleIDPattern)
	return pattern.MatchString(ruleID)
}

// LoadPolicy loads a YAML file and returns a new policy
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
				if !checkRuleID(ruleDef.ID) {
					return nil, fmt.Errorf("rule ID does not match pattern %s", ruleIDPattern)
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
				if !checkRuleID(macroDef.ID) {
					return nil, fmt.Errorf("macro ID does not match pattern %s", ruleIDPattern)
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
