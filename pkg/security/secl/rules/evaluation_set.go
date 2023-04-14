// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
)

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
func (ps *EvaluationSet) GetPolicies() []*Policy {
	var policiesList []*Policy
	for _, rs := range ps.RuleSets {
		policiesList = append(policiesList, rs.policies...)
	}
	return policiesList
}

func (es *EvaluationSet) LoadPolicies(loader *PolicyLoader, opts PolicyLoaderOpts) *multierror.Error {
	var (
		errs                     *multierror.Error
		probeEvaluationRules     []*RuleDefinition
		ruleSetTaggedRules       = make(map[eval.RuleSetTagValue][]*RuleDefinition)
		allMacros                []*MacroDefinition
		macroIndex               = make(map[string]*MacroDefinition)
		probeEvaluationRuleIndex = make(map[eval.RuleID]*RuleDefinition)
		ruleSetTaggedIndex       = make(map[eval.RuleSetTagValue]map[eval.RuleID]*RuleDefinition)
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
			if existingRule := probeEvaluationRuleIndex[rule.ID]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				probeEvaluationRuleIndex[rule.ID] = rule
				probeEvaluationRules = append(probeEvaluationRules, rule)
			}
		}

		for tagValue, rules := range policy.RuleSetTaggedRules {
			if _, ok := ruleSetTaggedIndex[tagValue]; !ok {
				ruleSetTaggedIndex[tagValue] = make(map[string]*RuleDefinition)
			}
			for _, rule := range rules {
				if existingRule := ruleSetTaggedIndex[tagValue][rule.ID]; existingRule != nil {
					if err := existingRule.MergeWith(rule); err != nil {
						errs = multierror.Append(errs, err)
					}
				} else {
					ruleSetTaggedIndex[tagValue][rule.ID] = rule
					ruleSetTaggedRules[tagValue] = append(ruleSetTaggedRules[tagValue], rule)
				}
			}
		}
	}

	allRuleLists := make(map[eval.RuleSetTagValue][]*RuleDefinition)
	allRuleLists[DefaultRuleSetTagValue] = probeEvaluationRules
	for tags, ruleList := range ruleSetTaggedRules {
		allRuleLists[tags] = ruleList
	}

	for ruleSetTagValue, rs := range es.RuleSets {
		for ruleListName, ruleList := range allRuleLists {
			if ruleListName == ruleSetTagValue {
				// Add the macros to the ruleset and generate macros evaluators
				if err := rs.AddMacros(parsingContext, allMacros); err.ErrorOrNil() != nil {
					errs = multierror.Append(errs, err)
				}

				if err := rs.validatePolicyRules(ruleList); err.ErrorOrNil() != nil {
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
