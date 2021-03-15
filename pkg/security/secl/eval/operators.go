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

// ArrayStringEquals evaluates array of strings
func ArrayStringEquals(a *StringEvaluator, b *StringArrayEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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

	var arrayOp func(a string, b []string) bool

	if a.isPattern {
		arrayOp = func(as string, bs []string) bool {
			for _, v := range bs {
				if a.regexp.MatchString(v) {
					return true
				}
			}
			return false
		}
	} else if b.isPattern {
		arrayOp = func(as string, bs []string) bool {
			for _, re := range b.regexps {
				if re.MatchString(as) {
					return true
				}
			}
			return false
		}
	} else {
		arrayOp = func(as string, bs []string) bool {
			for _, v := range bs {
				if as == v {
					return true
				}
			}
			return false
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ea(ctx), eb(ctx))
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
			Value:     arrayOp(ea, eb),
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
			return arrayOp(ea(ctx), eb)
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
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}
