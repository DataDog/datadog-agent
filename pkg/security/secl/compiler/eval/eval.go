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

// wrapEvaluatorWithArrayIndex wraps an array evaluator to return a specific index
func wrapEvaluatorWithArrayIndex(evaluator interface{}, index int, field Field) (Evaluator, error) {
	switch arrayEval := evaluator.(type) {
	case *StringArrayEvaluator:
		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				array := arrayEval.Eval(ctx).([]string)
				if index < 0 || index >= len(array) {
					return ""
				}
				return array[index]
			},
			Field:  field,
			Weight: arrayEval.Weight,
		}, nil
	case *IntArrayEvaluator:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				array := arrayEval.Eval(ctx).([]int)
				if index < 0 || index >= len(array) {
					return 0
				}
				return array[index]
			},
			Field:  field,
			Weight: arrayEval.Weight,
		}, nil
	case *BoolArrayEvaluator:
		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool {
				array := arrayEval.Eval(ctx).([]bool)
				if index < 0 || index >= len(array) {
					return false
				}
				return array[index]
			},
			Field:  field,
			Weight: arrayEval.Weight,
		}, nil
	case *CIDRArrayEvaluator:
		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				array := arrayEval.Eval(ctx).([]net.IPNet)
				if index < 0 || index >= len(array) {
					return net.IPNet{}
				}
				return array[index]
			},
			Field:  field,
			Weight: arrayEval.Weight,
		}, nil
	default:
		return nil, fmt.Errorf("field '%s' is not an array type or array type is not supported for index access", field)
	}
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
