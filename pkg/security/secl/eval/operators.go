// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"regexp"

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

// StringEquals evaluates string
func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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

	if a.isRegexp {
		arrayOp = func(as string, bs string) bool {
			if a.regexp.MatchString(bs) {
				return true
			}
			return false
		}
	} else if b.isRegexp {
		arrayOp = func(as string, bs string) bool {
			if b.regexp.MatchString(as) {
				return true
			}
			return false
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
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: b.valueType, Regex: b.regexp}); err != nil {
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
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: a.valueType, Regex: a.regexp}); err != nil {
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

	return &BoolEvaluator{
		Value:     !a.Value,
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

// ArrayStringContains evaluates array of strings against a value
func ArrayStringContains(a *StringEvaluator, b *StringArrayEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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

	if a.isRegexp {
		arrayOp = func(as string, bs []string) bool {
			for _, v := range bs {
				if a.regexp.MatchString(v) {
					return true
				}
			}
			return false
		}
	} else if b.isRegexp {
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
			var regexp *regexp.Regexp
			for i, value := range b.Values {
				if b.isRegexp {
					regexp = b.regexps[i]
				}

				if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: b.valueTypes[i], Regex: regexp}); err != nil {
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
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: a.valueType, Regex: a.regexp}); err != nil {
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

// ArrayStringMatches weak comparison, a least one element of a should be in b. a can't contain regexp
func ArrayStringMatches(a *StringArrayEvaluator, b *StringArrayEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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

	if a.isRegexp {
		return nil, errors.New("pattern not supported on left list")
	}

	var arrayOp func(a []string, b []string) bool

	if b.isRegexp {
		arrayOp = func(as []string, bs []string) bool {
			for _, va := range as {
				for _, re := range b.regexps {
					if re.MatchString(va) {
						return true
					}
				}
			}
			return false
		}
	} else {
		arrayOp = func(as []string, bs []string) bool {
			for _, va := range as {
				for _, vb := range bs {
					if va == vb {
						return true
					}
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
			var regexp *regexp.Regexp
			for i, value := range b.Values {
				if b.isRegexp {
					regexp = b.regexps[i]
				}

				if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: b.valueTypes[i], Regex: regexp}); err != nil {
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

	if b.Field != "" {
		var regexp *regexp.Regexp
		for i, value := range a.Values {
			if a.isRegexp {
				regexp = a.regexps[i]
			}

			if err := state.UpdateFieldValues(b.Field, FieldValue{Value: value, Type: a.valueTypes[i], Regex: regexp}); err != nil {
				return nil, err
			}
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

// ArrayIntMatches weak comparison, a least one element of a should be in b
func ArrayIntMatches(a *IntArrayEvaluator, b *IntArrayEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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

	ea, eb := a.Values, b.EvalFnc

	if b.Field != "" {
		for _, value := range a.Values {
			if err := state.UpdateFieldValues(b.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
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

// ArrayBoolContains evaluates array of bool against a value
func ArrayBoolContains(a *BoolEvaluator, b *BoolArrayEvaluator, opts *Opts, state *state) (*BoolEvaluator, error) {
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
