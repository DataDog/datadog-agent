// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

// Field name
type Field = string

// FieldValueType represents the type of the value of a field
type FieldValueType int

// Field value types
const (
	ScalarValueType  FieldValueType = 1
	PatternValueType FieldValueType = 2
	BitmaskValueType FieldValueType = 4
)

// defines factor applied by specific operator
const (
	FunctionWeight       = 5
	InArrayWeight        = 10
	HandlerWeight        = 50
	PatternWeight        = 100
	InPatternArrayWeight = 1000
	IteratorWeight       = 2000
)

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType

	Regex *regexp.Regexp
}

// Opts are the options to be passed to the evaluator
type Opts struct {
	LegacyAttributes map[Field]Field
	Constants        map[string]interface{}
	Macros           map[MacroID]*Macro
}

// Evaluator is the interface of an evaluator
type Evaluator interface {
	Eval(ctx *Context) interface{}
}

// EvaluatorStringer implements the stringer in order to show the result of an evaluation. Should probably used only for logging
type EvaluatorStringer struct {
	Ctx       *Context
	Evaluator Evaluator
}

// BoolEvalFnc describe a eval function return a boolean
type BoolEvalFnc = func(ctx *Context) bool

// BoolEvaluator returns a bool as result of the evaluation
type BoolEvaluator struct {
	EvalFnc BoolEvalFnc
	Field   Field
	Value   bool
	Weight  int

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
	Weight  int

	isPartial bool
}

// Eval returns the result of the evaluation
func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc   func(ctx *Context) string
	Field     Field
	Value     string
	Weight    int
	IsPattern bool

	isPartial bool
}

// Eval returns the result of the evaluation
func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// StringArrayEvaluator returns an array of strings
type StringArrayEvaluator struct {
	EvalFnc   func(ctx *Context) []string
	Field     Field
	Values    []string
	Weight    int
	IsPattern bool

	isPartial bool
}

