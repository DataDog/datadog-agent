// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import "fmt"

// OpOverrides describes how the string operators of a field differ from the
// default comparison of its value. The comparison of the field value itself is
// always compiled exactly once: an override either changes how every
// comparison of the field compares (StringCmpOpts, GlobPatterns), adds other
// values of the field compared the same way (ExtraInputs), restricts the
// operators of the field (EqualsOnly), or replaces its comparison with a static
// value altogether (StaticEquals).
//
// A field can have several overrides: their comparison options merge, all
// applying together, and their extra inputs add up.
type OpOverrides struct {
	// StringCmpOpts are comparison options applied to every comparison of the
	// field, the one of its value and the ones of its extra inputs. Options set
	// by any of the overrides of a field apply.
	StringCmpOpts StringCmpOpts

	// GlobPatterns makes the patterns compared with the field globs, a `*` not
	// matching a `/`, for every comparison of the field.
	GlobPatterns bool

	// ExtraInputs returns the other values of the field that are compared the
	// same way as its value, the field matching when any of them matches. It
	// returns nil when the field has no other value. The returned evaluators
	// are fresh: the comparison options of the field are set on them.
	ExtraInputs func(field Field) []*StringEvaluator

	// EqualsOnly restricts the field to `==` and `=~`, every other comparison
	// of it being an error.
	EqualsOnly bool

	// StaticEquals, when set, compiles `==` and `=~` of the field with a static
	// value, used instead of the default comparison and of the options and
	// extra inputs of the other overrides. Comparing the field with anything
	// but a static value is an error.
	StaticEquals func(field Field, value string, state *State) (*BoolEvaluator, error)
}

// checkEqualsOnly returns an error when one of the overrides restricts the
// field to `==`
func checkEqualsOnly(opOverrides []*OpOverrides, field Field) error {
	for _, opOverride := range opOverrides {
		if opOverride.EqualsOnly {
			return fmt.Errorf("field `%s` only supports `==`", field)
		}
	}
	return nil
}

// staticEquals compiles `a == b` with the StaticEquals of an override, one of
// the operands being the field and the other its static value
func staticEquals(opOverride *OpOverrides, a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
	if a.IsStatic() {
		return opOverride.StaticEquals(b.Field, a.Value, state)
	} else if b.IsStatic() {
		return opOverride.StaticEquals(a.Field, b.Value, state)
	}
	return nil, fmt.Errorf("field `%s` only supports matching a single static value", fieldOf(a.Field, b.Field))
}

// fieldOf returns the field of a comparison: the left one, else the right one
func fieldOf(a, b Field) Field {
	if a != "" {
		return a
	}
	return b
}

// fieldOpOverrides returns the overrides of a comparison: the ones of its left
// operand, else the ones of its right operand
func fieldOpOverrides(a, b []*OpOverrides) []*OpOverrides {
	if len(a) > 0 {
		return a
	}
	return b
}

// merge returns the options with the ones of other set as well
func (o StringCmpOpts) merge(other StringCmpOpts) StringCmpOpts {
	if other.CaseInsensitive {
		o.CaseInsensitive = true
	}
	if other.PathSeparatorNormalize {
		o.PathSeparatorNormalize = true
	}
	if other.Sanitize != nil {
		o.Sanitize = other.Sanitize
	}
	return o
}

// overridesCmpOpts returns the comparison options of all the overrides of a
// field merged
func overridesCmpOpts(opOverrides []*OpOverrides) StringCmpOpts {
	var opts StringCmpOpts
	for _, opOverride := range opOverrides {
		opts = opts.merge(opOverride.StringCmpOpts)
	}
	return opts
}

// globPatterns returns whether the patterns compared with a field are globs
func globPatterns(opOverrides []*OpOverrides) bool {
	for _, opOverride := range opOverrides {
		if opOverride.GlobPatterns {
			return true
		}
	}
	return false
}

