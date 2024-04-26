// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
)

// EvaluationSet defines an evalation set
type EvaluationSet struct {
	RuleSets map[eval.RuleSetTagValue]*RuleSet
}

// NewEvaluationSet returns a new policy set for the specified data model
func NewEvaluationSet(ruleSetsToInclude []*RuleSet) (*EvaluationSet, error) {
	ruleSets := make(map[string]*RuleSet)

	if len(ruleSetsToInclude) == 0 {
		return nil, ErrNoRuleSetsInEvaluationSet
	}

	for _, ruleSet := range ruleSetsToInclude {
		if ruleSet != nil {
			ruleSets[ruleSet.GetRuleSetTag()] = ruleSet
		} else {
			return nil, fmt.Errorf("nil rule set with tag value %s provided include in an evaluation set", ruleSet.GetRuleSetTag())
		}
	}

	return &EvaluationSet{RuleSets: ruleSets}, nil
}

// GetPolicies returns the policies
func (es *EvaluationSet) GetPolicies() []*Policy {
	var policies []*Policy

	seen := make(map[string]bool)

	for _, rs := range es.RuleSets {
		for _, policy := range rs.policies {
			if _, ok := seen[policy.Name]; !ok {
				seen[policy.Name] = true
				policies = append(policies, policy)
			}
		}
	}

	return policies
}

type ruleIndexEntry struct {
	value  eval.RuleSetTagValue
	ruleID eval.RuleID
}

// LoadPolicies load policies
func (es *EvaluationSet) LoadPolicies(loader *PolicyLoader, opts PolicyLoaderOpts) *multierror.Error {
	var (
		errs       *multierror.Error
		rules      = make(map[eval.RuleSetTagValue][]*RuleDefinition)
		allMacros  []*MacroDefinition
		macroIndex = make(map[string]*MacroDefinition)
		rulesIndex = make(map[ruleIndexEntry]*RuleDefinition)
	)

	parsingContext := ast.NewParsingContext()

	policies, err := loader.LoadPolicies(opts)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	for _, rs := range es.RuleSets {
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
			tagValue, _ := rule.GetTag("ruleset")
			if tagValue == "" {
				tagValue = DefaultRuleSetTagValue
			}

			if existingRule := rulesIndex[ruleIndexEntry{tagValue, rule.ID}]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				rulesIndex[ruleIndexEntry{tagValue, rule.ID}] = rule
				rules[tagValue] = append(rules[tagValue], rule)
			}
		}
	}

	for ruleSetTagValue, rs := range es.RuleSets {
		for rulesIndexTagValue, ruleList := range rules {
			if rulesIndexTagValue == ruleSetTagValue {
				// Add the macros to the ruleset and generate macros evaluators
				if err := rs.AddMacros(parsingContext, allMacros); err.ErrorOrNil() != nil {
					errs = multierror.Append(errs, err)
				}

				if err := rs.populateFieldsWithRuleActionsData(ruleList, opts); err.ErrorOrNil() != nil {
					errs = multierror.Append(errs, err)
				}

				// Add rules to the ruleset and generate rules evaluators
				if err := rs.AddRules(parsingContext, ruleList); err.ErrorOrNil() != nil {
					errs = multierror.Append(errs, err)
				}
			}
		}
	}

	for ruleSetTag, ruleSet := range es.RuleSets {
		if ruleSet == nil {
			delete(es.RuleSets, ruleSetTag)
		}
	}

	return errs
}
