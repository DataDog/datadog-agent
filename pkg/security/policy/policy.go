package policy

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Section struct {
	Type string
}

type Policy struct {
	Rules []*ast.Rule
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
				rule := &ast.Rule{}
				if err := mapstructure.Decode(value, rule); err != nil {
					return nil, errors.Wrap(err, "invalid policy")
				}
				policy.Rules = append(policy.Rules, rule)
			default:
				return nil, fmt.Errorf("invalid policy item '%s'", key)
			}
		}
	}

	return policy, nil
}
