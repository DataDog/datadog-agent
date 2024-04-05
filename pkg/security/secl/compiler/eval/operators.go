// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"net"
	"strings"
)

// OpOverrides defines operator override functions
type OpOverrides struct {
	StringEquals         func(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error)
	StringValuesContains func(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error)
	StringArrayContains  func(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error)
	StringArrayMatches   func(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error)
}

// return whether a arithmetic operation is deterministic
func isArithmDeterministic(a Evaluator, b Evaluator, state *State) bool {
	isDc := a.IsDeterministicFor(state.field) || b.IsDeterministicFor(state.field)

	if aField := a.GetField(); aField != "" && state.field != "" && aField != state.field {
		isDc = false
	}
	if bField := b.GetField(); bField != "" && state.field != "" && bField != state.field {
		isDc = false
	}

	return isDc
}

// Or operator
func Or(a *BoolEvaluator, b *BoolEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := a.IsDeterministicFor(state.field) || b.IsDeterministicFor(state.field)

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		if state.field != "" {
			if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
				eb = func(ctx *Context) bool {
					return true
				}
			}
		}

		if a.Weight > b.Weight {
			tmp := ea
			ea = eb
			eb = tmp
		}

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) || eb(ctx)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           ea || eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		if a.Field != "" {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}

		if state.field != "" {
			if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
				eb = true
			}
		}

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) || eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if state.field != "" {
		if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
			ea = true
		}
		if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
			eb = func(ctx *Context) bool {
				return true
			}
		}
	}

	evalFnc := func(ctx *Context) bool {
		return ea || eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// And operator
func And(a *BoolEvaluator, b *BoolEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := a.IsDeterministicFor(state.field) || b.IsDeterministicFor(state.field)

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		if state.field != "" {
			if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
				eb = func(ctx *Context) bool {
					return true
				}
			}
		}

		if a.Weight > b.Weight {
			tmp := ea
			ea = eb
			eb = tmp
		}

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) && eb(ctx)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           ea && eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		if a.Field != "" {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}

		if state.field != "" {
			if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
				eb = true
			}
		}

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) && eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if state.field != "" {
		if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
			ea = true
		}
		if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
			eb = func(ctx *Context) bool {
				return true
			}
		}
	}

	evalFnc := func(ctx *Context) bool {
		return ea && eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// IntNot ^int operator
func IntNot(a *IntEvaluator, state *State) *IntEvaluator {
	isDc := a.IsDeterministicFor(state.field)

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) int {
			return ^ea(ctx)
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}
	}

	return &IntEvaluator{
		Value:           ^a.Value,
		Weight:          a.Weight,
		isDeterministic: isDc,
	}
}

