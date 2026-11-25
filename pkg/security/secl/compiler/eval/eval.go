// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate operators -output eval_operators.go

// Package eval holds eval related files
package eval

import (
	"fmt"
	"net"
	"reflect"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// defines factor applied by specific operator
const (
	FunctionWeight       = 5
	InArrayWeight        = 10
	HandlerWeight        = 50
	PatternWeight        = 80
	RegexpWeight         = 100
	InPatternArrayWeight = 1000
	IteratorWeight       = 2000
)

// BoolEvalFnc describe a eval function return a boolean
type BoolEvalFnc = func(ctx *Context) bool

// wrapEvaluatorWithArrayIndex wraps an array evaluator to return the element
// at an index, the zero value out of the array
func wrapEvaluatorWithArrayIndex(evaluator interface{}, index *IntEvaluator, field Field) (Evaluator, error) {
	indexAt := index.EvalFnc
	if indexAt == nil {
		value := index.Value
		indexAt = func(*Context) int { return value }
	}

	switch arrayEval := evaluator.(type) {
	case *StringArrayEvaluator:
		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				array, i := arrayEval.Eval(ctx).([]string), indexAt(ctx)
				if i < 0 || i >= len(array) {
					return ""
				}
				return array[i]
			},
			Field:  field,
			Weight: arrayEval.Weight + index.Weight,
		}, nil
	case *IntArrayEvaluator:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				array, i := arrayEval.Eval(ctx).([]int), indexAt(ctx)
				if i < 0 || i >= len(array) {
					return 0
				}
				return array[i]
			},
			Field:  field,
			Weight: arrayEval.Weight + index.Weight,
		}, nil
	case *BoolArrayEvaluator:
		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool {
				array, i := arrayEval.Eval(ctx).([]bool), indexAt(ctx)
				if i < 0 || i >= len(array) {
					return false
				}
				return array[i]
			},
			Field:  field,
			Weight: arrayEval.Weight + index.Weight,
		}, nil
	case *CIDRArrayEvaluator:
		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				array, i := arrayEval.Eval(ctx).([]net.IPNet), indexAt(ctx)
				if i < 0 || i >= len(array) {
					return net.IPNet{}
				}
				return array[i]
			},
			Field:  field,
			Weight: arrayEval.Weight + index.Weight,
		}, nil
	default:
		return nil, fmt.Errorf("field '%s' is not an array type or array type is not supported for index access", field)
	}
}

// wrapEvaluatorWithKey wraps a dictionary evaluator to return the value at a
// key, the empty string for a missing key
func wrapEvaluatorWithKey(evaluator interface{}, key *StringEvaluator, field Field) (Evaluator, error) {
	keyOf := key.EvalFnc
	if keyOf == nil {
		value := key.Value
		keyOf = func(*Context) string { return value }
	}

	switch dictEval := evaluator.(type) {
	case *StringMapEvaluator:
		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				return dictEval.EvalFnc(ctx)[keyOf(ctx)]
			},
			Field:  field,
			Weight: dictEval.Weight + key.Weight,
			Offset: dictEval.Offset,
		}, nil
	case *StringMapArrayEvaluator:
		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				dicts, k := dictEval.EvalFnc(ctx), keyOf(ctx)
				values := make([]string, len(dicts))
				for i, dict := range dicts {
					values[i] = dict[k]
				}
				return values
			},
			Field:  field,
			Weight: dictEval.Weight + key.Weight,
			Offset: dictEval.Offset,
		}, nil
	default:
		return nil, fmt.Errorf("field '%s' is not a dictionary, it can't be accessed at a key", field)
	}
}

