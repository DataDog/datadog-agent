// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

const defaultPolicy = "default.policy"

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
	var (
		result *multierror.Error
		macros []*MacroDefinition
		rules  []*RuleDefinition
	)

	for _, macroDef := range p.Macros {
		if macroDef.ID == "" {
			result = multierror.Append(result, &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)})
			continue
		}
		if !checkRuleID(macroDef.ID) {
			result = multierror.Append(result, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
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

		if ruleDef.Expression == "" && !ruleDef.Disabled {
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
		result     *multierror.Error
		allRules   []*RuleDefinition
		allMacros  []*MacroDefinition
		macroIndex = make(map[string]*MacroDefinition)
		ruleIndex  = make(map[string]*RuleDefinition)
	)

	policyFiles, err := os.ReadDir(policiesDir)
	if err != nil {
		return multierror.Append(result, ErrPoliciesLoad{Name: policiesDir, Err: err})
	}
	sort.Slice(policyFiles, func(i, j int) bool {
		switch {
		case policyFiles[i].Name() == defaultPolicy:
			return true
		case policyFiles[j].Name() == defaultPolicy:
			return false
		default:
			return policyFiles[i].Name() < policyFiles[j].Name()
		}
	})

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
			for _, macro := range macros {
				if existingMacro := macroIndex[macro.ID]; existingMacro != nil {
					if err := existingMacro.MergeWith(macro); err != nil {
						result = multierror.Append(result, err)
					}
				} else {
					macroIndex[macro.ID] = macro
					allMacros = append(allMacros, macro)
				}
			}
		}

		// aggregates them as we may need to have all the macro before compiling
		if len(rules) > 0 {
			for _, rule := range rules {
				if existingRule := ruleIndex[rule.ID]; existingRule != nil {
					if err := existingRule.MergeWith(rule); err != nil {
						result = multierror.Append(result, err)
					}
				} else {
					ruleIndex[rule.ID] = rule
					allRules = append(allRules, rule)
				}
			}
		}
	}

	// Add the macros to the ruleset and generate macros evaluators
	if mErr := ruleSet.AddMacros(allMacros); mErr.ErrorOrNil() != nil {
		result = multierror.Append(result, err)
	}

	for _, rule := range allRules {
		for _, action := range rule.Actions {
			if err := action.Check(); err != nil {
				result = multierror.Append(result, fmt.Errorf("invalid action: %w", err))
			}

			if action.Set != nil {
				varName := action.Set.Name
				if action.Set.Scope != "" {
					varName = string(action.Set.Scope) + "." + varName
				}

				if _, err := ruleSet.model.NewEvent().GetFieldValue(varName); err == nil {
					result = multierror.Append(result, fmt.Errorf("variable '%s' conflicts with field", varName))
					continue
				}

				if _, found := ruleSet.opts.Constants[varName]; found {
					result = multierror.Append(result, fmt.Errorf("variable '%s' conflicts with constant", varName))
					continue
				}

				var scope StateScope

				if action.Set.Scope != "" {
					if scope = ruleSet.opts.StateScopes[action.Set.Scope]; scope == nil {
						result = multierror.Append(result, fmt.Errorf("invalid scope '%s'", action.Set.Scope))
						continue
					}
				} else {
					scope = ruleSet
				}

				var variable eval.VariableValue
				var variableValue interface{}

				if action.Set.Value != nil {
					switch value := action.Set.Value.(type) {
					case []interface{}:
						if len(value) == 0 {
							result = multierror.Append(result, fmt.Errorf("unable to infer item type for '%s'", action.Set.Name))
							continue
						}

						switch arrayType := value[0].(type) {
						case int:
							action.Set.Value = cast.ToIntSlice(value)
						case string:
							action.Set.Value = cast.ToStringSlice(value)
						default:
							result = multierror.Append(result, fmt.Errorf("unsupported array item type '%s' for array '%s'", reflect.TypeOf(arrayType), action.Set.Name))
							continue
						}
					}

					variableValue = action.Set.Value
				} else if action.Set.Field != "" {
					kind, err := ruleSet.eventCtor().GetFieldType(action.Set.Field)
					if err != nil {
						result = multierror.Append(result, fmt.Errorf("failed to get field '%s': %w", action.Set.Field, err))
						continue
					}

					switch kind {
					case reflect.String:
						if action.Set.Append {
							variableValue = []string{""}
						} else {
							variableValue = ""
						}
					case reflect.Int:
						if action.Set.Append {
							variableValue = []int{0}
						} else {
							variableValue = 0
						}
						variableValue = 0
					case reflect.Bool:
						variableValue = false
					default:
						result = multierror.Append(result, fmt.Errorf("unsupported field type '%s' for variable '%s'", kind, action.Set.Name))
						continue
					}
				}

				variable, err = scope.GetVariable(action.Set.Name, variableValue)
				if err != nil {
					result = multierror.Append(result, fmt.Errorf("invalid type '%s' for variable '%s': %w", reflect.TypeOf(action.Set.Value), action.Set.Name, err))
					continue
				}

				if existingVariable, found := ruleSet.opts.Variables[varName]; found && reflect.TypeOf(variable) != reflect.TypeOf(existingVariable) {
					result = multierror.Append(result, fmt.Errorf("variable '%s' conflicts with variable (%s != %s)", varName, reflect.TypeOf(variable), reflect.TypeOf(existingVariable)))
					continue
				}

				ruleSet.opts.Variables[varName] = variable
			}
		}
	}

	// Add rules to the ruleset and generate rules evaluators
	if err := ruleSet.AddRules(allRules); err.ErrorOrNil() != nil {
		result = multierror.Append(result, err)
	}

	return result
}
