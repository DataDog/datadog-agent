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

func TestLowerCaseEquals(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "BAR",
			ValueType: ScalarValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(CaseInsensitiveCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "FOO",
			ValueType: ScalarValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(CaseInsensitiveCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("glob", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "FO*",
			ValueType: PatternValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(CaseInsensitiveCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("regex", func(t *testing.T) {
		a := &StringEvaluator{
			Value:     "FO.*",
			ValueType: RegexpValueType,
		}

		b := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = overridesCmp(CaseInsensitiveCmp).StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "BAR"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "aaa", Type: ScalarValueType})
		values.AppendFieldValue(FieldValue{Value: "foo", Type: ScalarValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "FOO"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "aaa", Type: ScalarValueType})
		values.AppendFieldValue(FieldValue{Value: "foo", Type: ScalarValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("glob", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "aaa", Type: ScalarValueType})
		values.AppendFieldValue(FieldValue{Value: "FOO*", Type: PatternValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("regex", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "aaa", Type: ScalarValueType})
		values.AppendFieldValue(FieldValue{Value: "FO.*", Type: RegexpValueType})

		b := &StringValuesEvaluator{
			Values: values,
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		values.AppendFieldValue(FieldValue{Value: "[Ff][Oo].*", Type: RegexpValueType})

		b = &StringValuesEvaluator{
			Values: values,
		}

		e, err = overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "FOO"
			},
		}

		var values StringValues
		values.AppendFieldValue(FieldValue{Value: "aaa", Type: ScalarValueType})
		values.AppendFieldValue(FieldValue{Value: "fo*", Type: PatternValueType})

		opts := StringCmpOpts{
			CaseInsensitive: true,
		}

		if err := values.Compile(opts); err != nil {
			t.Error(err)
		}

		b := &StringValuesEvaluator{
			EvalFnc: func(*Context) *StringValues {
				return &values
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseArrayContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "BAR"
			},
		}

		b := &StringArrayEvaluator{
			Values: []string{"aaa", "bbb"},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "FOO"
			},
		}

		b := &StringArrayEvaluator{
			Values: []string{"aaa", "foo"},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(*Context) string {
				return "foo"
			},
		}
		b := &StringArrayEvaluator{
			Field: "array",
			EvalFnc: func(*Context) []string {
				return []string{"aaa", "foo"}
			},
		}

		var ctx Context
		state := NewState(&testModel{})

		e, err := overridesCmp(CaseInsensitiveCmp).StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

// The options of the overrides of a field merge: a case insensitive extension
// is sanitized too.
func TestCaseInsensitiveExtension(t *testing.T) {
	for _, test := range []struct {
		value    string
		input    string
		expected bool
	}{
		{value: "txt", input: ".TXT", expected: true},
		{value: ".TXT", input: ".txt", expected: true},
		{value: "TXT", input: ".txt", expected: true},
		{value: "txt", input: ".exe", expected: false},
		// the sanitized value has a leading dot, an input without one doesn't match
		{value: "txt", input: "txt", expected: false},
	} {
		input := test.input
		field := &StringEvaluator{
			Field:       "field",
			EvalFnc:     func(*Context) string { return input },
			OpOverrides: []*OpOverrides{CaseInsensitiveCmp, ExtensionCmp},
		}
		e, err := StringEqualsWrapper(field, &StringEvaluator{Value: test.value, ValueType: ScalarValueType}, NewState(&testModel{}))
		assert.NoError(t, err)
		assert.Equal(t, test.expected, e.Eval(NewContext(nil)), "%s == %s", test.input, test.value)
	}
}
