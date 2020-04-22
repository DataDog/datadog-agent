// Code generated - DO NOT EDIT.

package eval

func Or(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		if opts.Field != "" {
			if a.IsPartialLeaf {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.IsPartialLeaf {
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
			IsPartialLeaf: isPartialLeaf,
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

		if opts.Field != "" && a.IsPartialLeaf {
			ea = func(ctx *Context) bool {
				return true
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
			IsPartialLeaf: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if opts.Field != "" && b.IsPartialLeaf {
		eb = func(ctx *Context) bool {
			return true
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func And(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		if opts.Field != "" {
			if a.IsPartialLeaf {
				ea = func(ctx *Context) bool {
					return true
				}
			}
			if b.IsPartialLeaf {
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
			IsPartialLeaf: isPartialLeaf,
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

		if opts.Field != "" && a.IsPartialLeaf {
			ea = func(ctx *Context) bool {
				return true
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
			IsPartialLeaf: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if opts.Field != "" && b.IsPartialLeaf {
		eb = func(ctx *Context) bool {
			return true
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func IntEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func IntNotEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func IntOr(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func IntXor(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func StringNotEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func BoolNotEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {
	var isPartialLeaf bool

	if a.Field != "" || b.Field != "" {
		if a.Field != opts.Field && b.Field != opts.Field {
			isPartialLeaf = true
		}
		if a.Field != "" && b.Field != "" {
			isPartialLeaf = true
		}
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
			IsPartialLeaf: isPartialLeaf,
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
			IsPartialLeaf: isPartialLeaf,
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
		IsPartialLeaf: isPartialLeaf,
	}
}
