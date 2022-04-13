// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestLowerCaseEquals(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Value:     "BAR",
			ValueType: eval.ScalarValueType,
		}

		b := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringEquals(a, b, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = DNSNameCmp.StringEquals(b, a, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Value:     "FOO",
			ValueType: eval.ScalarValueType,
		}

		b := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringEquals(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = DNSNameCmp.StringEquals(b, a, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("glob", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Value:     "FO*",
			ValueType: eval.PatternValueType,
		}

		b := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringEquals(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))

		e, err = DNSNameCmp.StringEquals(b, a, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("regex", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Value:     "FO.*",
			ValueType: eval.RegexpValueType,
		}

		b := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringEquals(a, b, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		e, err = DNSNameCmp.StringEquals(b, a, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "BAR"
			},
		}

		var values eval.StringValues
		values.AppendFieldValue(eval.FieldValue{Value: "aaa", Type: eval.ScalarValueType})
		values.AppendFieldValue(eval.FieldValue{Value: "foo", Type: eval.ScalarValueType})

		b := &eval.StringValuesEvaluator{
			Values: values,
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "FOO"
			},
		}

		var values eval.StringValues
		values.AppendFieldValue(eval.FieldValue{Value: "aaa", Type: eval.ScalarValueType})
		values.AppendFieldValue(eval.FieldValue{Value: "foo", Type: eval.ScalarValueType})

		b := &eval.StringValuesEvaluator{
			Values: values,
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("glob", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var values eval.StringValues
		values.AppendFieldValue(eval.FieldValue{Value: "aaa", Type: eval.ScalarValueType})
		values.AppendFieldValue(eval.FieldValue{Value: "FOO*", Type: eval.PatternValueType})

		b := &eval.StringValuesEvaluator{
			Values: values,
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("regex", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}

		var values eval.StringValues
		values.AppendFieldValue(eval.FieldValue{Value: "aaa", Type: eval.ScalarValueType})
		values.AppendFieldValue(eval.FieldValue{Value: "FO.*", Type: eval.RegexpValueType})

		b := &eval.StringValuesEvaluator{
			Values: values,
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))

		values.AppendFieldValue(eval.FieldValue{Value: "[Ff][Oo].*", Type: eval.RegexpValueType})

		b = &eval.StringValuesEvaluator{
			Values: values,
		}

		e, err = DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "FOO"
			},
		}

		var values eval.StringValues
		values.AppendFieldValue(eval.FieldValue{Value: "aaa", Type: eval.ScalarValueType})
		values.AppendFieldValue(eval.FieldValue{Value: "fo*", Type: eval.PatternValueType})

		opts := eval.StringCmpOpts{
			ScalarCaseInsensitive: true,
			GlobCaseInsensitive:   true,
		}

		if err := values.Compile(opts); err != nil {
			t.Error(err)
		}

		b := &eval.StringValuesEvaluator{
			EvalFnc: func(ctx *eval.Context) *eval.StringValues {
				return &values
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringValuesContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}

func TestLowerCaseArrayContains(t *testing.T) {
	t.Run("no-match", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "BAR"
			},
		}

		b := &eval.StringArrayEvaluator{
			Values: []string{"aaa", "bbb"},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringArrayContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.False(t, e.Eval(&ctx).(bool))
	})

	t.Run("scalar", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "FOO"
			},
		}

		b := &eval.StringArrayEvaluator{
			Values: []string{"aaa", "foo"},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringArrayContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})

	t.Run("eval", func(t *testing.T) {
		a := &eval.StringEvaluator{
			Field: "field",
			EvalFnc: func(ctx *eval.Context) string {
				return "foo"
			},
		}
		b := &eval.StringArrayEvaluator{
			Field: "array",
			EvalFnc: func(ctx *eval.Context) []string {
				return []string{"aaa", "foo"}
			},
		}

		var ctx eval.Context
		state := eval.NewState(&model.Model{}, "", nil)

		e, err := DNSNameCmp.StringArrayContains(a, b, nil, state)
		assert.Empty(t, err)
		assert.True(t, e.Eval(&ctx).(bool))
	})
}
