// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func toLowerStringEvaluator(evaluator *eval.StringEvaluator) (*eval.StringEvaluator, error) {
	if evaluator.IsScalar() {
		evaluator.Value = strings.ToLower(evaluator.Value)
		switch evaluator.ValueType {
		case eval.PatternValueType, eval.RegexpValueType:
			evaluator.Value = strings.ToLower(evaluator.Value)
			if err := evaluator.Compile(); err != nil {
				return nil, err
			}
		}

		return evaluator, nil
	}

	return &eval.StringEvaluator{
		EvalFnc: func(ctx *eval.Context) string {
			return strings.ToLower(evaluator.EvalFnc(ctx))
		},
	}, nil
}

func toLowerStringArrayEvaluator(evaluator *eval.StringArrayEvaluator) *eval.StringArrayEvaluator {
	if evaluator.IsScalar() {
		var values []string
		for _, value := range evaluator.Values {
			values = append(values, strings.ToLower(value))
		}
		evaluator.Values = values

		return evaluator
	}
	return &eval.StringArrayEvaluator{
		EvalFnc: func(ctx *eval.Context) []string {
			values := evaluator.EvalFnc(ctx)
			for _, value := range evaluator.Values {
				values = append(values, strings.ToLower(value))
			}
			return values
		},
	}
}

func toLowerStringValues(values *eval.StringValues) (*eval.StringValues, error) {
	var lowerValues eval.StringValues

	for _, value := range values.GetFieldValues() {
		if str, ok := value.Value.(string); ok {
			value.Value = strings.ToLower(str)
		}
		if err := lowerValues.AppendFieldValue(value); err != nil {
			// return the original values in case of error to avoid stack trace in case of being called from EvalFnc
			return values, err
		}
	}
	return &lowerValues, nil
}

func toLowerStringValuesEvaluator(evaluator *eval.StringValuesEvaluator) (*eval.StringValuesEvaluator, error) {
	if evaluator.IsScalar() {
		values, err := toLowerStringValues(&evaluator.Values)
		if err != nil {
			return nil, err
		}
		evaluator.Values = *values

		return evaluator, nil
	}
	return &eval.StringValuesEvaluator{
		EvalFnc: func(ctx *eval.Context) *eval.StringValues {
			evaluator, _ := toLowerStringValues(evaluator.EvalFnc(ctx))
			return evaluator
		},
	}, nil
}

var (
	// LowerCaseCmp lower case values before comparing. Important : this operator override doesn't support approvers
	LowerCaseCmp = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			var err error

			if a, err = toLowerStringEvaluator(a); err != nil {
				return nil, err
			}

			if b, err = toLowerStringEvaluator(b); err != nil {
				return nil, err
			}

			return eval.StringEquals(a, b, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			var err error

			if a, err = toLowerStringEvaluator(a); err != nil {
				return nil, err
			}

			if b, err = toLowerStringValuesEvaluator(b); err != nil {
				return nil, err
			}

			return eval.StringValuesContains(a, b, opts, state)
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			var err error

			if a, err = toLowerStringEvaluator(a); err != nil {
				return nil, err
			}

			b = toLowerStringArrayEvaluator(b)

			return eval.StringArrayContains(a, b, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			a = toLowerStringArrayEvaluator(a)

			var err error
			if b, err = toLowerStringValuesEvaluator(b); err != nil {
				return nil, err
			}

			return eval.StringArrayMatches(a, b, opts, state)
		},
	}
)
