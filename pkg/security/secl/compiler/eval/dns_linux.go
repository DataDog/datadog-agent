// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import "github.com/alecthomas/participle/lexer"

func evaluatorFromRootDomainHandler(fieldname string, pos lexer.Position, state *State) (interface{}, lexer.Position, error) {
	evaluator, err := state.model.GetEvaluator(fieldname, "", 0)
	if err != nil {
		return nil, pos, NewError(pos, "field '%s' doesn't exist", fieldname)
	}

	// Return length evaluator based on field type
	switch fieldEval := evaluator.(type) {
	case *StringArrayEvaluator:
		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				v := fieldEval.Eval(ctx)
				return GetPublicTLDs(v.([]string))
			},
		}, pos, nil
	case *StringEvaluator:
		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				v := fieldEval.Eval(ctx)
				return GetPublicTLD(v.(string))
			},
		}, pos, nil
	default:
		return nil, pos, NewError(pos, "'length' cannot be used on field '%s'", fieldname)
	}
}
