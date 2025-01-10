// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filter holds filter related files
package filter

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

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

// IsAccepted checks whether the rule is accepted
func (r *SECLRuleFilter) IsAccepted(filters []string) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}

	expression := mergeFilterExpressions(filters)
	astRule, err := r.parsingContext.ParseRule(expression)
	if err != nil {
		return false, err
	}

	evalOpts := &eval.Opts{}
	evalOpts.
		WithConstants(map[string]interface{}{
			"true":  &eval.BoolEvaluator{Value: true},
			"false": &eval.BoolEvaluator{Value: false},
		})

	evaluator, err := eval.NewRuleEvaluator(astRule, r.model, evalOpts)
	if err != nil {
		return false, err
	}

	ctx := r.newEvalContext()
	return evaluator.Eval(&ctx), nil
}
