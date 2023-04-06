package rules

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cast"
	"reflect"
)

type PolicySet struct {
	RuleSets map[string]*RuleSet
}

// NewPolicySet returns a new policy set for the specified data model
func NewPolicySet(defaultRS *RuleSet, threatScoreRS *RuleSet) *PolicySet {
	ruleSets := make(map[string]*RuleSet)
	ruleSets[DefaultRuleSetName] = defaultRS
	ruleSets[ThreatScoreRuleSetName] = threatScoreRS

	return &PolicySet{RuleSets: ruleSets}
}

// GetPolicies returns the policies
func (ps *PolicySet) GetPolicies() []*Policy {
	var policiesList []*Policy
	for _, rs := range ps.RuleSets {
		policiesList = append(policiesList, rs.policies...)
	}
	return policiesList
}

// LoadPolicies loads policies from the provided policy loader
func (ps *PolicySet) LoadPolicies(loader *PolicyLoader, opts PolicyLoaderOpts) *multierror.Error {
	var (
		errs       *multierror.Error
		allRules   []*RuleDefinition
		allMacros  []*MacroDefinition
		macroIndex = make(map[string]*MacroDefinition)
		ruleIndex  = make(map[string]*RuleDefinition)
	)

	parsingContext := ast.NewParsingContext()

	policies, err := loader.LoadPolicies(opts)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	for _, rs := range ps.RuleSets {
		rs.policies = policies
	}

	for _, policy := range policies {
		for _, macro := range policy.Macros {
			if existingMacro := macroIndex[macro.ID]; existingMacro != nil {
				if err := existingMacro.MergeWith(macro); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				macroIndex[macro.ID] = macro
				allMacros = append(allMacros, macro)
			}
		}

		for _, rule := range policy.Rules {
			if existingRule := ruleIndex[rule.ID]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				ruleIndex[rule.ID] = rule
				allRules = append(allRules, rule)
			}
		}

		for _, rule := range policy.SpecialRules[ThreatScoreRuleSetName] {
			if existingRule := ruleIndex[rule.ID]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				ruleIndex[rule.ID] = rule
			}
		}
	}

	// Add the macros to the ruleset and generate macros evaluators
	for _, rs := range ps.RuleSets {
		if err := rs.AddMacros(parsingContext, allMacros); err.ErrorOrNil() != nil {
			errs = multierror.Append(errs, err)
		}
	}

	for _, rule := range allRules {
		for _, action := range rule.Actions {
			if err := action.Check(); err != nil {
				errs = multierror.Append(errs, fmt.Errorf("invalid action: %w", err))
				continue
			}

			if action.Set != nil {
				varName := action.Set.Name
				if action.Set.Scope != "" {
					varName = string(action.Set.Scope) + "." + varName
				}

				for _, rs := range ps.RuleSets {
					if _, err := rs.eventCtor().GetFieldValue(varName); err == nil {
						errs = multierror.Append(errs, fmt.Errorf("variable '%s' conflicts with field", varName))
						continue
					}

					if _, found := rs.evalOpts.Constants[varName]; found {
						errs = multierror.Append(errs, fmt.Errorf("variable '%s' conflicts with constant", varName))
						continue
					}
				}

				var variableValue interface{}

				if action.Set.Value != nil {
					switch value := action.Set.Value.(type) {
					case int:
						action.Set.Value = []int{value}
					case string:
						action.Set.Value = []string{value}
					case []interface{}:
						if len(value) == 0 {
							errs = multierror.Append(errs, fmt.Errorf("unable to infer item type for '%s'", action.Set.Name))
							continue
						}

						switch arrayType := value[0].(type) {
						case int:
							action.Set.Value = cast.ToIntSlice(value)
						case string:
							action.Set.Value = cast.ToStringSlice(value)
						default:
							errs = multierror.Append(errs, fmt.Errorf("unsupported item type '%s' for array '%s'", reflect.TypeOf(arrayType), action.Set.Name))
							continue
						}
					}

					variableValue = action.Set.Value
				} else if action.Set.Field != "" {
					for _, rs := range ps.RuleSets {

						kind, err := rs.eventCtor().GetFieldType(action.Set.Field)
						if err != nil {
							errs = multierror.Append(errs, fmt.Errorf("failed to get field '%s': %w", action.Set.Field, err))
							continue
						}

						switch kind {
						case reflect.String:
							variableValue = []string{}
						case reflect.Int:
							variableValue = []int{}
						case reflect.Bool:
							variableValue = false
						default:
							errs = multierror.Append(errs, fmt.Errorf("unsupported field type '%s' for variable '%s'", kind, action.Set.Name))
							continue
						}
					}
				}

				var variable eval.VariableValue
				var variableProvider VariableProvider

				if action.Set.Scope != "" {
					for _, rs := range ps.RuleSets {

						stateScopeBuilder := rs.opts.StateScopes[action.Set.Scope]
						if stateScopeBuilder == nil {
							errs = multierror.Append(errs, fmt.Errorf("invalid scope '%s'", action.Set.Scope))
							continue
						}

						if _, found := rs.scopedVariables[action.Set.Scope]; !found {
							rs.scopedVariables[action.Set.Scope] = stateScopeBuilder()
						}

						variableProvider = rs.scopedVariables[action.Set.Scope]
					}
				} else {
					for _, rs := range ps.RuleSets {
						variableProvider = &rs.globalVariables
					}
				}

				variable, err := variableProvider.GetVariable(action.Set.Name, variableValue)
				if err != nil {
					errs = multierror.Append(errs, fmt.Errorf("invalid type '%s' for variable '%s': %w", reflect.TypeOf(action.Set.Value), action.Set.Name, err))
					continue
				}

				for _, rs := range ps.RuleSets {
					if existingVariable := rs.evalOpts.VariableStore.Get(varName); existingVariable != nil && reflect.TypeOf(variable) != reflect.TypeOf(existingVariable) {
						errs = multierror.Append(errs, fmt.Errorf("conflicting types for variable '%s'", varName))
						continue
					}

					rs.evalOpts.VariableStore.Add(varName, variable)
				}
			}
		}
	}

	// Add rules to the ruleset and generate rules evaluators
	for _, rs := range ps.RuleSets {

		if err := rs.AddRules(parsingContext, allRules); err.ErrorOrNil() != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs
}
