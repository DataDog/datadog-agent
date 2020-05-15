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

type Policy struct {
	Rules  []*RuleDefinition
	Macros []*MacroDefinition
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
