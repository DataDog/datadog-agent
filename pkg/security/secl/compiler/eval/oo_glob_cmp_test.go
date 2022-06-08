// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
			EvalFnc: func(ctx *Context) string {
				return "/2/abc/3"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringEquals(a, b, nilReplCtx(), state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = GlobCmp.StringEquals(b, a, nilReplCtx(), state)
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
			EvalFnc: func(ctx *Context) string {
				return "/2/abc/3"
			},
		}

		var ctx Context
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringEquals(a, b, nilReplCtx(), state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = GlobCmp.StringEquals(b, a, nilReplCtx(), state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestOPOverrideGlobContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringValuesContains(a, b, nilReplCtx(), state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("match", func(t *testing.T) {
		a := &StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *Context) string {
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringValuesContains(a, b, nilReplCtx(), state)
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringArrayMatches(a, b, nilReplCtx(), state)
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringArrayMatches(a, b, nilReplCtx(), state)
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringArrayContains(a, b, nilReplCtx(), state)
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
		state := NewState(&testModel{}, "", nil)

		e, err := GlobCmp.StringArrayContains(a, b, nilReplCtx(), state)
		assert.Empty(t, err)
		assert.True(t, e.Value)
	})
}
