// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/alecthomas/participle/lexer"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

// Field name
type Field = string

// IdentEvaluator represents the evaluator of an identifier
type IdentEvaluator struct {
	Eval func(ctx *Context) bool
}

// FieldValueType represents the type of the value of a field
type FieldValueType int

// Field value types
const (
	ScalarValueType  FieldValueType = 1
	PatternValueType FieldValueType = 2
	BitmaskValueType FieldValueType = 4
)

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType
}

// Opts are the options to be passed to the evaluator
type Opts struct {
	Debug     bool
	Constants map[string]interface{}
	Macros    map[MacroID]*Macro
}

// NewOptsWithParams initializes a new Opts instance with Debug and Constants parameters
func NewOptsWithParams(debug bool, constants map[string]interface{}) *Opts {
	return &Opts{
		Debug:     debug,
		Constants: constants,
		Macros:    make(map[MacroID]*Macro),
	}
}

// Evaluator is the interface of an evaluator
type Evaluator interface {
	Eval(ctx *Context) interface{}
}

// BoolEvaluator returns a bool as result of the evaluation
type BoolEvaluator struct {
	EvalFnc func(ctx *Context) bool
	Field   Field
	Value   bool

	isPartial bool
}

// Eval returns the result of the evaluation
func (b *BoolEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(nil)
}

// IntEvaluator returns an int as result of the evaluation
type IntEvaluator struct {
	EvalFnc func(ctx *Context) int
	Field   Field
	Value   int

	isPartial bool
}

// Eval returns the result of the evaluation
func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc func(ctx *Context) string
	Field   Field
	Value   string

	isPartial bool
}

// Eval returns the result of the evaluation
func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// StringArray represents an array of string values
type StringArray struct {
	Values []string
}

// IntArray represents an array of integer values
type IntArray struct {
	Values []int
}

func nodeToEvaluator(obj interface{}, opts *Opts, state *state) (interface{}, interface{}, lexer.Position, error) {
	switch obj := obj.(type) {
	case *ast.BooleanExpression:
		return nodeToEvaluator(obj.Expression, opts, state)
	case *ast.Expression:
		cmp, _, pos, err := nodeToEvaluator(obj.Comparison, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.Op != nil {
			cmpBool, ok := cmp.(*BoolEvaluator)
			if !ok {
				return nil, nil, obj.Pos, NewTypeError(obj.Pos, reflect.Bool)
			}

			next, _, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			nextBool, ok := next.(*BoolEvaluator)
			if !ok {
				return nil, nil, pos, NewTypeError(pos, reflect.Bool)
			}

			switch *obj.Op {
			case "||":
				boolEvaluator, err := Or(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, nil, obj.Pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			case "&&":
				boolEvaluator, err := And(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, nil, obj.Pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}
		return cmp, nil, obj.Pos, nil
	case *ast.BitOperation:
		unary, _, pos, err := nodeToEvaluator(obj.Unary, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.Op != nil {
			bitInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
			}

			next, _, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			nextInt, ok := next.(*IntEvaluator)
			if !ok {
				return nil, nil, pos, NewTypeError(pos, reflect.Int)
			}

			switch *obj.Op {
			case "&":
				intEvaluator, err := IntAnd(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return intEvaluator, nil, obj.Pos, nil
			case "|":
				IntEvaluator, err := IntOr(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return IntEvaluator, nil, obj.Pos, nil
			case "^":
				IntEvaluator, err := IntXor(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return IntEvaluator, nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}
		return unary, nil, obj.Pos, nil

	case *ast.Comparison:
		unary, _, pos, err := nodeToEvaluator(obj.BitOperation, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.ArrayComparison != nil {
			next, _, pos, err := nodeToEvaluator(obj.ArrayComparison, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch unary := unary.(type) {
			case *StringEvaluator:
				nextStringArray, ok := next.(*StringArray)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Array)
				}

				boolEvaluator, err := StringArrayContains(unary, nextStringArray, *obj.ArrayComparison.Op == "notin", opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			case *IntEvaluator:
				nextIntArray, ok := next.(*IntArray)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Array)
				}

				intEvaluator, err := IntArrayContains(unary, nextIntArray, *obj.ArrayComparison.Op == "notin", opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return intEvaluator, nil, obj.Pos, nil
			default:
				return nil, nil, pos, NewTypeError(pos, reflect.Array)
			}
		} else if obj.ScalarComparison != nil {
			next, _, pos, err := nodeToEvaluator(obj.ScalarComparison, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				nextBool, ok := next.(*BoolEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Bool)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err := BoolNotEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "==":
					boolEvaluator, err := BoolEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					stringEvaluator, err := StringNotEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return stringEvaluator, nil, pos, nil
				case "==":
					stringEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return stringEvaluator, nil, pos, nil
				case "=~", "!~":
					eval, err := StringMatches(unary, nextString, *obj.ScalarComparison.Op == "!~", opts, state)
					if err != nil {
						return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op, err)
					}
					return eval, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *IntEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					boolEvaluator, err := LesserThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "<=":
					boolEvaluator, err := LesserOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case ">":
					boolEvaluator, err := GreaterThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case ">=":
					boolEvaluator, err := GreaterOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "!=":
					boolEvaluator, err := IntNotEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "==":
					boolEvaluator, err := IntEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			}
		} else {
			return unary, nil, pos, nil
		}

	case *ast.ArrayComparison:
		return nodeToEvaluator(obj.Array, opts, state)

	case *ast.ScalarComparison:
		return nodeToEvaluator(obj.Next, opts, state)

	case *ast.Unary:
		if obj.Op != nil {
			unary, _, pos, err := nodeToEvaluator(obj.Unary, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch *obj.Op {
			case "!":
				unaryBool, ok := unary.(*BoolEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Bool)
				}

				return Not(unaryBool, opts, state), nil, obj.Pos, nil
			case "-":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				return Minus(unaryInt, opts, state), nil, pos, nil
			case "^":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				return IntNot(unaryInt, opts, state), nil, pos, nil
			}
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}

		return nodeToEvaluator(obj.Primary, opts, state)
	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			if accessor, ok := opts.Constants[*obj.Ident]; ok {
				return accessor, nil, obj.Pos, nil
			}

			if state.macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}

			accessor, err := state.model.GetEvaluator(*obj.Ident)
			if err != nil {
				return nil, nil, obj.Pos, err
			}

			state.UpdateFields(*obj.Ident)

			return accessor, nil, obj.Pos, nil
		case obj.Number != nil:
			return &IntEvaluator{
				Value: *obj.Number,
			}, nil, obj.Pos, nil
		case obj.String != nil:
			return &StringEvaluator{
				Value: *obj.String,
			}, nil, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression, opts, state)
		default:
			return nil, nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("unknown primary '%s'", reflect.TypeOf(obj)))
		}
	case *ast.Array:
		if len(obj.Numbers) != 0 {
			ints := obj.Numbers
			sort.Ints(ints)
			return &IntArray{Values: ints}, nil, obj.Pos, nil
		} else if len(obj.Strings) != 0 {
			strs := obj.Strings
			sort.Strings(strs)
			return &StringArray{Values: strs}, nil, obj.Pos, nil
		} else if obj.Ident != nil {
			if state.macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}
		}
	}

	return nil, nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}
