// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"

	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
)

// PolicyMacro represents a macro loaded from a policy
type PolicyMacro struct {
	Def      *MacroDefinition
	Accepted bool
	Error    error
	Policy   *Policy
}

func (m *PolicyMacro) isAccepted() bool {
	return m.Accepted && m.Error == nil
}

// MergeWith merges macro m2 into m
func (m *PolicyMacro) MergeWith(m2 *PolicyMacro) error {
	switch m2.Def.Combine {
	case MergePolicy:
		if m.Def.Expression != "" || m2.Def.Expression != "" {
			return &ErrMacroLoad{Macro: m2, Err: ErrCannotMergeExpression}
		}
		m.Def.Values = append(m.Def.Values, m2.Def.Values...)
	case OverridePolicy:
		m.Def.Values = m2.Def.Values
	}
	return nil
}

// PolicyRule represents a rule loaded from a policy
type PolicyRule struct {
	Def      *RuleDefinition
	Actions  []*Action
	Accepted bool
	Error    error
	// FilterType is used to keep track of the type of filter that caused the rule to be filtered out
	FilterType FilterType
	Policy     PolicyInfo
	ModifiedBy []PolicyInfo
	UsedBy     []PolicyInfo
}

// Policies returns an iterator over the policies that this rule is part of.
func (r *PolicyRule) Policies(includeInternalPolicies bool) iter.Seq[*PolicyInfo] {
	return func(yield func(*PolicyInfo) bool) {
		if !r.Policy.IsInternal || includeInternalPolicies {
			if !yield(&r.Policy) {
				return
			}
		}
		for _, policy := range r.UsedBy {
			if !policy.IsInternal || includeInternalPolicies {
				if !yield(&policy) {
					return
				}
			}
		}
	}
}

func (r *PolicyRule) isAccepted() bool {
	return r.Accepted && r.Error == nil
}

func (r *PolicyRule) isFiltered() bool {
	return !r.Accepted && r.Error == nil
}

func applyOverride(rd1, rd2 *PolicyRule) {
	// keep track of the combine
	rd1.Def.Combine = rd2.Def.Combine

	wasOverridden := false
	// for backward compatibility, by default only the expression is copied if no options
	if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideAllFields) && rd1.Policy.Type == DefaultPolicyType {
		tmpExpression := rd1.Def.Expression
		*rd1.Def = *rd2.Def
		rd1.Def.Expression = tmpExpression
		wasOverridden = true
	} else {
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideActionFields) {
			var toAdd []*ActionDefinition
			for _, action := range rd2.Def.Actions {
				duplicated := false
				for _, a := range rd1.Def.Actions {
					if reflect.DeepEqual(action, a) {
						duplicated = true
						break
					}
				}
				if !duplicated {
					toAdd = append(toAdd, action)
				}
			}

			if len(toAdd) > 0 {
				wasOverridden = true
				rd1.Def.Actions = append(rd1.Def.Actions, toAdd...)
			}
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideEveryField) {
			rd1.Def.Every = rd2.Def.Every
			wasOverridden = true
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideTagsField) {
			for k, tag := range rd2.Def.Tags {
				rd1.Def.Tags[k] = tag
				wasOverridden = true
			}
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideProductTagsField) {
			rd1.Def.ProductTags = rd2.Def.ProductTags
		}
	}

	if wasOverridden {
		rd1.Policy = rd2.Policy
	}
}

// MergeWith merges rule r2 into r
func (r *PolicyRule) MergeWith(r2 *PolicyRule) {
	switch r2.Def.Combine {
	case OverridePolicy:
		if !r2.Def.Disabled {
			applyOverride(r, r2)
		}
	default:
		if r.Def.Disabled == r2.Def.Disabled {
			return
		}
	}

	if r.Def.Disabled {
		r.Def.Disabled = r2.Def.Disabled
		r.Policy = r2.Policy
	} else {
		if r.Policy.Type == DefaultPolicyType && r2.Policy.Type == CustomPolicyType {
			r.Def.Disabled = r2.Def.Disabled
			r.Policy = r2.Policy
		}
	}

	r.ModifiedBy = append(r.ModifiedBy, r2.Policy)
}

// PolicyType represents the type of a policy
type PolicyType string

const (
	// DefaultPolicyType is the default policy type
	DefaultPolicyType PolicyType = "default"
	// CustomPolicyType is the custom policy type
	CustomPolicyType PolicyType = "custom"
	// InternalPolicyType is the policy for internal use (bundled_policy_provider)
	InternalPolicyType PolicyType = "internal"
	// SelftestPolicy is the policy for self tests
	SelftestPolicy PolicyType = "selftest"
)

// PolicyInfo contains information about a policy that aren't part of the policy definition
type PolicyInfo struct {
	// Name is the name of the policy
	Name string
	// Source is the source of the policy
	Source string
	// Type is the type of the policy
	Type PolicyType
	// Version is the version of the policy, this field is copied from the policy definition
	Version string
	// IsInternal is true if the policy is internal
	IsInternal bool
}

// Equals compares two PolicyInfo objects and returns true if they are equal
func (pi *PolicyInfo) Equals(other *PolicyInfo) bool {
	return reflect.DeepEqual(pi, other)
}

// Policy represents a policy which is composed of a list of rules, macros and on-demand hook points
type Policy struct {
	// Def is the policy definition
	Def *PolicyDef
	// Info contains the policy information such as its name, source and type
	Info PolicyInfo
	// multiple macros can have the same ID but different filters (e.g. agent version)
	macros map[MacroID][]*PolicyMacro
	// multiple rules can have the same ID but different filters (e.g. agent version)
	rules map[RuleID][]*PolicyRule
}

