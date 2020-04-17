// Code generated - DO NOT EDIT.

package eval

func Or(a *BoolEvaluator, b *BoolEvaluator) *BoolEvaluator {
	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value || b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func And(a *BoolEvaluator, b *BoolEvaluator) *BoolEvaluator {
	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value && b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func IntEquals(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value == b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func IntNotEquals(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value != b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func IntAnd(a *IntEvaluator, b *IntEvaluator) *IntEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &IntEvaluator{
			Value: a.Value & b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func IntOr(a *IntEvaluator, b *IntEvaluator) *IntEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &IntEvaluator{
			Value: a.Value | b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func IntXor(a *IntEvaluator, b *IntEvaluator) *IntEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &IntEvaluator{
			Value: a.Value ^ b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func StringEquals(a *StringEvaluator, b *StringEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value == b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func StringNotEquals(a *StringEvaluator, b *StringEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value != b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value == b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func BoolNotEquals(a *BoolEvaluator, b *BoolEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value != b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value > b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value >= b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func LesserThan(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value < b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator) *BoolEvaluator {
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
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &BoolEvaluator{
			Value: a.Value <= b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval
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
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval
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
	}
}