func dictContains(keys interface{}, dict interface{}, all bool, keysPos ast.Position) (*BoolEvaluator, bool, error) {
	var (
		dicts  func(ctx *Context) []map[string]string
		weight int
	)
	switch dict := dict.(type) {
	case *StringMapEvaluator:
		dicts, weight = func(ctx *Context) []map[string]string { return []map[string]string{dict.EvalFnc(ctx)} }, dict.Weight
	case *StringMapArrayEvaluator:
		dicts, weight = dict.EvalFnc, dict.Weight
	default:
		return nil, false, nil
	}

	var (
		values   *StringValues
		keysOf   func(ctx *Context) []string
		matchers []StringMatcher
	)
	switch keys := keys.(type) {
	case *StringEvaluator:
		if keys.EvalFnc == nil {
			values = &StringValues{}
			values.AppendFieldValue(FieldValue{Value: keys.Value, Type: keys.ValueType})
			break
		}
		keysOf, weight = func(ctx *Context) []string { return []string{keys.EvalFnc(ctx)} }, weight+keys.Weight
	case *StringValuesEvaluator:
		values = &keys.Values
	case *StringArrayEvaluator:
		keysOf, weight = func(ctx *Context) []string { return keys.Eval(ctx).([]string) }, weight+keys.Weight
	default:
		return nil, true, NewTypeError(keysPos, reflect.String)
	}

	if values != nil {
		if err := values.Compile(StringCmpOpts{}); err != nil {
			return nil, true, NewError(keysPos, "%s", err)
		}
		scalars := values.GetScalarValues()
		keysOf, matchers = func(*Context) []string { return scalars }, values.GetStringMatchers()
	}

	matchesAKey := func(matcher StringMatcher, dict map[string]string) bool {
		for key := range dict {
			if matcher.Matches(key) {
				return true
			}
		}
		return false
	}
	// holds returns whether a dictionary holds any of the keys, or all of them:
	// the first key found decides for any, the first one missing for all
	holds := func(dict map[string]string, keys []string) bool {
		for _, key := range keys {
			if _, ok := dict[key]; ok != all {
				return !all
			}
		}
		for _, matcher := range matchers {
			if matchesAKey(matcher, dict) != all {
				return !all
			}
		}
		return all
	}

	return &BoolEvaluator{Weight: weight, EvalFnc: func(ctx *Context) bool {
		keys := keysOf(ctx)
		for _, dict := range dicts(ctx) {
			if holds(dict, keys) {
				return true
			}
		}
		return false
	}}, true, nil
}

// fieldToEvaluator compiles a reference to a field.
func fieldToEvaluator(node *ast.FieldRef, state *State) (interface{}, ast.Position, error) {
	pos, field := node.Pos, node.Name
	var regID RegisterID
	if node.Iter != nil {
		regID = node.Iter.Register
	}

	evaluator, err := state.model.GetEvaluator(field, regID, pos.Offset)
	if err != nil {
		return nil, pos, err
	}

	state.UpdateFields(field)

	if node.Iter != nil {
		// avoid wildcard register for the moment
		if regID == "_" {
			return nil, pos, NewError(pos, "`_` can't be used as a iterator variable name")
		}

		// avoid using the same register on two different fields
		if slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID && r.Field != node.Iter.Iterator
		}) {
			return nil, pos, NewError(pos, "iterator variable used by different fields '%s'", regID)
		}

		if !slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID
		}) {
			state.registers = append(state.registers, Register{ID: regID, Field: node.Iter.Iterator})
		}
	}

	return evaluator, pos, nil
}

func evaluatorFromVariable(varname string, pos ast.Position, opts *Opts, state *State) (interface{}, ast.Position, error) {
	var variableEvaluator interface{}
	variable := opts.VariableStore.Get(varname)
	if variable != nil {
		return variable.GetEvaluator(), pos, nil
	}

	if before, ok := strings.CutSuffix(varname, ".length"); ok {
		trimmedVariable := before
		if variable = opts.VariableStore.Get(trimmedVariable); variable != nil {
			variableEvaluator = variable.GetEvaluator()
			switch evaluator := variableEvaluator.(type) {
			case *StringArrayEvaluator:
				return &IntEvaluator{
					EvalFnc: func(ctx *Context) int {
						v := evaluator.Eval(ctx)
						return len(v.([]string))
					},
				}, pos, nil
			case *StringEvaluator:
				return &IntEvaluator{
					EvalFnc: func(ctx *Context) int {
						v := evaluator.Eval(ctx)
						return len(v.(string))
					},
				}, pos, nil
			case *IntArrayEvaluator:
				return &IntEvaluator{
					EvalFnc: func(ctx *Context) int {
						v := evaluator.Eval(ctx)
						return len(v.([]int))
					},
				}, pos, nil
			case *CIDRArrayEvaluator:
				return &IntEvaluator{
					EvalFnc: func(ctx *Context) int {
						v := evaluator.Eval(ctx)
						return len(v.([]net.IPNet))
					},
				}, pos, nil
			default:
				return nil, pos, NewError(pos, "'length' cannot be used on '%s'", trimmedVariable)
			}
		}

	}

	// No fallback to field - variables and fields are explicitly separated
	if state != nil && state.model != nil {
		if _, err := state.model.GetEvaluator(varname, "", 0); err == nil {
			return nil, pos, NewError(pos, "variable '%s' doesn't exist, use %%{%s} to reference the field", varname, varname)
		}
	}
	return nil, pos, NewError(pos, "variable '%s' doesn't exist", varname)
}

