package eval

import (
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func IntNot(a *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
	isPartialLeaf := a.IsPartial
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
			IsPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     ^a.Value,
		IsPartial: isPartialLeaf,
	}
}

func StringMatches(a *StringEvaluator, b *StringEvaluator, not bool, opts *Opts, state *State) (*BoolEvaluator, error) {
	if b.Eval != nil {
		return nil, errors.New("regex has to be a scalar string")
	}

	isPartialLeaf := a.IsPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	p := strings.ReplaceAll(b.Value, "*", ".*")
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
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
			IsPartial: isPartialLeaf,
		}, nil
	}

	ea := true
	if !isPartialLeaf {
		ea = re.MatchString(a.Value)
		if not {
			return &BoolEvaluator{
				Value:     !ea,
				IsPartial: isPartialLeaf,
			}, nil
		}
	}

	return &BoolEvaluator{
		Value:     ea,
		IsPartial: isPartialLeaf,
	}, nil
}

func Not(a *BoolEvaluator, opts *Opts, state *State) *BoolEvaluator {
	isPartialLeaf := a.IsPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Eval != nil {
		ea := func(ctx *Context) bool {
			return !a.Eval(ctx)
		}

		if state.field != "" {
			if a.IsPartial {
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
			IsPartial: isPartialLeaf,
		}
	}

	value := true
	if !isPartialLeaf {
		value = !a.Value
	}

	return &BoolEvaluator{
		Value:     value,
		IsPartial: isPartialLeaf,
	}
}

func Minus(a *IntEvaluator, opts *Opts, state *State) *IntEvaluator {
	isPartialLeaf := a.IsPartial
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
			IsPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     -a.Value,
		IsPartial: isPartialLeaf,
	}
}

func StringArrayContains(a *StringEvaluator, b *StringArray, not bool, opts *Opts, state *State) *BoolEvaluator {
	isPartialLeaf := a.IsPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
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
			IsPartial: isPartialLeaf,
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
		IsPartial: isPartialLeaf,
	}
}

func IntArrayContains(a *IntEvaluator, b *IntArray, not bool, opts *Opts, state *State) *BoolEvaluator {
	isPartialLeaf := a.IsPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
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
			IsPartial: isPartialLeaf,
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
		IsPartial: isPartialLeaf,
	}
}
