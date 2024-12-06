// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
)

// RuleFilter definition of a rule filter
type RuleFilter interface {
	IsRuleAccepted(*RuleDefinition) (bool, error)
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
	model          eval.Model
	parsingContext *ast.ParsingContext
}

// NewSECLRuleFilter returns a new agent version based rule filter
func NewSECLRuleFilter(model eval.Model) *SECLRuleFilter {
	return &SECLRuleFilter{
		model:          model,
		parsingContext: ast.NewParsingContext(false),
	}
}

func mergeFilterExpressions(filters []string) (expression string) {
	for i, filter := range filters {
		if i != 0 {
			expression += " || "
		}
		expression += "(" + filter + ")"
	}
	return
}

func (r *SECLRuleFilter) newEvalContext() eval.Context {
	return eval.Context{
		Event: r.model.NewEvent(),
	}
}

// IsRuleAccepted checks whether the rule is accepted
func (r *SECLRuleFilter) IsRuleAccepted(rule *RuleDefinition) (bool, error) {
	if len(rule.Filters) == 0 {
		return true, nil
	}

	expression := mergeFilterExpressions(rule.Filters)
	astRule, err := r.parsingContext.ParseRule(expression)
	if err != nil {
		return false, err
	}

	evalOpts := &eval.Opts{}
	evalOpts.
		WithConstants(model.BooleanConstants)

	evaluator, err := eval.NewRuleEvaluator(astRule, r.model, evalOpts)
	if err != nil {
		return false, err
	}

	ctx := r.newEvalContext()
	return evaluator.Eval(&ctx), nil
}

// IsMacroAccepted checks whether the macro is accepted
func (r *SECLRuleFilter) IsMacroAccepted(macro *MacroDefinition) (bool, error) {
	if len(macro.Filters) == 0 {
		return true, nil
	}

	expression := mergeFilterExpressions(macro.Filters)
	astRule, err := r.parsingContext.ParseRule(expression)
	if err != nil {
		return false, err
	}

	evaluator, err := eval.NewRuleEvaluator(astRule, r.model, &eval.Opts{})
	if err != nil {
		return false, err
	}

	ctx := r.newEvalContext()
	return evaluator.Eval(&ctx), nil
}