// StringEqualsWrapper makes use of operator overrides
func StringEqualsWrapper(a *StringEvaluator, b *StringEvaluator, state *State) (*BoolEvaluator, error) {
	var evaluator *BoolEvaluator
	var opOverrides []*OpOverrides

	if len(a.OpOverrides) > 0 {
		opOverrides = a.OpOverrides
	} else if len(b.OpOverrides) > 0 {
		opOverrides = b.OpOverrides
	}

	for _, opOverride := range opOverrides {
		if opOverride.StringEquals != nil {
			eval, err := opOverride.StringEquals(a, b, state)
			if err != nil {
				return nil, err
			}

			if evaluator != nil {
				or, err := Or(evaluator, eval, state)
				if err != nil {
					return nil, err
				}
				evaluator = or
			} else {
				evaluator = eval
			}
		}
	}

	// if evaluator is still nil at this point this means no override has been applied
	// in this case we use the default implementation
	if evaluator == nil {
		var err error
		evaluator, err = StringEquals(a, b, state)
		if err != nil {
			return nil, err
		}
	}

	return evaluator, nil
}

// StringArrayContainsWrapper makes use of operator overrides
func StringArrayContainsWrapper(a *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
	var evaluator *BoolEvaluator
	var opOverrides []*OpOverrides

	if len(a.OpOverrides) > 0 {
		opOverrides = a.OpOverrides
	} else if len(b.OpOverrides) > 0 {
		opOverrides = b.OpOverrides
	}

	for _, opOverride := range opOverrides {
		if opOverride.StringArrayContains != nil {
			eval, err := opOverride.StringArrayContains(a, b, state)
			if err != nil {
				return nil, err
			}

			if evaluator != nil {
				or, err := Or(evaluator, eval, state)
				if err != nil {
					return nil, err
				}
				evaluator = or
			} else {
				evaluator = eval
			}
		}
	}

	// if evaluator is still nil at this point this means no override has been applied
	// in this case we use the default implementation
	if evaluator == nil {
		var err error
		evaluator, err = StringArrayContains(a, b, state)
		if err != nil {
			return nil, err
		}
	}

	return evaluator, nil
}

// stringValuesContainsWrapper makes use of operator overrides
func stringValuesContainsWrapper(a *StringEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	var evaluator *BoolEvaluator
	var opOverrides []*OpOverrides

	if len(a.OpOverrides) > 0 {
		opOverrides = a.OpOverrides
	}

	for _, opOverride := range opOverrides {
		if opOverride.StringValuesContains != nil {
			eval, err := opOverride.StringValuesContains(a, b, state)
			if err != nil {
				return nil, err
			}

			if evaluator != nil {
				or, err := Or(evaluator, eval, state)
				if err != nil {
					return nil, err
				}
				evaluator = or
			} else {
				evaluator = eval
			}
		}
	}

	// if evaluator is still nil at this point this means no override has been applied
	// in this case we use the default implementation
	if evaluator == nil {
		var err error
		evaluator, err = StringValuesContains(a, b, state)
		if err != nil {
			return nil, err
		}
	}

	return evaluator, nil
}

// stringArrayMatchesWrapper makes use of operator overrides
func stringArrayMatchesWrapper(a *StringArrayEvaluator, b *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
	var evaluator *BoolEvaluator
	var opOverrides []*OpOverrides

	if len(a.OpOverrides) > 0 {
		opOverrides = a.OpOverrides
	}

	for _, opOverride := range opOverrides {
		if opOverride.StringArrayMatches != nil {
			eval, err := opOverride.StringArrayMatches(a, b, state)
			if err != nil {
				return nil, err
			}

			if evaluator != nil {
				or, err := Or(evaluator, eval, state)
				if err != nil {
					return nil, err
				}
				evaluator = or
			} else {
				evaluator = eval
			}
		}
	}

	// if evaluator is still nil at this point this means no override has been applied
	// in this case we use the default implementation
	if evaluator == nil {
		var err error
		evaluator, err = StringArrayMatches(a, b, state)
		if err != nil {
			return nil, err
		}
	}

	return evaluator, nil
}
