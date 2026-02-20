// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filter holds filter related files
package filter

import (
	"runtime"
	"strings"

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
		parsingContext: ast.NewParsingContext(true),
	}
}

func mergeFilterExpressions(filters []string) string {
	var builder strings.Builder
	for i, filter := range filters {
		if i != 0 {
			builder.WriteString(" || ")
		}
		builder.WriteString("(")
		builder.WriteString(filter)
		builder.WriteString(")")
	}
	return builder.String()
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

	// early check for obvious and most used cases
	if len(filters) == 1 {
		switch filters[0] {
		case `os == "linux"`:
			return runtime.GOOS == "linux", nil
		case `os == "windows"`:
			return runtime.GOOS == "windows", nil
		}
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
