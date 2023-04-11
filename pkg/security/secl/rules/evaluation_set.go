// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
)

type EvaluationSet struct {
	RuleSets map[string]*RuleSet
}

// NewEvaluationSet returns a new policy set for the specified data model
func NewEvaluationSet(ruleSetsToInclude []*RuleSet) *EvaluationSet {
	ruleSets := make(map[string]*RuleSet)

	for _, ruleSet := range ruleSetsToInclude {
		if ruleSet != nil {
			ruleSets[ruleSet.opts.Tag] = ruleSet
		}
	}

	return &EvaluationSet{RuleSets: ruleSets}
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
		errs                 *multierror.Error
		probeEvaluationRules []*RuleDefinition
		taggedRules          = make(map[eval.NormalizedRuleTag][]*RuleDefinition)
		allMacros            []*MacroDefinition
		macroIndex           = make(map[string]*MacroDefinition)
		ruleIndex            = make(map[eval.RuleID]*RuleDefinition)
		taggedRulesRuleIndex = make(map[eval.NormalizedRuleTag]map[eval.RuleID]*RuleDefinition)
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
			if existingRule := ruleIndex[rule.ID]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				ruleIndex[rule.ID] = rule
				probeEvaluationRules = append(probeEvaluationRules, rule)
			}
		}

		// TODO: if there are no taggedRules, populate default ruleset

		for tag, rules := range policy.TaggedRules {
			if _, ok := taggedRulesRuleIndex[tag]; !ok {
				taggedRulesRuleIndex[tag] = make(map[string]*RuleDefinition)
			}
			for _, rule := range rules {
				if existingRule := taggedRulesRuleIndex[tag][rule.ID]; existingRule != nil {
					if err := existingRule.MergeWith(rule); err != nil {
						errs = multierror.Append(errs, err)
					}
				} else {
					taggedRulesRuleIndex[tag][rule.ID] = rule
					taggedRules[tag] = append(taggedRules[tag], rule)
				}
			}
		}
	}

	allRuleLists := make(map[string][]*RuleDefinition)
	allRuleLists[ProbeEvaluationRuleSetTag] = probeEvaluationRules
	for tags, ruleList := range taggedRules {
		allRuleLists[tags] = ruleList
	}

	for ruleSetTag, rs := range es.RuleSets {
		for ruleListName, ruleList := range allRuleLists {
			if ruleListName == ruleSetTag {
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
