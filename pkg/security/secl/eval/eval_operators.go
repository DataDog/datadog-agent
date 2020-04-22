// Code generated - DO NOT EDIT.

package eval

import (
	"fmt"
)

func Or(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {

	fmt.Printf("YYYYYYYYYYYYYYYYYYYYYYY\n")

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) || eb(ctx)
		}

		if opts.PartialField != "" {
			if a.IsOpLeaf && !b.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return ea(ctx) || false
				}
			} else if !a.IsOpLeaf && b.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return false || eb(ctx)
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) || eb
		}

		if opts.PartialField != "" {
			if !a.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return false || eb
				}
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea || eb(ctx)
	}

	if opts.PartialField != "" {
		if !b.IsOpLeaf {
			eval = func(ctx *Context) bool {
				return ea || false
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func And(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {

	fmt.Printf("YYYYYYYYYYYYYYYYYYYYYYY\n")

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) && eb(ctx)
		}

		if opts.PartialField != "" {
			if a.IsOpLeaf && !b.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return ea(ctx) && true
				}
			} else if !a.IsOpLeaf && b.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return true && eb(ctx)
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) && eb
		}

		if opts.PartialField != "" {
			if !a.IsOpLeaf {
				eval = func(ctx *Context) bool {
					return true && eb
				}
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea && eb(ctx)
	}

	if opts.PartialField != "" {
		if !b.IsOpLeaf {
			eval = func(ctx *Context) bool {
				return ea && true
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func IntEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea == eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func IntNotEquals(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea != eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func IntAnd(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) int {
			return ea(ctx) & eb(ctx)
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 & op2
				ctx.Logf("Evaluating %v & %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) int {
			return ea(ctx) & eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) int {
		return ea & eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func IntOr(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) int {
			return ea(ctx) | eb(ctx)
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 | op2
				ctx.Logf("Evaluating %v | %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) int {
			return ea(ctx) | eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) int {
		return ea | eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func IntXor(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *IntEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) int {
			return ea(ctx) ^ eb(ctx)
		}

		return &IntEvaluator{
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 ^ op2
				ctx.Logf("Evaluating %v ^ %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) int {
			return ea(ctx) ^ eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) int {
		return ea ^ eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func StringEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea == eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func StringNotEquals(a *StringEvaluator, b *StringEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea != eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func BoolEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 == op2
				ctx.Logf("Evaluating %v == %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) == eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea == eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func BoolNotEquals(a *BoolEvaluator, b *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 != op2
				ctx.Logf("Evaluating %v != %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) != eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea != eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func GreaterThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) > eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 > op2
				ctx.Logf("Evaluating %v > %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) > eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea > eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func GreaterOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) >= eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 >= op2
				ctx.Logf("Evaluating %v >= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) >= eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea >= eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func LesserThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) < eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 < op2
				ctx.Logf("Evaluating %v < %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) < eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea < eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}

func LesserOrEqualThan(a *IntEvaluator, b *IntEvaluator, opts *Opts, state *State) *BoolEvaluator {

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) bool {
			return ea(ctx) <= eb(ctx)
		}

		return &BoolEvaluator{
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 <= op2
				ctx.Logf("Evaluating %v <= %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
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

		eval := func(ctx *Context) bool {
			return ea(ctx) <= eb
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
			Eval:     eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) bool {
		return ea <= eb(ctx)
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
		Eval:     eval,
		IsOpLeaf: isOpLeaf,
	}
}
