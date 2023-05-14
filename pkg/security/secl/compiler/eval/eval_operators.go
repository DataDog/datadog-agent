// Code generated - DO NOT EDIT.

package eval

import (
	"errors"
)

func IntEquals(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) == eb(ctx)
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
			Value:           ea == eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) == eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea == eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		return nil, errors.New("full dynamic bitmask operation not supported")
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &IntEvaluator{
			Value:           ea & eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) int {
			return ea(ctx) & eb
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea & eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntOr(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		return nil, errors.New("full dynamic bitmask operation not supported")
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &IntEvaluator{
			Value:           ea | eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) int {
			return ea(ctx) | eb
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea | eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntXor(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		return nil, errors.New("full dynamic bitmask operation not supported")
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &IntEvaluator{
			Value:           ea ^ eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) int {
			return ea(ctx) ^ eb
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea ^ eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) == eb(ctx)
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
			Value:           ea == eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) == eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea == eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) > eb(ctx)
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
			Value:           ea > eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) > eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea > eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) >= eb(ctx)
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
			Value:           ea >= eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) >= eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea >= eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) < eb(ctx)
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
			Value:           ea < eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) < eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea < eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) <= eb(ctx)
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
			Value:           ea <= eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ea(ctx) <= eb
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ea <= eb(ctx)
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) < int64(eb(ctx))
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
			Value:           ctx.Now().UnixNano()-int64(ea) < int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) < int64(eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ctx.Now().UnixNano()-int64(ea) < int64(eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) <= int64(eb(ctx))
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
			Value:           ctx.Now().UnixNano()-int64(ea) <= int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) <= int64(eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ctx.Now().UnixNano()-int64(ea) <= int64(eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) > int64(eb(ctx))
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
			Value:           ctx.Now().UnixNano()-int64(ea) > int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) > int64(eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ctx.Now().UnixNano()-int64(ea) > int64(eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) >= int64(eb(ctx))
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
			Value:           ctx.Now().UnixNano()-int64(ea) >= int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			return ctx.Now().UnixNano()-int64(ea(ctx)) >= int64(eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Field:           a.Field,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return ctx.Now().UnixNano()-int64(ea) >= int64(eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Field:           b.Field,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayEquals(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if a == v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func BoolArrayEquals(a *BoolEvaluator, b *BoolArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a bool, b []bool) bool {
		for _, v := range b {
			if a == v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayGreaterThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if a > v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayGreaterOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if a >= v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayLesserThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if a < v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayLesserOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if a <= v {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayLesserThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) < int64(v) {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayLesserOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) <= int64(v) {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayGreaterThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) > int64(v) {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayGreaterOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) bool {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) >= int64(v) {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &BoolEvaluator{
			Value:           arrayOp(ctx, ea, eb),
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}
