// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"github.com/pkg/errors"
)

// IntNot - ^int operator
func IntNot(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) int {
			return ^ea(ctx)
		}

		return &IntEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight,
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     ^a.Value,
		Weight:    a.Weight,
		isPartial: isPartialLeaf,
	}
}

// StringMatches - String pattern matching operator
func StringMatches(a *StringEvaluator, b *StringEvaluator, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	re, err := patternToRegexp(b.Value)
	if err != nil {
		return nil, err
	}

	if b.EvalFnc != nil {
		return nil, errors.New("regex has to be a scalar string")
	}

	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: PatternValueType, Regex: re}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) bool {
			result := re.MatchString(ea(ctx))
			if not {
				return !result
			}
			return result
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + PatternWeight,
			isPartial: isPartialLeaf,
		}, nil
	}

	ea := true
	if !isPartialLeaf {
		ea = re.MatchString(a.Value)
		if not {
			return &BoolEvaluator{
				Value:     !ea,
				Weight:    a.Weight + PatternWeight,
				isPartial: isPartialLeaf,
			}, nil
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		Weight:    a.Weight + PatternWeight,
		isPartial: isPartialLeaf,
	}, nil
}

// Not - !true operator
func Not(a *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := func(ctx *Context) bool {
			return !a.EvalFnc(ctx)
		}

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
		}

		return &BoolEvaluator{
			EvalFnc:   ea,
			Weight:    a.Weight,
			isPartial: isPartialLeaf,
		}
	}

	value := true
	if !isPartialLeaf {
		value = !a.Value
	}

	return &BoolEvaluator{
		Value:     value,
		Weight:    a.Weight,
		isPartial: isPartialLeaf,
	}
}

// Minus - -int operator
func Minus(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) int {
			return -ea(ctx)
		}

		return &IntEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight,
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     -a.Value,
		Weight:    a.Weight,
		isPartial: isPartialLeaf,
	}
}

func stringArrayContains(s string, a []string) bool {
	for _, v := range a {
		if s == v {
			return true
		}
	}
	return false
}

// StringArrayContains - "test" in ["...", "..."] operator
func StringArrayContains(a *StringEvaluator, b *StringArrayEvaluator, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	partialA, partialB := a.isPartial, b.isPartial

	if a.EvalFnc == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.EvalFnc == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			if not {
				return !stringArrayContains(ea(ctx), eb(ctx))
			}
			return stringArrayContains(ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + b.Weight,
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		return &BoolEvaluator{
			Value:     stringArrayContains(ea, eb) && !not,
			Weight:    a.Weight + InArrayWeight*len(eb),
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		if a.Field != "" {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}

		evalFnc := func(ctx *Context) bool {
			if not {
				return !stringArrayContains(ea(ctx), eb)
			}
			return stringArrayContains(ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InArrayWeight*len(eb),
			isPartial: isPartialLeaf,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	evalFnc := func(ctx *Context) bool {
		if not {
			return !stringArrayContains(ea, eb(ctx))
		}
		return stringArrayContains(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}

// StringArrayMatches - "test" in [~"...", "..."] operator
func StringArrayMatches(a *StringEvaluator, b *PatternArray, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) bool {
			s := ea(ctx)

			var result bool
			for _, reg := range b.Regexps {
				if result = reg.MatchString(s); result {
					break
				}
			}

			if not {
				return !result
			}
			return result
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InPatternArrayWeight*len(b.Values),
			isPartial: isPartialLeaf,
		}, nil
	}

	ea := true
	if !isPartialLeaf {
		for _, reg := range b.Regexps {
			if ea = reg.MatchString(a.Value); ea {
				break
			}
		}

		if not {
			ea = !ea
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		Weight:    a.Weight + InPatternArrayWeight*len(b.Values),
		isPartial: isPartialLeaf,
	}, nil
}
