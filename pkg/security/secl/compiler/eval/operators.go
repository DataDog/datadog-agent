// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"regexp"
)

// OpOverride defines a operator override function suite
type OpOverrides struct {
	StringEquals         func(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) (*BoolEvaluator, error)
	StringValuesContains func(a *StringEvaluator, b *StringValuesEvaluator, opts *Opts, state *State) (*BoolEvaluator, error)
	StringArrayContains  func(a *StringEvaluator, b *StringArrayEvaluator, opts *Opts, state *State) (*BoolEvaluator, error)
	StringArrayMatches   func(a *StringArrayEvaluator, b *StringValuesEvaluator, opts *Opts, state *State) (*BoolEvaluator, error)
}

func isPartialLeaf(a Evaluator, b Evaluator, state *State) bool {
	partialA, partialB := a.IsPartial(), b.IsPartial()

	if a.IsScalar() || (a.GetField() != "" && a.GetField() != state.field) {
		partialA = true
	}
	if b.IsScalar() || (b.GetField() != "" && b.GetField() != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.GetField() != "" && b.GetField() != "" {
		isPartialLeaf = true
	}

	return isPartialLeaf
}

// IntNot - ^int operator
func IntNot(a *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
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

// StringEquals evaluates string
func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
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

	var arrayOp func(a string, b string) bool

	if a.regexp != nil {
		arrayOp = func(as string, bs string) bool {
			return a.regexp.MatchString(bs)
		}
	} else if b.regexp != nil {
		arrayOp = func(as string, bs string) bool {
			return b.regexp.MatchString(as)
		}
	} else {
		arrayOp = func(as string, bs string) bool {
			return as == bs
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
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     arrayOp(ea, eb),
			Weight:    a.Weight + InArrayWeight*len(eb),
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		if a.Field != "" {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: b.ValueType, Regexp: b.regexp}); err != nil {
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
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: a.ValueType, Regexp: a.regexp}); err != nil {
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

// Not - !true operator
func Not(a *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
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

	return &BoolEvaluator{
		Value:     !a.Value,
		Weight:    a.Weight,
		isPartial: isPartialLeaf,
	}
}

// Minus - -int operator
func Minus(a *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
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

// StringArrayContains evaluates array of strings against a value
func StringArrayContains(a *StringEvaluator, b *StringArrayEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
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

	arrayOp := func(a string, b []string) bool {
		for _, bs := range b {
			if a == bs {
				return true
			}
		}

		return false
	}

	reArrayOp := func(re *regexp.Regexp, b []string) bool {
		for _, bs := range b {
			if re.MatchString(bs) {
				return true
			}
		}

		return false
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
		if a.regexp != nil {
			ea, eb := a.regexp, b.Values

			return &BoolEvaluator{
				Value:     reArrayOp(ea, eb),
				Weight:    a.Weight + InArrayWeight*len(eb),
				isPartial: isPartialLeaf,
			}, nil
		}
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
			for _, value := range eb {
				if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
					return nil, err
				}
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
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: a.ValueType, Regexp: a.regexp}); err != nil {
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

// StringArrayContains evaluates array of strings against a value
func StringValuesContains(a *StringEvaluator, b *StringValuesEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
	partialA, partialB := a.isPartial, b.isPartial

	if a.EvalFnc == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.EvalFnc == nil {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			values := eb(ctx)
			return values.Match(ea(ctx))
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
			Value:     eb.Match(ea),
			Weight:    a.Weight + InArrayWeight*len(eb.fieldValues),
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		if a.Field != "" {
			for _, value := range eb.fieldValues {
				if err := state.UpdateFieldValues(a.Field, value); err != nil {
					return nil, err
				}
			}
		}

		evalFnc := func(ctx *Context) bool {
			return eb.Match(ea(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InArrayWeight*len(eb.fieldValues),
			isPartial: isPartialLeaf,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		values := eb(ctx)
		return values.Match(ea)
	}

	return &BoolEvaluator{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}

// StringArrayMatches weak comparison, a least one element of a should be in b. a can't contain regexp
func StringArrayMatches(a *StringArrayEvaluator, b *StringValuesEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
	partialA, partialB := a.isPartial, b.isPartial

	if a.EvalFnc == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.EvalFnc == nil {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" {
		isPartialLeaf = true
	}

	arrayOp := func(a []string, b StringValues) bool {
		for _, as := range a {
			if b.Match(as) {
				return true
			}
		}
		return false
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
		ea, eb := a.Values, b.Values

		return &BoolEvaluator{
			Value:     arrayOp(ea, eb),
			Weight:    a.Weight + InArrayWeight*len(eb.fieldValues),
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		if a.Field != "" {
			for _, value := range eb.fieldValues {
				if err := state.UpdateFieldValues(a.Field, value); err != nil {
					return nil, err
				}
			}
		}

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InArrayWeight*len(eb.fieldValues),
			isPartial: isPartialLeaf,
		}, nil
	}

	ea, eb := a.Values, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}

// IntArrayMatches weak comparison, a least one element of a should be in b
func IntArrayMatches(a *IntArrayEvaluator, b *IntArrayEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
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

	arrayOp := func(a []int, b []int) bool {
		for _, va := range a {
			for _, vb := range b {
				if va == vb {
					return true
				}
			}
		}
		return false
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
		ea, eb := a.Values, b.Values

		return &BoolEvaluator{
			Value:     arrayOp(ea, eb),
			Weight:    a.Weight + InArrayWeight*len(eb),
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		if a.Field != "" {
			for _, value := range b.Values {
				if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value}); err != nil {
					return nil, err
				}
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

	ea, eb := a.Values, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}

// ArrayBoolContains evaluates array of bool against a value
func ArrayBoolContains(a *BoolEvaluator, b *BoolArrayEvaluator, opts *Opts, state *State) (*BoolEvaluator, error) {
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

	arrayOp := func(a bool, b []bool) bool {
		for _, v := range b {
			if v == a {
				return true
			}
		}
		return false
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
			for _, value := range eb {
				if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value}); err != nil {
					return nil, err
				}
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
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea}); err != nil {
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