// StringEquals evaluates string
func StringEquals(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: b.ValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: a.ValueType}); err != nil {
			return nil, err
		}
	}

	// default comparison
	op := func(as string, bs string) bool {
		return as == bs
	}

	if a.Field != "" && b.Field != "" {
		if a.StringCmpOpts.CaseInsensitive || b.StringCmpOpts.CaseInsensitive {
			op = strings.EqualFold
		}
	} else if a.Field != "" {
		matcher, err := b.ToStringMatcher(a.StringCmpOpts)
		if err != nil {
			return nil, err
		}

		if matcher != nil {
			op = func(as string, bs string) bool {
				return matcher.Matches(as)
			}
		}
	} else if b.Field != "" {
		matcher, err := a.ToStringMatcher(b.StringCmpOpts)
		if err != nil {
			return nil, err
		}

		if matcher != nil {
			op = func(as string, bs string) bool {
				return matcher.Matches(bs)
			}
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return op(ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:           op(ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return op(ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return op(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// Not !true operator
func Not(a *BoolEvaluator, state *State) *BoolEvaluator {
	isDc := a.IsDeterministicFor(state.field)

	if a.EvalFnc != nil {
		ea := func(ctx *Context) bool {
			return !a.EvalFnc(ctx)
		}

		if state.field != "" && !a.IsDeterministicFor(state.field) {
			ea = func(ctx *Context) bool {
				return true
			}
		}

		return &BoolEvaluator{
			EvalFnc:         ea,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}
	}

	return &BoolEvaluator{
		Value:           !a.Value,
		Weight:          a.Weight,
		isDeterministic: isDc,
	}
}

// Minus -int operator
func Minus(a *IntEvaluator, state *State) *IntEvaluator {
	isDc := a.IsDeterministicFor(state.field)

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		evalFnc := func(ctx *Context) int {
			return -ea(ctx)
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}
	}

	return &IntEvaluator{
		Value:           -a.Value,
		Weight:          a.Weight,
		isDeterministic: isDc,
	}
}

// StringArrayContains evaluates array of strings against a value
func StringArrayContains(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: a.ValueType}); err != nil {
			return nil, err
		}
	}

	op := func(a string, b []string, cmp func(a, b string) bool) bool {
		for _, bs := range b {
			if cmp(a, bs) {
				return true
			}
		}
		return false
	}

	cmp := func(a, b string) bool {
		return a == b
	}

	if a.Field != "" && b.Field != "" {
		if a.StringCmpOpts.CaseInsensitive || b.StringCmpOpts.CaseInsensitive {
			cmp = strings.EqualFold
		}
	} else if a.Field != "" && a.StringCmpOpts.CaseInsensitive {
		cmp = strings.EqualFold
	} else if b.Field != "" {
		matcher, err := a.ToStringMatcher(b.StringCmpOpts)
		if err != nil {
			return nil, err
		}

		if matcher != nil {
			cmp = func(a, b string) bool {
				return matcher.Matches(b)
			}
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return op(ea(ctx), eb(ctx), cmp)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		return &BoolEvaluator{
			Value:           op(ea, eb, cmp),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return op(ea(ctx), eb, cmp)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return op(ea, eb(ctx), cmp)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// StringValuesContains evaluates a string against values
func StringValuesContains(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values.fieldValues {
			if err := state.UpdateFieldValues(a.Field, value); err != nil {
				return nil, err
			}
		}
	}

	if err := b.Compile(a.StringCmpOpts); err != nil {
		return nil, err
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			values := eb(ctx)
			return values.Matches(ea(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		return &BoolEvaluator{
			Value:           eb.Matches(ea),
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return eb.Matches(ea(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		values := eb(ctx)
		return values.Matches(ea)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// StringArrayMatches weak comparison, a least one element of a should be in b. a can't contain regexp
func StringArrayMatches(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values.fieldValues {
			if err := state.UpdateFieldValues(a.Field, value); err != nil {
				return nil, err
			}
		}
	}

	if err := b.Compile(a.StringCmpOpts); err != nil {
		return nil, err
	}

	arrayOp := func(a []string, b *StringValues) bool {
		for _, as := range a {
			if b.Matches(as) {
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
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Values, b.Values

		return &BoolEvaluator{
			Value:           arrayOp(ea, &eb),
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ea(ctx), &eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Values, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// IntArrayMatches weak comparison, a least one element of a should be in b
func IntArrayMatches(a *IntArrayEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value}); err != nil {
				return nil, err
			}
		}
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
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Values, b.Values

		return &BoolEvaluator{
			Value:           arrayOp(ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Values, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// ArrayBoolContains evaluates array of bool against a value
func ArrayBoolContains(a *BoolEvaluator, b *BoolArrayEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value}); err != nil {
			return nil, err
		}
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
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		return &BoolEvaluator{
			Value:           arrayOp(ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// CIDREquals evaluates CIDR ranges
func CIDREquals(a *CIDREvaluator, b *CIDREvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: b.ValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: a.ValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			a, b := ea(ctx), eb(ctx)
			return IPNetsMatch(&a, &b)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:           IPNetsMatch(&ea, &eb),
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			a := ea(ctx)
			return IPNetsMatch(&a, &eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		b := eb(ctx)
		return IPNetsMatch(&ea, &b)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// CIDRValuesContains evaluates a CIDR against a list of CIDRs
func CIDRValuesContains(a *CIDREvaluator, b *CIDRValuesEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Value.fieldValues {
			if err := state.UpdateFieldValues(a.Field, value); err != nil {
				return nil, err
			}
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			a := ea(ctx)
			return eb(ctx).Contains(&a)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:           eb.Contains(&ea),
			Weight:          a.Weight + InArrayWeight*len(eb.ipnets),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			ipnet := ea(ctx)
			return eb.Contains(&ipnet)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return eb(ctx).Contains(&ea)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func cidrArrayMatches(a *CIDRArrayEvaluator, b *CIDRValuesEvaluator, state *State, arrayOp func(a *CIDRValues, b []net.IPNet) bool) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Value.fieldValues {
			if err := state.UpdateFieldValues(a.Field, value); err != nil {
				return nil, err
			}
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(eb(ctx), ea(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:           arrayOp(&eb, ea),
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return arrayOp(&eb, ea(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb.fieldValues),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(eb(ctx), ea)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

// CIDRArrayMatches weak comparison, at least one element of a should be in b.
func CIDRArrayMatches(a *CIDRArrayEvaluator, b *CIDRValuesEvaluator, state *State) (*BoolEvaluator, error) {
	op := func(values *CIDRValues, ipnets []net.IPNet) bool {
		return values.Match(ipnets)
	}
	return cidrArrayMatches(a, b, state, op)
}

// CIDRArrayMatchesAll ensures that all values from a and b match.
func CIDRArrayMatchesAll(a *CIDRArrayEvaluator, b *CIDRValuesEvaluator, state *State) (*BoolEvaluator, error) {
	op := func(values *CIDRValues, ipnets []net.IPNet) bool {
		return values.MatchAll(ipnets)
	}
	return cidrArrayMatches(a, b, state, op)
}

// CIDRArrayContains evaluates a CIDR against a list of CIDRs
func CIDRArrayContains(a *CIDREvaluator, b *CIDRArrayEvaluator, state *State) (*BoolEvaluator, error) {
	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Value {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Type: IPNetValueType, Value: value}); err != nil {
				return nil, err
			}
		}
	}

	arrayOp := func(a *net.IPNet, b []net.IPNet) bool {
		for _, n := range b {
			if IPNetsMatch(a, &n) {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			a := ea(ctx)
			return arrayOp(&a, eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:           arrayOp(&ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			ipnet := ea(ctx)
			return arrayOp(&ipnet, eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(&ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}
