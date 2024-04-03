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
			EvalFnc: func(ctx *Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = CaseInsensitiveCmp.StringEquals(b, a, state)
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
			EvalFnc: func(ctx *Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = CaseInsensitiveCmp.StringEquals(b, a, state)
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
			EvalFnc: func(ctx *Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = CaseInsensitiveCmp.StringEquals(b, a, state)
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
			EvalFnc: func(ctx *Context) string {
				return "foo"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringEquals(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = CaseInsensitiveCmp.StringEquals(b, a, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("glob", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("regex", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		values.AppendFieldValue(FieldValue{Value: "[Ff][Oo].*", Type: RegexpValueType})

		b = &StringValuesEvaluator{
			Values: values,
		}

		e, err = CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
			EvalFnc: func(ctx *Context) *StringValues {
				return &values
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringValuesContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseArrayContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
				return "BAR"
			},
		}

		b := &StringArrayEvaluator{
			Values: []string{"aaa", "bbb"},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
				return "FOO"
			},
		}

		b := &StringArrayEvaluator{
			Values: []string{"aaa", "foo"},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
				return "foo"
			},
		}
		b := &StringArrayEvaluator{
			Field: "array",
			EvalFnc: func(ctx *Context) []string {
				return []string{"aaa", "foo"}
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := CaseInsensitiveCmp.StringArrayContains(a, b, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}
