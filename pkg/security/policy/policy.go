package policy

import (
	"fmt"
	"io"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var (
	ErrUnnamedRule = errors.New("policy has no name")
	ErrEmptyRule   = errors.New("policy has no expression")
)

type Section struct {
	Type string
}

type RuleDefinition struct {
	ID         string
	Expression string
}

type Policy struct {
	Rules []*RuleDefinition
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
				ruleDef := &RuleDefinition{}
				if err := mapstructure.Decode(value, ruleDef); err != nil {
					return nil, errors.Wrap(err, "invalid policy")
				}

				if ruleDef.ID == "" {
					return nil, ErrUnnamedRule
				}

				if ruleDef.Expression == "" {
					return nil, ErrEmptyRule
				}

				policy.Rules = append(policy.Rules, ruleDef)
			default:
				return nil, fmt.Errorf("invalid policy item '%s'", key)
			}
		}
	}

	return policy, nil
}