// GetAcceptedMacros returns the list of accepted macros that are part of the policy
func (p *Policy) GetAcceptedMacros() []*PolicyMacro {
	var acceptedMacros []*PolicyMacro
	for _, macros := range p.macros {
		for _, macro := range macros {
			if macro.isAccepted() {
				acceptedMacros = append(acceptedMacros, macro)
			}
		}
	}
	return acceptedMacros
}

// GetAcceptedRules returns the list of accepted rules that are part of the policy
func (p *Policy) GetAcceptedRules() []*PolicyRule {
	var acceptedRules []*PolicyRule
	for _, rules := range p.rules {
		for _, rule := range rules {
			if rule.isAccepted() {
				acceptedRules = append(acceptedRules, rule)
			}
		}
	}
	return acceptedRules
}

// GetFilteredRules returns the list of filtered rules that are part of the policy
func (p *Policy) GetFilteredRules() []*PolicyRule {
	var filteredRules []*PolicyRule
	for _, rules := range p.rules {
		for _, rule := range rules {
			if rule.isFiltered() {
				filteredRules = append(filteredRules, rule)
			}
		}
	}
	return filteredRules
}

// SetInternalCallbackAction adds an internal callback action for the given rule IDs
func (p *Policy) SetInternalCallbackAction(ruleID ...RuleID) {
	for _, id := range ruleID {
		if rules, ok := p.rules[id]; ok {
			for _, rule := range rules {
				if rule.isAccepted() && rule.Def.ID == id {
					rule.Actions = append(rule.Actions, &Action{
						InternalCallback: &InternalCallbackDefinition{},
						Def:              &ActionDefinition{},
					})
				}
			}
		}
	}
}

func (p *Policy) parse(macroFilters []MacroFilter, ruleFilters []RuleFilter) error {
	var errs *multierror.Error

MACROS:
	for _, macroDef := range p.Def.Macros {
		macro := &PolicyMacro{
			Def:      macroDef,
			Accepted: true,
			Policy:   p,
		}
		p.macros[macroDef.ID] = append(p.macros[macroDef.ID], macro)
		for _, filter := range macroFilters {
			macro.Accepted, macro.Error = filter.IsMacroAccepted(macroDef)
			if macro.Error != nil {
				errs = multierror.Append(errs, &ErrMacroLoad{Macro: macro, Err: fmt.Errorf("error when evaluating one of the macro filters: %w", macro.Error)})
			}

			if !macro.Accepted {
				continue MACROS
			}
		}

		if macroDef.ID == "" {
			macro.Error = &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)}
			errs = multierror.Append(errs, macro.Error)
			continue
		}
		if !validators.CheckRuleID(macroDef.ID) {
			macro.Error = &ErrMacroLoad{Macro: macro, Err: fmt.Errorf("ID does not match pattern `%s`", validators.RuleIDPattern)}
			errs = multierror.Append(errs, macro.Error)
			continue
		}
	}

RULES:
	for _, ruleDef := range p.Def.Rules {
		rule := &PolicyRule{
			Def:      ruleDef,
			Accepted: true,
			Policy:   p.Info, // copy the policy information as it can be modified on a per-rule basis when merging rules from different policies
		}
		p.rules[ruleDef.ID] = append(p.rules[ruleDef.ID], rule)
		for _, filter := range ruleFilters {
			rule.Accepted, rule.Error = filter.IsRuleAccepted(ruleDef)
			if rule.Error != nil {
				errs = multierror.Append(errs, &ErrRuleLoad{Rule: rule, Err: rule.Error})
			}

			if !rule.Accepted {
				rule.FilterType = filter.GetType()
				continue RULES
			}
		}

		if rule.Def.ID == "" {
			rule.Error = &ErrRuleLoad{Rule: rule, Err: ErrRuleWithoutID}
			errs = multierror.Append(errs, rule.Error)
			continue
		}
		if !validators.CheckRuleID(ruleDef.ID) {
			rule.Error = &ErrRuleLoad{Rule: rule, Err: ErrRuleIDPattern}
			errs = multierror.Append(errs, rule.Error)
			continue
		}

		if ruleDef.GroupID != "" && !validators.CheckRuleID(ruleDef.GroupID) {
			rule.Error = &ErrRuleLoad{Rule: rule, Err: ErrRuleIDPattern}
			errs = multierror.Append(errs, rule.Error)
			continue
		}

		if ruleDef.Expression == "" && !ruleDef.Disabled && ruleDef.Combine == "" {
			rule.Error = &ErrRuleLoad{Rule: rule, Err: ErrRuleWithoutExpression}
			errs = multierror.Append(errs, rule.Error)
			continue
		}
	}

	return errs.ErrorOrNil()
}

// LoadPolicyFromDefinition load a policy from a definition
func LoadPolicyFromDefinition(info *PolicyInfo, def *PolicyDef, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	if def != nil && def.Version != "" {
		info.Version = def.Version
	}

	p := &Policy{
		Def:    def,
		Info:   *info,
		macros: make(map[MacroID][]*PolicyMacro, len(def.Macros)),
		rules:  make(map[RuleID][]*PolicyRule, len(def.Rules)),
	}

	return p, p.parse(macroFilters, ruleFilters)
}

// LoadPolicy load a policy
func LoadPolicy(info *PolicyInfo, reader io.Reader, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	def := PolicyDef{}
	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: info.Name, Source: info.Source, Err: err}
	}

	return LoadPolicyFromDefinition(info, &def, macroFilters, ruleFilters)
}
