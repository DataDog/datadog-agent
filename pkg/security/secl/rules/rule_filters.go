// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules/filter"
	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
)

// FilterType defines the type of a rule filter
type FilterType string

const (
	// FilterTypeRuleID defines a rule ID based filter
	FilterTypeRuleID FilterType = "rule_id"
	// FilterTypeAgentVersion defines a agent version based filter
	FilterTypeAgentVersion FilterType = "agent_version"
	// FilterTypeRuleFilter defines a SECL rule based filter
	FilterTypeRuleFilter FilterType = "rule_filter"
)

// RuleFilter definition of a rule filter
type RuleFilter interface {
	IsRuleAccepted(*RuleDefinition) (bool, error)
	GetType() FilterType
}

// MacroFilter definition of a macro filter
type MacroFilter interface {
	IsMacroAccepted(*MacroDefinition) (bool, error)
}

// RuleIDFilter defines a ID based filter
type RuleIDFilter struct {
	ID string
}

// IsRuleAccepted checks whether the rule is accepted
func (r *RuleIDFilter) IsRuleAccepted(rule *RuleDefinition) (bool, error) {
	return r.ID == rule.ID, nil
}

// GetType returns the type of this rule filter
func (r *RuleIDFilter) GetType() FilterType {
	return FilterTypeRuleID
}

// AgentVersionFilter defines a agent version filter
type AgentVersionFilter struct {
	version *semver.Version
}

// NewAgentVersionFilter returns a new agent version based rule filter
func NewAgentVersionFilter(version *semver.Version) (*AgentVersionFilter, error) {
	withoutPreAgentVersion, err := version.SetPrerelease("")
	if err != nil {
		return nil, err
	}

	cleanAgentVersion, err := withoutPreAgentVersion.SetMetadata("")
	if err != nil {
		return nil, err
	}

	return &AgentVersionFilter{
		version: &cleanAgentVersion,
	}, nil
}

// IsRuleAccepted checks whether the rule is accepted
func (r *AgentVersionFilter) IsRuleAccepted(rule *RuleDefinition) (bool, error) {
	constraint, err := validators.ValidateAgentVersionConstraint(rule.AgentVersionConstraint)
	if err != nil {
		return false, fmt.Errorf("failed to parse agent version constraint: %v", err)
	}

	return constraint.Check(r.version), nil
}

// GetType returns the type of this rule filter
func (r *AgentVersionFilter) GetType() FilterType {
	return FilterTypeAgentVersion
}

// IsMacroAccepted checks whether the macro is accepted
func (r *AgentVersionFilter) IsMacroAccepted(macro *MacroDefinition) (bool, error) {
	constraint, err := validators.ValidateAgentVersionConstraint(macro.AgentVersionConstraint)
	if err != nil {
		return false, fmt.Errorf("failed to parse agent version constraint: %v", err)
	}

	return constraint.Check(r.version), nil
}

// SECLRuleFilter defines a SECL rule filter
type SECLRuleFilter struct {
	inner *filter.SECLRuleFilter
}

// NewSECLRuleFilter returns a new agent version based rule filter
func NewSECLRuleFilter(model eval.Model) *SECLRuleFilter {
	return &SECLRuleFilter{
		inner: filter.NewSECLRuleFilter(model),
	}
}

// IsRuleAccepted checks whether the rule is accepted
func (r *SECLRuleFilter) IsRuleAccepted(rule *RuleDefinition) (bool, error) {
	return r.inner.IsAccepted(rule.Filters)
}

// GetType returns the type of this rule filter
func (r *SECLRuleFilter) GetType() FilterType {
	return FilterTypeRuleFilter
}

// IsMacroAccepted checks whether the macro is accepted
func (r *SECLRuleFilter) IsMacroAccepted(macro *MacroDefinition) (bool, error) {
	return r.inner.IsAccepted(macro.Filters)
}
