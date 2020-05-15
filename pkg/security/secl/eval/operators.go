package eval

import (
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func IntNot(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Eval != nil {
		ea := a.Eval
		return &IntEvaluator{
			Eval: func(ctx *Context) int {
				return ^ea(ctx)
			},
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := ^ea(ctx)
				ctx.Logf("Evaluation ^%d => %d", op, result)
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     ^a.Value,
		isPartial: isPartialLeaf,
	}
}

func StringMatches(a *StringEvaluator, b *StringEvaluator, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	if b.Eval != nil {
		return nil, errors.New("regex has to be a scalar string")
	}

	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	p := strings.ReplaceAll(b.Value, "*", ".*")
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}

	if a.Field != "" {
		state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: PatternValueType})
	}

	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				result := re.MatchString(ea(ctx))
				if not {
					return !result
				}
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op := ea(ctx)
				result := re.MatchString(op)
				if not {
					return !result
				}
				ctx.Logf("Evaluating %s ~= %s => %v", op, p, result)
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}, nil
	}

	ea := true
	if !isPartialLeaf {
		ea = re.MatchString(a.Value)
		if not {
			return &BoolEvaluator{
				Value:     !ea,
				isPartial: isPartialLeaf,
			}, nil
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		isPartial: isPartialLeaf,
	}, nil
}

func Not(a *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Eval != nil {
		ea := func(ctx *Context) bool {
			return !a.Eval(ctx)
		}

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
		}

		return &BoolEvaluator{
			Eval: ea,
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op := a.Eval(ctx)
				result := !op
				ctx.Logf("Evaluating ! %v => %v", op, result)
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}
	}

	value := true
	if !isPartialLeaf {
		value = !a.Value
	}

	return &BoolEvaluator{
		Value:     value,
		isPartial: isPartialLeaf,
	}
}

func Minus(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Eval != nil {
		ea := a.Eval
		return &IntEvaluator{
			Eval: func(ctx *Context) int {
				return -ea(ctx)
			},
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := -op
				ctx.Logf("Evaluating -%d => %d", op, result)
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     -a.Value,
		isPartial: isPartialLeaf,
	}
}

func StringArrayContains(a *StringEvaluator, b *StringArray, not bool, opts *Opts, state *state) *BoolEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		for _, value := range b.Values {
			state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType})
		}
	}

	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				s := ea(ctx)
				i := sort.SearchStrings(b.Values, s)
				result := i < len(b.Values) && b.Values[i] == s
				if not {
					result = !result
				}
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				s := ea(ctx)
				i := sort.SearchStrings(b.Values, s)
				result := i < len(b.Values) && b.Values[i] == s
				ctx.Logf("Evaluating %s in %+v => %v", s, b.Values, result)
				if not {
					result = !result
				}
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}
	}

	ea := true
	if !isPartialLeaf {
		i := sort.SearchStrings(b.Values, a.Value)
		ea = i < len(b.Values) && b.Values[i] == a.Value
		if not {
			ea = !ea
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		isPartial: isPartialLeaf,
	}
}

func IntArrayContains(a *IntEvaluator, b *IntArray, not bool, opts *Opts, state *state) *BoolEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		for _, value := range b.Values {
			state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType})
		}
	}

	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				ctx.evalDepth++
				n := ea(ctx)
				i := sort.SearchInts(b.Values, n)
				result := i < len(b.Values) && b.Values[i] == n
				if not {
					result = !result
				}
				ctx.evalDepth--
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				n := ea(ctx)
				i := sort.SearchInts(b.Values, n)
				result := i < len(b.Values) && b.Values[i] == n
				if not {
					result = !result
				}
				ctx.Logf("Evaluating %d in %+v => %v", n, b.Values, result)
				ctx.evalDepth--
				return result
			},
			isPartial: isPartialLeaf,
		}
	}

	ea := true
	if !isPartialLeaf {
		i := sort.SearchInts(b.Values, a.Value)
		ea = i < len(b.Values) && b.Values[i] == a.Value
		if not {
			ea = !ea
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		isPartial: isPartialLeaf,
	}
}
