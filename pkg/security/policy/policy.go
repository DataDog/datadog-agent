package policy

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
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
	Ast        *ast.Macro `mapstructure:",omitempty"`
}

// LoadAST - Loads the AST of the macro
func (md *MacroDefinition) LoadAST() error {
	astMacro, err := ast.ParseMacro(md.Expression)
	if err != nil {
		return err
	}
	md.Ast = astMacro
	return nil
}

type RuleDefinition struct {
	ID         string
	Expression string
	Tags       map[string]string
	Ast        *ast.Rule `mapstructure:",omitempty"`
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

// LoadAST - Loads the AST of the rule
func (rd *RuleDefinition) LoadAST() error {
	astRule, err := ast.ParseRule(rd.Expression)
	if err != nil {
		return err
	}
	rd.Ast = astRule
	return nil
}

type Policy struct {
	Rules  []*RuleDefinition
	Macros []*MacroDefinition
}

// PolicySet - Regroups the macros and the rules of multiple policies in a usable format by the ruleset
type PolicySet struct {
	// Rules holds the list of merged rule definitions, indexed by their IDs
	Rules  map[string]*RuleDefinition
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

// GetMacroASTs - Returns the list of Macro ASTs of the merged policy set, indexed by their IDs.
func (ps *PolicySet) GetMacroASTs() map[string]*ast.Macro {
	macros := make(map[string]*ast.Macro)
	for _, macro := range ps.Macros {
		macros[macro.ID] = macro.Ast
	}
	return macros
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

				if err := ruleDef.LoadAST(); err != nil {
					return nil, errors.Wrap(err, "couldn't load rule ast")
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

				if err := macroDef.LoadAST(); err != nil {
					return nil, errors.Wrap(err, "couldn't load macro ast")
				}

				policy.Macros = append(policy.Macros, macroDef)

			default:
				return nil, fmt.Errorf("invalid policy item '%s'", key)
			}
		}
	}

	return policy, nil
}