// fieldExtraInputs returns the extra inputs an override adds to a field, the
// comparison options of the field set on them
func fieldExtraInputs(opOverride *OpOverrides, field Field, opts StringCmpOpts) []*StringEvaluator {
	if field == "" || opOverride.ExtraInputs == nil {
		return nil
	}

	inputs := opOverride.ExtraInputs(field)
	for _, input := range inputs {
		input.StringCmpOpts = input.StringCmpOpts.merge(opts)
	}
	return inputs
}

// globValues returns the values with their patterns turned into globs
func globValues(b *StringValuesEvaluator) *StringValuesEvaluator {
	var values StringValues
	for _, v := range b.Values.GetFieldValues() {
		if v.Type == PatternValueType {
			v.Type = GlobValueType
		}
		values.AppendFieldValue(v)
	}
	return &StringValuesEvaluator{
		Values: values,
	}
}

// orInputs returns the evaluator true when one of the comparisons is, the
// comparisons evaluated in order. The comparisons are the ones of a field and of
// its extra inputs, they report their own matching values.
func orInputs(evaluators []*BoolEvaluator, state *State) (*BoolEvaluator, error) {
	if len(evaluators) == 1 {
		return evaluators[0], nil
	}

	var weight int
	evalFncs := make([]BoolEvalFnc, 0, len(evaluators))
	for _, evaluator := range evaluators {
		if evaluator.EvalFnc == nil || evaluator.Field != "" {
			// a static comparison or a comparison carrying a field, which the
			// generic `||` handles
			return orFold(evaluators, state)
		}
		evalFncs = append(evalFncs, evaluator.EvalFnc)
		weight += evaluator.Weight
	}

	if len(evalFncs) == 2 {
		ea, eb := evalFncs[0], evalFncs[1]
		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) || eb(ctx)
			},
			Weight: weight,
		}, nil
	}

	return &BoolEvaluator{
		EvalFnc: func(ctx *Context) bool {
			for _, evalFnc := range evalFncs {
				if evalFnc(ctx) {
					return true
				}
			}
			return false
		},
		Weight: weight,
	}, nil
}

// orFold folds the comparisons with the `||` operator
func orFold(evaluators []*BoolEvaluator, state *State) (*BoolEvaluator, error) {
	evaluator := evaluators[0]
	for _, next := range evaluators[1:] {
		or, err := Or(evaluator, next, state)
		if err != nil {
			return nil, err
		}
		evaluator = or
	}
	return evaluator, nil
}

