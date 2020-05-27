// Code generated - DO NOT EDIT.

package eval

func Or(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.isPartial {
				eb = func(ctx *Context) bool {
					return true
				}
			}
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 || op2
				ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) || eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		if state.field != "" {
			if a.isPartial {
				ea = true
			}
			if b.isPartial {
				eb = true
			}
		}

		return &BoolEvaluator{
			Value:     ea || eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.isPartial {
				eb = true
			}
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 || op2
				ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) || eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	if state.field != "" {
		if a.isPartial {
			ea = true
		}
		if b.isPartial {
			eb = func(ctx *Context) bool {
				return true
			}
		}
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 || op2
			ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea || eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func And(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.isPartial {
				eb = func(ctx *Context) bool {
					return true
				}
			}
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 && op2
				ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) && eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		if state.field != "" {
			if a.isPartial {
				ea = true
			}
			if b.isPartial {
				eb = true
			}
		}

		return &BoolEvaluator{
			Value:     ea && eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.isPartial {
				eb = true
			}
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 && op2
				ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) && eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	if state.field != "" {
		if a.isPartial {
			ea = true
		}
		if b.isPartial {
			eb = func(ctx *Context) bool {
				return true
			}
		}
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 && op2
			ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea && eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntNotEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 & op2
				ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) & eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea & eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 & op2
				ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) & eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEvalFnc: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 & op2
			ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) int {
			return ea & eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntOr(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 | op2
				ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) | eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea | eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 | op2
				ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) | eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEvalFnc: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 | op2
			ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) int {
			return ea | eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntXor(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 ^ op2
				ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) ^ eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea ^ eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEvalFnc: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 ^ op2
				ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) int {
				return ea(ctx) ^ eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEvalFnc: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 ^ op2
			ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) int {
			return ea ^ eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func StringNotEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func BoolNotEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 > op2
				ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) > eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea > eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 > op2
				ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) > eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 > op2
			ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea > eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 >= op2
				ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) >= eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea >= eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 >= op2
				ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) >= eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 >= op2
			ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea >= eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 < op2
				ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) < eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea < eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 < op2
				ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) < eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 < op2
			ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea < eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
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

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc
		dea, deb := a.DebugEvalFnc, b.DebugEvalFnc

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 <= op2
				ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) <= eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea <= eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value
		dea := a.DebugEvalFnc

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEvalFnc: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 <= op2
				ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			EvalFnc: func(ctx *Context) bool {
				return ea(ctx) <= eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.EvalFnc
	deb := b.DebugEvalFnc

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEvalFnc: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 <= op2
			ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		EvalFnc: func(ctx *Context) bool {
			return ea <= eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}
