// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"regexp"
	"sort"

	"github.com/pkg/errors"
)

// IntNot - ^int operator
func IntNot(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		var evalFnc func(ctx *Context) int
		if opts.Debug {
			evalFnc = func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := ^ea(ctx)
				ctx.Logf("Evaluation ^%d => %d", op, result)
				ctx.evalDepth--
				return result
			}
		} else {
			evalFnc = func(ctx *Context) int {
				return ^ea(ctx)
			}
		}

		return &IntEvaluator{
			EvalFnc:   evalFnc,
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     ^a.Value,
		isPartial: isPartialLeaf,
	}
}

func patternToRegexp(pattern string) (*regexp.Regexp, error) {
	// only accept suffix wilcard, ex: /etc/* or /etc/*.conf
	if matched, err := regexp.Match(`\*.*/`, []byte(pattern)); err != nil || matched {
		return nil, &ErrInvalidPattern{Pattern: pattern}
	}

	// quote eveything except wilcard
	re := regexp.MustCompile(`[\.*+?()|\[\]{}^$]`)
	quoted := re.ReplaceAllStringFunc(pattern, func(s string) string {
		if s != "*" {
			return "\\" + s
		}
		return ".*"
	})

	return regexp.Compile("^" + quoted + "$")
}

// StringMatches - String pattern matching operator
func StringMatches(a *StringEvaluator, b *StringEvaluator, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	re, err := patternToRegexp(b.Value)
	if err != nil {
		return nil, err
	}

	if b.EvalFnc != nil {
		return nil, errors.New("regex has to be a scalar string")
	}

	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: PatternValueType}); err != nil {
			return nil, err
		}
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		var evalFnc func(ctx *Context) bool
		if opts.Debug {
			evalFnc = func(ctx *Context) bool {
				ctx.evalDepth++
				op := ea(ctx)
				result := re.MatchString(op)
				if not {
					return !result
				}
				ctx.Logf("Evaluating %s ~= %s => %v", op, re.String(), result)
				ctx.evalDepth--
				return result
			}
		} else {
			evalFnc = func(ctx *Context) bool {
				result := re.MatchString(ea(ctx))
				if not {
					return !result
				}
				return result
			}
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
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

// Not - !true operator
func Not(a *BoolEvaluator, opts *Opts, state *state) *BoolEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := func(ctx *Context) bool {
			return !a.EvalFnc(ctx)
		}

		if state.field != "" {
			if a.isPartial {
				ea = func(ctx *Context) bool {
					return true
				}
			}
		}

		var evalFnc func(ctx *Context) bool
		if opts.Debug {
			evalFnc = func(ctx *Context) bool {
				ctx.evalDepth++
				op := a.EvalFnc(ctx)
				result := !op
				ctx.Logf("Evaluating ! %v => %v", op, result)
				ctx.evalDepth--
				return result
			}
		} else {
			evalFnc = ea
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
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

// Minus - -int operator
func Minus(a *IntEvaluator, opts *Opts, state *state) *IntEvaluator {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		var evalFnc func(ctx *Context) int
		if opts.Debug {
			evalFnc = func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := -op
				ctx.Logf("Evaluating -%d => %d", op, result)
				ctx.evalDepth--
				return result
			}
		} else {
			evalFnc = func(ctx *Context) int {
				return -ea(ctx)
			}
		}

		return &IntEvaluator{
			EvalFnc:   evalFnc,
			isPartial: isPartialLeaf,
		}
	}

	return &IntEvaluator{
		Value:     -a.Value,
		isPartial: isPartialLeaf,
	}
}

// StringArrayContains - "test" in ["...", "..."] operator
func StringArrayContains(a *StringEvaluator, b *StringArray, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		var evalFnc func(ctx *Context) bool
		if opts.Debug {
			evalFnc = func(ctx *Context) bool {
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
			}
		} else {
			evalFnc = func(ctx *Context) bool {
				s := ea(ctx)
				i := sort.SearchStrings(b.Values, s)
				result := i < len(b.Values) && b.Values[i] == s
				if not {
					result = !result
				}
				return result
			}
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			isPartial: isPartialLeaf,
		}, nil
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
	}, nil
}

// IntArrayContains - 1 in [1, 2, 3] operator
func IntArrayContains(a *IntEvaluator, b *IntArray, not bool, opts *Opts, state *state) (*BoolEvaluator, error) {
	isPartialLeaf := a.isPartial
	if a.Field != "" && state.field != "" && a.Field != state.field {
		isPartialLeaf = true
	}

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if a.EvalFnc != nil {
		ea := a.EvalFnc

		var evalFnc func(ctx *Context) bool
		if opts.Debug {
			evalFnc = func(ctx *Context) bool {
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
			}
		} else {
			evalFnc = func(ctx *Context) bool {
				ctx.evalDepth++
				n := ea(ctx)
				i := sort.SearchInts(b.Values, n)
				result := i < len(b.Values) && b.Values[i] == n
				if not {
					result = !result
				}
				ctx.evalDepth--
				return result
			}
		}

		return &BoolEvaluator{
			EvalFnc:   evalFnc,
			isPartial: isPartialLeaf,
		}, nil
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
	}, nil
}