// Eval returns the result of the evaluation
func (s *StringArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IntArrayEvaluator returns an array of int
type IntArrayEvaluator struct {
	EvalFnc   func(ctx *Context) []int
	Field     Field
	Values    []int
	Weight    int
	IsPattern bool

	isPartial bool
}

// Eval returns the result of the evaluation
func (s *IntArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// BoolArrayEvaluator returns an array of bool
type BoolArrayEvaluator struct {
	EvalFnc func(ctx *Context) []bool
	Field   Field
	Values  []bool
	Weight  int

	isPartial bool
}

// Eval returns the result of the evaluation
func (b *BoolArrayEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(nil)
}

// PatternArray represents an array of pattern values
type PatternArray struct {
	Values  []string
	Regexps []*regexp.Regexp
}

func extractField(field string) (Field, Field, RegisterID, error) {
	var regID RegisterID

	re := regexp.MustCompile(`\[([^\]]*)\]`)
	ids := re.FindStringSubmatch(field)

	switch len(ids) {
	case 0:
		return field, "", "", nil
	case 2:
		regID = ids[1]
	default:
		return "", "", "", fmt.Errorf("wrong register format for fields: %s", field)
	}

	re = regexp.MustCompile(`(.+)\[[^\]]+\](.*)`)
	field, itField := re.ReplaceAllString(field, `$1$2`), re.ReplaceAllString(field, `$1`)

	return field, itField, regID, nil
}

func patternToRegexp(pattern string) (*regexp.Regexp, error) {
	// do not accept full wildcard value
	if matched, err := regexp.Match(`[a-zA-Z0-9\.]+`, []byte(pattern)); err != nil || !matched {
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

type ident struct {
	Pos   lexer.Position
	Ident *string
}

func identToEvaluator(obj *ident, opts *Opts, state *state) (interface{}, lexer.Position, error) {
	if accessor, ok := opts.Constants[*obj.Ident]; ok {
		return accessor, obj.Pos, nil
	}

	if state.macros != nil {
		if macro, ok := state.macros[*obj.Ident]; ok {
			return macro.Value, obj.Pos, nil
		}
	}

	field, itField, regID, err := extractField(*obj.Ident)
	if err != nil {
		return nil, obj.Pos, err
	}

	// transform extracted field to support legacy SECL attributes
	if opts.LegacyAttributes != nil {
		if newField, ok := opts.LegacyAttributes[field]; ok {
			field = newField
		}
		if newField, ok := opts.LegacyAttributes[field]; ok {
			itField = newField
		}
	}

	// extract iterator
	var iterator Iterator
	if itField != "" {
		if iterator, err = state.model.GetIterator(itField); err != nil {
			return nil, obj.Pos, err
		}
	} else {
		// detect whether a iterator is along the path
		var candidate string
		for _, node := range strings.Split(field, ".") {
			if candidate == "" {
				candidate = node
			} else {
				candidate = candidate + "." + node
			}

			iterator, err = state.model.GetIterator(candidate)
			if err == nil {
				break
			}
		}
	}

	if iterator != nil {
		// regID not specified generate one
		if regID == "" {
			regID = RandString(8)
		}

		if info, exists := state.registersInfo[regID]; exists {
			if info.field != itField {
				return nil, obj.Pos, NewRegisterMultipleFields(obj.Pos, regID, errors.New("used by multiple fields"))
			}

			info.subFields[field] = true
		} else {
			info = &registerInfo{
				field:    itField,
				iterator: iterator,
				subFields: map[Field]bool{
					field: true,
				},
			}
			state.registersInfo[regID] = info
		}
	}

	accessor, err := state.model.GetEvaluator(field, regID)
	if err != nil {
		return nil, obj.Pos, err
	}

	state.UpdateFields(field)

	fmt.Printf("PPPPPPPPPPPPP: %v\n", accessor)

	return accessor, obj.Pos, nil
}

func nodeToEvaluator(obj interface{}, opts *Opts, state *state) (interface{}, lexer.Position, error) {
	switch obj := obj.(type) {
	case *ast.BooleanExpression:
		return nodeToEvaluator(obj.Expression, opts, state)
	case *ast.Expression:
		cmp, pos, err := nodeToEvaluator(obj.Comparison, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			cmpBool, ok := cmp.(*BoolEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Bool)
			}

			next, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, pos, err
			}

			nextBool, ok := next.(*BoolEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Bool)
			}

			switch *obj.Op {
			case "||", "or":
				boolEvaluator, err := Or(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, obj.Pos, err
				}
				return boolEvaluator, obj.Pos, nil
			case "&&", "and":
				boolEvaluator, err := And(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, obj.Pos, err
				}
				return boolEvaluator, obj.Pos, nil
			}
			return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}
		return cmp, obj.Pos, nil
	case *ast.BitOperation:
		unary, pos, err := nodeToEvaluator(obj.Unary, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			bitInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
			}

			next, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, pos, err
			}

			nextInt, ok := next.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			switch *obj.Op {
			case "&":
				intEvaluator, err := IntAnd(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, pos, err
				}
				return intEvaluator, obj.Pos, nil
			case "|":
				IntEvaluator, err := IntOr(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, pos, err
				}
				return IntEvaluator, obj.Pos, nil
			case "^":
				IntEvaluator, err := IntXor(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, pos, err
				}
				return IntEvaluator, obj.Pos, nil
			}
			return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}
		return unary, obj.Pos, nil

	case *ast.Comparison:
		unary, pos, err := nodeToEvaluator(obj.BitOperation, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.ArrayComparison != nil {
			fmt.Printf("AAAAA\n")
			next, pos, err := nodeToEvaluator(obj.ArrayComparison, opts, state)
			if err != nil {
				return nil, pos, err
			}
			fmt.Printf("EEEE: %v\n", reflect.TypeOf(next))

			switch unary := unary.(type) {
			case *StringEvaluator:
				switch next.(type) {
				case *StringArrayEvaluator:
					boolEvaluator, err := StringArrayContains(unary, next.(*StringArrayEvaluator), *obj.ArrayComparison.Op == "notin", opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case *PatternArray:
					boolEvaluator, err := StringArrayMatches(unary, next.(*PatternArray), *obj.ArrayComparison.Op == "notin", opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			case *IntEvaluator:
				switch next.(type) {
				case *IntArrayEvaluator:
					boolEvaluator, err := IntArrayContains(unary, next.(*IntArrayEvaluator), *obj.ArrayComparison.Op == "notin", opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			default:
				return nil, pos, NewTypeError(pos, reflect.Array)
			}
		} else if obj.ScalarComparison != nil {
			next, pos, err := nodeToEvaluator(obj.ScalarComparison, opts, state)
			if err != nil {
				return nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				nextBool, ok := next.(*BoolEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Bool)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err := BoolNotEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "==":
					boolEvaluator, err := BoolEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					var eval *BoolEvaluator
					var err error

					if nextString.IsPattern {
						eval, err = StringMatches(unary, nextString, true, opts, state)
					} else {
						eval, err = StringNotEquals(unary, nextString, opts, state)
					}

					if err != nil {
						return nil, pos, err
					}
					return eval, pos, nil
				case "==":
					var eval *BoolEvaluator
					var err error

					if nextString.IsPattern {
						eval, err = StringMatches(unary, nextString, false, opts, state)
					} else {
						eval, err = StringEquals(unary, nextString, opts, state)
					}

					if err != nil {
						return nil, pos, err
					}
					return eval, pos, nil
				case "=~", "!~":
					eval, err := StringMatches(unary, nextString, *obj.ScalarComparison.Op == "!~", opts, state)
					if err != nil {
						return nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op, err)
					}
					return eval, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *IntEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					boolEvaluator, err := LesserThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "<=":
					boolEvaluator, err := LesserOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case ">":
					boolEvaluator, err := GreaterThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case ">=":
					boolEvaluator, err := GreaterOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "!=":
					boolEvaluator, err := IntNotEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "==":
					boolEvaluator, err := IntEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			}
		} else {
			return unary, pos, nil
		}

	case *ast.ArrayComparison:
		return nodeToEvaluator(obj.Array, opts, state)

	case *ast.ScalarComparison:
		return nodeToEvaluator(obj.Next, opts, state)

	case *ast.Unary:
		if obj.Op != nil {
			unary, pos, err := nodeToEvaluator(obj.Unary, opts, state)
			if err != nil {
				return nil, pos, err
			}

			switch *obj.Op {
			case "!", "not":
				unaryBool, ok := unary.(*BoolEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Bool)
				}

				return Not(unaryBool, opts, state), obj.Pos, nil
			case "-":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				return Minus(unaryInt, opts, state), pos, nil
			case "^":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				return IntNot(unaryInt, opts, state), pos, nil
			}
			return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}

		return nodeToEvaluator(obj.Primary, opts, state)
	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			return identToEvaluator(&ident{Pos: obj.Pos, Ident: obj.Ident}, opts, state)
		case obj.Number != nil:
			return &IntEvaluator{
				Value: *obj.Number,
			}, obj.Pos, nil
		case obj.String != nil:
			return &StringEvaluator{
				Value: *obj.String,
			}, obj.Pos, nil
		case obj.Pattern != nil:
			return &StringEvaluator{
				Value:     *obj.Pattern,
				IsPattern: true,
			}, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression, opts, state)
		default:
			return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("unknown primary '%s'", reflect.TypeOf(obj)))
		}
	case *ast.Array:
		if len(obj.Numbers) != 0 {
			ints := obj.Numbers
			sort.Ints(ints)

			return nil, obj.Pos, nil
		} else if len(obj.StringMembers) != 0 {
			var strs []string
			var hasPatterns bool

			for _, member := range obj.StringMembers {
				if member.String != nil {
					strs = append(strs, *member.String)
				} else {
					strs = append(strs, *member.Pattern)
					hasPatterns = true
				}
			}

			if hasPatterns {
				var regs []*regexp.Regexp
				var reg *regexp.Regexp
				var err error

				for _, member := range obj.StringMembers {
					if member.String != nil {
						// escape wildcard
						str := strings.ReplaceAll(*member.String, "*", "\\*")

						if reg, err = patternToRegexp(str); err != nil {
							return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("invalid pattern '%s': %s", *member.String, err))
						}
					} else {
						if reg, err = patternToRegexp(*member.Pattern); err != nil {
							return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("invalid pattern '%s': %s", *member.Pattern, err))
						}
					}
					regs = append(regs, reg)
				}
				return &PatternArray{Values: strs, Regexps: regs}, obj.Pos, nil
			}

			sort.Strings(strs)
			//return &StringArray{Values: strs}, obj.Pos, nil
			return nil, obj.Pos, nil
		} else if obj.Ident != nil {
			fmt.Printf("JJJJJJJJJJJJ\n")
			if state.macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, obj.Pos, nil
				}
			}

			// could be an iterator
			fmt.Printf("QQQQQQQQQQQQ\n")
			return identToEvaluator(&ident{Pos: obj.Pos, Ident: obj.Ident}, opts, state)
		}
	}

	return nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}
