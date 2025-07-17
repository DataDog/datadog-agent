// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Action represents the action to take when a rule is triggered
// It can either come from policy a definition or be an internal callback
type Action struct {
	Def                 *ActionDefinition
	InternalCallback    *InternalCallbackDefinition
	FilterEvaluator     *eval.RuleEvaluator
	ScopeFieldEvaluator eval.Evaluator
}

// CompileFilter compiles the filter expression
func (a *Action) CompileFilter(parsingContext *ast.ParsingContext, model eval.Model, evalOpts *eval.Opts) error {
	if a.Def.Filter == nil || *a.Def.Filter == "" {
		return nil
	}

	expression := *a.Def.Filter

	eval, err := eval.NewRule("action_rule", expression, parsingContext, evalOpts)
	if err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	if err := eval.GenEvaluator(model); err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	a.FilterEvaluator = eval.GetEvaluator()

	return nil
}

// CompileScopeField compiles the scope field
func (a *Action) CompileScopeField(model eval.Model) error {
	if a.Def.Set == nil || len(a.Def.Set.ScopeField) == 0 {
		return nil
	}

	evaluator, err := model.GetEvaluator(a.Def.Set.ScopeField, "", 0)
	if err != nil {
		return &ErrScopeField{Expression: a.Def.Set.ScopeField, Err: err}
	}

	a.ScopeFieldEvaluator = evaluator
	return nil
}

// IsAccepted returns whether a filter is accepted and has to be executed
func (a *Action) IsAccepted(ctx *eval.Context) bool {
	return a.FilterEvaluator == nil || a.FilterEvaluator.Eval(ctx)
}

// InternalCallbackDefinition describes an internal rule action
type InternalCallbackDefinition struct{}
