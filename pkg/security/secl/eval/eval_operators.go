// Code generated - DO NOT EDIT.

package eval

func Or(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

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
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 || op2
				ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) || eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
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

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

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
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 || op2
				ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) || eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

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
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 || op2
			ctx.Logf("Evaluating %v || %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea || eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func And(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

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
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 && op2
				ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) && eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
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

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

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
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 && op2
				ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) && eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

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
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 && op2
			ctx.Logf("Evaluating %v && %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea && eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntNotEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 & op2
				ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) & eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea & eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 & op2
				ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) & eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEval: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 & op2
			ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) int {
			return ea & eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntOr(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 | op2
				ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) | eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea | eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 | op2
				ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) | eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEval: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 | op2
			ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) int {
			return ea | eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func IntXor(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 ^ op2
				ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) ^ eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &IntEvaluator{
			Value:     ea ^ eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 ^ op2
				ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) int {
				return ea(ctx) ^ eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &IntEvaluator{
		DebugEval: func(ctx *Context) int {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 ^ op2
			ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) int {
			return ea ^ eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func StringNotEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea == eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) == eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 == op2
			ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea == eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func BoolNotEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea != eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) != eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 != op2
			ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea != eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 > op2
				ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) > eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea > eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 > op2
				ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) > eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 > op2
			ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea > eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 >= op2
				ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) >= eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea >= eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 >= op2
				ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) >= eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 >= op2
			ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea >= eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 < op2
				ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) < eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea < eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 < op2
				ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) < eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 < op2
			ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea < eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *state) *BoolEvaluator {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 <= op2
				ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) <= eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		ea, eb := a.Value, b.Value

		return &BoolEvaluator{
			Value:     ea <= eb,
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 <= op2
				ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) bool {
				return ea(ctx) <= eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
	}

	return &BoolEvaluator{
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 <= op2
			ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) bool {
			return ea <= eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
}
