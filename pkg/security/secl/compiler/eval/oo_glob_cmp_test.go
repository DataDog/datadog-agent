// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOPOverrideGlobEquals(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "/abc/",
			ValueType: PatternValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "/2/abc/3"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(GlobCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("match", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "*/abc/*",
			ValueType: PatternValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "/2/abc/3"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(GlobCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestOPOverrideGlobContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "/2/abc/3"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "/abc/", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "abc/*", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "*/abc", Type: PatternValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "/2/abc/3"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "*/abc/*", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "abc", Type: PatternValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestOPOverrideGlobArrayMatches(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringArrayEvaluator{
			Values: []string{"/2/abc/3"},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "abc", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "abc/*", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "*/abc", Type: PatternValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringArrayMatches(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Value)
	})

	t.Run("match", func(t *testing.T) {
		a := &StringArrayEvaluator{
			Values: []string{"/2/abc/3"},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "*/abc/*", Type: PatternValueType})
		values.AppendFieldValue(FieldValue{Value: "abc", Type: PatternValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringArrayMatches(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Value)
	})
}

func TestOPOverrideGlobArrayContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "/abc/",
			ValueType: PatternValueType,
		}

		b := &StringArrayEvaluator{
			Values: []string{"/2/abc/3"},
		}
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Value)
	})

	t.Run("match", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "*/abc/*",
			ValueType: PatternValueType,
		}

		b := &StringArrayEvaluator{
			Field:  "dont_forget_me_or_it_wont_compile_a",
			Values: []string{"/2/abc/3"},
		}
		state := NewState(&testModel{})

		e, err := overridesCmp(GlobCmp).StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Value)
	})
}

// opOverridesCmp compiles the string operators with an operator override set on
// the field operands
type opOverridesCmp struct {
	opOverrides []*OpOverrides
}

func overridesCmp(opOverride *OpOverrides) opOverridesCmp {
	return opOverridesCmp{opOverrides: []*OpOverrides{opOverride}}
}

func (c opOverridesCmp) StringEquals(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
	if a.Field != "" {
		a.OpOverrides = c.opOverrides
	}
	if b.Field != "" {
		b.OpOverrides = c.opOverrides
	}
	return StringEqualsWrapper(a, b, state)
}

func (c opOverridesCmp) StringValuesContains(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	a.OpOverrides = c.opOverrides
	return stringValuesContainsWrapper(a, b, state)
}

func (c opOverridesCmp) StringArrayContains(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
	if a.Field != "" {
		a.OpOverrides = c.opOverrides
	}
	if b.Field != "" {
		b.OpOverrides = c.opOverrides
	}
	return StringArrayContainsWrapper(a, b, state)
}

func (c opOverridesCmp) StringArrayMatches(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	a.OpOverrides = c.opOverrides
	return stringArrayMatchesWrapper(a, b, state)
}

// The value of a field is compared once whatever the number of overrides of the
// field, and with the options of all of them.
func TestOpOverridesFieldComparedOnce(t *testing.T) {
	var fieldEvals, extraEvals int
	extraInputs := &OpOverrides{
		GlobPatterns: true,
		ExtraInputs: func(field Field) []*StringEvaluator {
			return []*StringEvaluator{{
				Field: field,
				EvalFnc: func(*Context) string {
					extraEvals++
					return "/extra/abc"
				},
			}}
		},
	}
	newField := func() *StringEvaluator {
		return &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				fieldEvals++
				return "/field/abc"
			},
			OpOverrides: []*OpOverrides{GlobCmp, extraInputs},
		}
	}

	for _, test := range []struct {
		value    string
		expected bool
	}{
		{value: "/nope/*", expected: false},
		{value: "/field/*", expected: true},
		{value: "/extra/*", expected: true},
		// glob semantics for the field and for its extra input alike
		{value: "/*", expected: false},
	} {
		fieldEvals, extraEvals = 0, 0

		state := NewState(&testModel{})
		e, err := StringEqualsWrapper(newField(), &StringEvaluator{Value: test.value, ValueType: PatternValueType}, state)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, e.Eval(NewContext(nil)), test.value)
		assert.Equal(t, 1, fieldEvals, test.value)
		assert.LessOrEqual(t, extraEvals, 1, test.value)
		assert.Equal(t, []FieldValue{{Value: test.value, Type: GlobValueType}}, state.fieldValues["field"], test.value)

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: test.value, Type: PatternValueType})
		fieldEvals, extraEvals = 0, 0
		e, err = stringValuesContainsWrapper(newField(), &StringValuesEvaluator{Values: values}, NewState(&testModel{}))
		assert.NoError(t, err)
		assert.Equal(t, test.expected, e.Eval(NewContext(nil)), test.value)
		assert.Equal(t, 1, fieldEvals, test.value)
		assert.LessOrEqual(t, extraEvals, 1, test.value)
	}
}
