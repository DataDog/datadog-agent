// Code generated - DO NOT EDIT.

package eval

import (
	"errors"
)

func IntEquals(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va == vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va == vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va == vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
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
			Weight:          a.Weight,
			isDeterministic: isDc,
			originField:     a.OriginField(),
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea & eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
		originField:     b.OriginField(),
	}, nil
}

func IntOr(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
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
			Weight:          a.Weight,
			isDeterministic: isDc,
			originField:     a.OriginField(),
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea | eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
		originField:     b.OriginField(),
	}, nil
}

func IntXor(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: BitmaskValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: BitmaskValueType}); err != nil {
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
			Weight:          a.Weight,
			isDeterministic: isDc,
			originField:     a.OriginField(),
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea ^ eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
		originField:     b.OriginField(),
	}, nil
}

func IntPlus(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) int {
			return ea(ctx) + eb(ctx)
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &IntEvaluator{
			Value:           ea + eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) int {
			return ea(ctx) + eb
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
			originField:     a.OriginField(),
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea + eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
		originField:     b.OriginField(),
	}, nil
}

func IntMinus(a *IntEvaluator, b *IntEvaluator, state *State) (*IntEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) int {
			return ea(ctx) - eb(ctx)
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &IntEvaluator{
			Value:           ea - eb,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) int {
			return ea(ctx) - eb
		}

		return &IntEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
			originField:     a.OriginField(),
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) int {
		return ea - eb(ctx)
	}

	return &IntEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
		originField:     b.OriginField(),
	}, nil
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va == vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va == vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va == vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va > vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va > vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va > vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va >= vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va >= vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va >= vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va < vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va < vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va < vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := va <= vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := va <= vb
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := va <= vb
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := ctx.Now().UnixNano()-int64(va) < int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := ctx.Now().UnixNano()-int64(va) < int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := ctx.Now().UnixNano()-int64(va) < int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := ctx.Now().UnixNano()-int64(va) <= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := ctx.Now().UnixNano()-int64(va) <= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := ctx.Now().UnixNano()-int64(va) <= int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := ctx.Now().UnixNano()-int64(va) > int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := ctx.Now().UnixNano()-int64(va) > int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := ctx.Now().UnixNano()-int64(va) > int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := ctx.Now().UnixNano()-int64(va) >= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			va, vb := ea(ctx), eb
			res := ctx.Now().UnixNano()-int64(va) >= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := ctx.Now().UnixNano()-int64(va) >= int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationEqual(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := ctx.Now().UnixNano()-int64(va) == int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           ctx.Now().UnixNano()-int64(ea) == int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := ctx.Now().UnixNano()-int64(va) == int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := ctx.Now().UnixNano()-int64(va) == int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserThanArithmeticOperation(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := int64(va) < int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           int64(ea) < int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := int64(va) < int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := int64(va) < int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationLesserOrEqualThanArithmeticOperation(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := int64(va) <= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           int64(ea) <= int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := int64(va) <= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := int64(va) <= int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterThanArithmeticOperation(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := int64(va) > int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           int64(ea) > int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := int64(va) > int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := int64(va) > int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationGreaterOrEqualThanArithmeticOperation(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: RangeValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := int64(va) >= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           int64(ea) >= int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := int64(va) >= int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := int64(va) >= int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationEqualArithmeticOperation(a *IntEvaluator, b *IntEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res := int64(va) == int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
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
			Value:           int64(ea) == int64(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res := int64(va) == int64(vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res := int64(va) == int64(vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayEquals(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if a == v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func BoolArrayEquals(a *BoolEvaluator, b *BoolArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a bool, b []bool) (bool, bool) {
		for _, v := range b {
			if a == v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayGreaterThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if a > v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayGreaterOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if a >= v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayLesserThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if a < v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func IntArrayLesserOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if a <= v {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayLesserThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) < int64(v) {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayLesserOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) <= int64(v) {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayGreaterThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) > int64(v) {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}

func DurationArrayGreaterOrEqualThan(a *IntEvaluator, b *IntArrayEvaluator, state *State) (*BoolEvaluator, error) {

	isDc := isArithmDeterministic(a, b, state)

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a int, b []int) (bool, int) {
		for _, v := range b {
			if ctx.Now().UnixNano()-int64(a) >= int64(v) {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &BoolEvaluator{
			Value:           res,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) bool {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &BoolEvaluator{
			EvalFnc:         evalFnc,
			Weight:          a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) bool {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr(MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &BoolEvaluator{
		EvalFnc:         evalFnc,
		Weight:          b.Weight,
		isDeterministic: isDc,
	}, nil
}
