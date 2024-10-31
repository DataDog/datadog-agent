// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"io"
	"slices"

	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"

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
	default:
		return &ErrMacroLoad{Macro: m2, Err: ErrDefinitionIDConflict}
	}
	return nil
}

// PolicyRule represents a rule loaded from a policy
type PolicyRule struct {
	Def        *RuleDefinition
	Actions    []*Action
	Accepted   bool
	Error      error
	Policy     *Policy
	ModifiedBy []*PolicyRule
}

func (r *PolicyRule) isAccepted() bool {
	return r.Accepted && r.Error == nil
}

func applyOverride(rd1, rd2 *PolicyRule) {
	// keep track of the combine
	rd1.Def.Combine = rd2.Def.Combine

	// for backward compatibility, by default only the expression is copied if no options
	if len(rd2.Def.OverrideOptions.Fields) == 0 {
		rd1.Def.Expression = rd2.Def.Expression
	} else if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideAllFields) {
		*rd1.Def = *rd2.Def
	} else {
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideExpressionField) {
			rd1.Def.Expression = rd2.Def.Expression
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideActionFields) {
			rd1.Def.Actions = rd2.Def.Actions
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideEveryField) {
			rd1.Def.Every = rd2.Def.Every
		}
		if slices.Contains(rd2.Def.OverrideOptions.Fields, OverrideTagsField) {
			rd1.Def.Tags = rd2.Def.Tags
		}
	}
}

// MergeWith merges rule r2 into r
func (r *PolicyRule) MergeWith(r2 *PolicyRule) error {
	switch r2.Def.Combine {
	case OverridePolicy:
		applyOverride(r, r2)
	default:
		if r.Def.Disabled == r2.Def.Disabled {
			return &ErrRuleLoad{Rule: r2, Err: ErrDefinitionIDConflict}
		}
	}
	r.Def.Disabled = r2.Def.Disabled
	r.ModifiedBy = append(r.ModifiedBy, r2)
	return nil
}

// Policy represents a policy which is composed of a list of rules, macros and on-demand hook points
type Policy struct {
	Def        *PolicyDef
	Name       string
	Source     string
	IsInternal bool
	// multiple macros can have the same ID but different filters (e.g. agent version)
	macros map[MacroID][]*PolicyMacro
	// multiple rules can have the same ID but different filters (e.g. agent version)
	rules              map[RuleID][]*PolicyRule
	onDemandHookPoints []OnDemandHookPoint
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
			Policy:   p,
		}
		p.rules[ruleDef.ID] = append(p.rules[ruleDef.ID], rule)
		for _, filter := range ruleFilters {
			rule.Accepted, rule.Error = filter.IsRuleAccepted(ruleDef)
			if rule.Error != nil {
				errs = multierror.Append(errs, &ErrRuleLoad{Rule: rule, Err: rule.Error})
			}

			if !rule.Accepted {
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

	p.onDemandHookPoints = p.Def.OnDemandHookPoints

	return errs.ErrorOrNil()
}

// LoadPolicyFromDefinition load a policy from a definition
func LoadPolicyFromDefinition(name string, source string, def *PolicyDef, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	p := &Policy{
		Def:    def,
		Name:   name,
		Source: source,
		macros: make(map[MacroID][]*PolicyMacro, len(def.Macros)),
		rules:  make(map[RuleID][]*PolicyRule, len(def.Rules)),
	}

	return p, p.parse(macroFilters, ruleFilters)
}

// LoadPolicy load a policy
func LoadPolicy(name string, source string, reader io.Reader, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	def := PolicyDef{}
	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	return LoadPolicyFromDefinition(name, source, &def, macroFilters, ruleFilters)
}