// compareInputs compiles the comparison of the value of a field, then the ones
// of its extra inputs, the field matching when any of them matches
func compareInputs(a *StringEvaluator, opOverrides []*OpOverrides, opts StringCmpOpts, cmp func(input *StringEvaluator) (*BoolEvaluator, error), state *State) (*BoolEvaluator, error) {
	inputs := []*StringEvaluator{a}
	for _, opOverride := range opOverrides {
		inputs = append(inputs, fieldExtraInputs(opOverride, a.Field, opts)...)
	}

	evaluators := make([]*BoolEvaluator, 0, len(inputs))
	for _, input := range inputs {
		evaluator, err := cmp(input)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return orInputs(evaluators, state)
}

// StringEqualsWrapper compiles `a == b` with the operator overrides of a, else
// of b
func StringEqualsWrapper(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
	opOverrides := fieldOpOverrides(a.OpOverrides, b.OpOverrides)
	if len(opOverrides) == 0 {
		return StringEquals(a, b, state)
	}

	for _, opOverride := range opOverrides {
		if opOverride.StaticEquals != nil {
			return staticEquals(opOverride, a, b, state)
		}
	}

	// the options apply to the field, the left one when both operands are fields
	opts := overridesCmpOpts(opOverrides)
	if a.Field != "" {
		a.StringCmpOpts = a.StringCmpOpts.merge(opts)
	} else if b.Field != "" {
		b.StringCmpOpts = b.StringCmpOpts.merge(opts)
	}

	// the static operand matches as a glob, its value registered as such
	if globPatterns(opOverrides) {
		if a.ValueType == PatternValueType {
			a.ValueType = GlobValueType
		} else if b.ValueType == PatternValueType {
			b.ValueType = GlobValueType
		}
	}

	// each override adds the extra inputs of the left field, else the ones of
	// the right field, an extra input being the left operand of its comparison
	evaluator, err := StringEquals(a, b, state)
	if err != nil {
		return nil, err
	}
	evaluators := []*BoolEvaluator{evaluator}
	for _, opOverride := range opOverrides {
		inputs, other := fieldExtraInputs(opOverride, a.Field, opts), b
		if len(inputs) == 0 {
			inputs, other = fieldExtraInputs(opOverride, b.Field, opts), a
		}
		for _, input := range inputs {
			evaluator, err := StringEquals(input, other, state)
			if err != nil {
				return nil, err
			}
			evaluators = append(evaluators, evaluator)
		}
	}
	return orInputs(evaluators, state)
}

// StringArrayContainsWrapper compiles `a in b`, b an array, with the operator
// overrides of a, else of b
func StringArrayContainsWrapper(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
	opOverrides := fieldOpOverrides(a.OpOverrides, b.OpOverrides)
	if len(opOverrides) == 0 {
		return StringArrayContains(a, b, state)
	}

	if err := checkEqualsOnly(opOverrides, fieldOf(a.Field, b.Field)); err != nil {
		return nil, err
	}

	opts := overridesCmpOpts(opOverrides)
	if a.Field != "" {
		a.StringCmpOpts = a.StringCmpOpts.merge(opts)
	} else if b.Field != "" {
		b.StringCmpOpts = b.StringCmpOpts.merge(opts)
	}

	if globPatterns(opOverrides) && a.ValueType == PatternValueType {
		a.ValueType = GlobValueType
	}

	// the extra inputs are the ones of the scalar, an array having none
	cmp := func(input *StringEvaluator) (*BoolEvaluator, error) {
		return StringArrayContains(input, b, state)
	}
	return compareInputs(a, opOverrides, opts, cmp, state)
}

// stringValuesContainsWrapper compiles `a in b`, b literal values, with the
// operator overrides of a
func stringValuesContainsWrapper(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	opOverrides := a.OpOverrides
	if len(opOverrides) == 0 {
		return StringValuesContains(a, b, state)
	}

	if err := checkEqualsOnly(opOverrides, a.Field); err != nil {
		return nil, err
	}

	opts := overridesCmpOpts(opOverrides)
	if a.Field != "" {
		a.StringCmpOpts = a.StringCmpOpts.merge(opts)
	}

	if globPatterns(opOverrides) {
		if a.ValueType == PatternValueType {
			a.ValueType = GlobValueType
		} else {
			b = globValues(b)
		}
	}

	// the values are compiled once, by the first comparison, with the options
	// of the field, and shared by the comparisons of the field and of its extra
	// inputs
	compiled := false
	cmp := func(input *StringEvaluator) (*BoolEvaluator, error) {
		evaluator, err := stringValuesContains(input, b, state, compiled)
		compiled = true
		return evaluator, err
	}
	return compareInputs(a, opOverrides, opts, cmp, state)
}

// stringArrayMatchesWrapper compiles `a in b`, a an array and b literal values,
// with the operator overrides of a
func stringArrayMatchesWrapper(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	opOverrides := a.OpOverrides
	if len(opOverrides) == 0 {
		return StringArrayMatches(a, b, state)
	}

	if err := checkEqualsOnly(opOverrides, a.Field); err != nil {
		return nil, err
	}

	opts := overridesCmpOpts(opOverrides)
	if a.Field != "" {
		a.StringCmpOpts = a.StringCmpOpts.merge(opts)
	}

	if globPatterns(opOverrides) {
		b = globValues(b)
	}

	// an array has no extra inputs
	return StringArrayMatches(a, b, state)
}
