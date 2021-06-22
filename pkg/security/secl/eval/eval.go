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
	ScalarValueType  FieldValueType = 1 << 0
	PatternValueType FieldValueType = 1 << 1
	RegexpValueType  FieldValueType = 1 << 2
	BitmaskValueType FieldValueType = 1 << 3
)

// defines factor applied by specific operator
const (
	FunctionWeight       = 5
	InArrayWeight        = 10
	HandlerWeight        = 50
	RegexpWeight         = 100
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
	IsPartial() bool
	GetField() string
	IsScalar() bool
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
	return b.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (b *BoolEvaluator) IsPartial() bool {
	return b.isPartial
}

// GetField returns field name used by this evaluator
func (b *BoolEvaluator) GetField() string {
	return b.Field
}

// IsScalar returns whether the evaluator is a scalar
func (b *BoolEvaluator) IsScalar() bool {
	return b.EvalFnc == nil
}

// IntEvaluator returns an int as result of the evaluation
type IntEvaluator struct {
	EvalFnc func(ctx *Context) int
	Field   Field
	Value   int
	Weight  int

	isPartial  bool
	isDuration bool
}

// Eval returns the result of the evaluation
func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (i *IntEvaluator) IsPartial() bool {
	return i.isPartial
}

// GetField returns field name used by this evaluator
func (i *IntEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc func(ctx *Context) string
	Field   Field
	Value   string
	Weight  int

	isRegexp  bool
	isPartial bool

	valueType FieldValueType

	// cache
	regexp *regexp.Regexp
}

// Eval returns the result of the evaluation
func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (s *StringEvaluator) IsPartial() bool {
	return s.isPartial
}

// GetField returns field name used by this evaluator
func (s *StringEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// StringArrayEvaluator returns an array of strings
type StringArrayEvaluator struct {
	EvalFnc func(ctx *Context) []string
	Field   Field
	Values  []string
	Weight  int

	isRegexp  bool
	isPartial bool

	valueTypes []FieldValueType

	// cache
	regexps []*regexp.Regexp
}

// Eval returns the result of the evaluation
func (s *StringArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (s *StringArrayEvaluator) IsPartial() bool {
	return s.isPartial
}

// GetField returns field name used by this evaluator
func (s *StringArrayEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringArrayEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// IntArrayEvaluator returns an array of int
type IntArrayEvaluator struct {
	EvalFnc func(ctx *Context) []int
	Field   Field
	Values  []int
	Weight  int

	isPartial bool
}

// Eval returns the result of the evaluation
func (i *IntArrayEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (i *IntArrayEvaluator) IsPartial() bool {
	return i.isPartial
}

// GetField returns field name used by this evaluator
func (i *IntArrayEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntArrayEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
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
	return b.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (b *BoolArrayEvaluator) IsPartial() bool {
	return b.isPartial
}

// GetField returns field name used by this evaluator
func (b *BoolArrayEvaluator) GetField() string {
	return b.Field
}

// IsScalar returns whether the evaluator is a scalar
func (b *BoolArrayEvaluator) IsScalar() bool {
	return b.EvalFnc == nil
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
		// Force "_" register for now.
		if regID != "" && regID != "_" {
			return nil, obj.Pos, NewRegisterNameNotAllowed(obj.Pos, regID, errors.New("only `_` is supported"))
		}

		// regID not specified or `_` generate one
		if regID == "" || regID == "_" {
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

	return accessor, obj.Pos, nil
}

func arrayToEvaluator(array *ast.Array, opts *Opts, state *state) (interface{}, lexer.Position, error) {
	if len(array.Numbers) != 0 {
		return &IntArrayEvaluator{
			Values: array.Numbers,
		}, array.Pos, nil
	} else if len(array.StringMembers) != 0 {
		var strs []string
		var valueTypes []FieldValueType

		isPlainStringArray := true
		for _, member := range array.StringMembers {
			if member.String != nil {
				strs = append(strs, *member.String)
				valueTypes = append(valueTypes, ScalarValueType)
			} else if member.Pattern != nil {
				strs = append(strs, *member.Pattern)
				valueTypes = append(valueTypes, PatternValueType)
				isPlainStringArray = false
			} else {
				strs = append(strs, *member.Regexp)
				valueTypes = append(valueTypes, RegexpValueType)
				isPlainStringArray = false
			}
		}

		if isPlainStringArray {
			return &StringArrayEvaluator{
				Values:     strs,
				valueTypes: valueTypes,
			}, array.Pos, nil
		}

		var reg *regexp.Regexp
		var regs []*regexp.Regexp
		var err error

		for _, member := range array.StringMembers {
			if member.String != nil {
				// escape wildcard
				str := strings.ReplaceAll(*member.String, "*", "\\*")

				if reg, err = patternToRegexp(str); err != nil {
					return nil, array.Pos, NewError(array.Pos, fmt.Sprintf("invalid pattern '%s': %s", *member.String, err))
				}
			} else if member.Pattern != nil {
				if reg, err = patternToRegexp(*member.Pattern); err != nil {
					return nil, array.Pos, NewError(array.Pos, fmt.Sprintf("invalid pattern '%s': %s", *member.Pattern, err))
				}
			} else {
				if reg, err = regexp.Compile(*member.Regexp); err != nil {
					return nil, array.Pos, NewError(array.Pos, fmt.Sprintf("invalid regexp '%s': %s", *member.Regexp, err))
				}
			}
			regs = append(regs, reg)
		}
		return &StringArrayEvaluator{
			Values:     strs,
			regexps:    regs,
			isRegexp:   true,
			valueTypes: valueTypes,
		}, array.Pos, nil
	} else if array.Ident != nil {
		if state.macros != nil {
			if macro, ok := state.macros[*array.Ident]; ok {
				return macro.Value, array.Pos, nil
			}
		}

		// could be an iterator
		return identToEvaluator(&ident{Pos: array.Pos, Ident: array.Ident}, opts, state)
	}

	return nil, array.Pos, NewError(array.Pos, "unknow array element type")
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
			next, pos, err := nodeToEvaluator(obj.ArrayComparison, opts, state)
			if err != nil {
				return nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				switch nextBool := next.(type) {
				case *BoolArrayEvaluator:
					boolEvaluator, err := ArrayBoolContains(unary, nextBool, opts, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, opts, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			case *StringEvaluator:
				switch nextString := next.(type) {
				case *StringArrayEvaluator:
					boolEvaluator, err := ArrayStringContains(unary, nextString, opts, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, opts, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			case *StringArrayEvaluator:
				switch nextStringArray := next.(type) {
				case *StringArrayEvaluator:
					boolEvaluator, err := ArrayStringMatches(unary, nextStringArray, opts, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, opts, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			case *IntEvaluator:
				switch nextInt := next.(type) {
				case *IntArrayEvaluator:
					boolEvaluator, err := ArrayIntEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, opts, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewTypeError(pos, reflect.Array)
				}
			case *IntArrayEvaluator:
				switch nextIntArray := next.(type) {
				case *IntArrayEvaluator:
					boolEvaluator, err := ArrayIntMatches(unary, nextIntArray, opts, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, opts, state), obj.Pos, nil
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
					boolEvaluator, err := BoolEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "==":
					boolEvaluator, err := BoolEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *BoolArrayEvaluator:
				nextBool, ok := next.(*BoolEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Bool)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err := ArrayBoolEquals(nextBool, unary, opts, state)
					if err != nil {
						return nil, pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "==":
					boolEvaluator, err := ArrayBoolEquals(nextBool, unary, opts, state)
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
					boolEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "!~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					if err := toPattern(nextString); err != nil {
						return nil, obj.Pos, NewError(obj.Pos, err.Error())
					}

					boolEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "==":
					boolEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "=~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					if err := toPattern(nextString); err != nil {
						return nil, obj.Pos, NewError(obj.Pos, err.Error())
					}

					boolEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringArrayEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err := ArrayStringContains(nextString, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "==":
					boolEvaluator, err := ArrayStringContains(nextString, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "!~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					if err := toPattern(nextString); err != nil {
						return nil, obj.Pos, NewError(obj.Pos, err.Error())
					}

					boolEvaluator, err := ArrayStringContains(nextString, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "=~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					if err := toPattern(nextString); err != nil {
						return nil, obj.Pos, NewError(obj.Pos, err.Error())
					}

					boolEvaluator, err := ArrayStringContains(nextString, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
			case *IntEvaluator:
				switch next.(type) {
				case *IntEvaluator:
					nextInt := next.(*IntEvaluator)

					if nextInt.isDuration {
						switch *obj.ScalarComparison.Op {
						case "<":
							boolEvaluator, err := DurationLesserThan(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case "<=":
							boolEvaluator, err := DurationLesserOrEqualThan(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case ">":
							boolEvaluator, err := DurationGreaterThan(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case ">=":
							boolEvaluator, err := DurationGreaterOrEqualThan(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						}
					} else {
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
							boolEvaluator, err := IntEquals(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}

							return Not(boolEvaluator, opts, state), obj.Pos, nil
						case "==":
							boolEvaluator, err := IntEquals(unary, nextInt, opts, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						default:
							return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
						}
					}
				case *IntArrayEvaluator:
					nextIntArray := next.(*IntArrayEvaluator)

					switch *obj.ScalarComparison.Op {
					case "<":
						boolEvaluator, err := ArrayIntLesserThan(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "<=":
						boolEvaluator, err := ArrayIntLesserOrEqualThan(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">":
						boolEvaluator, err := ArrayIntGreaterThan(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">=":
						boolEvaluator, err := ArrayIntGreaterOrEqualThan(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "!=":
						boolEvaluator, err := ArrayIntEquals(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return Not(boolEvaluator, opts, state), obj.Pos, nil
					case "==":
						boolEvaluator, err := ArrayIntEquals(unary, nextIntArray, opts, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					default:
						return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
					}
				}
				return nil, pos, NewTypeError(pos, reflect.Int)
			case *IntArrayEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					boolEvaluator, err := ArrayIntGreaterThan(nextInt, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "<=":
					boolEvaluator, err := ArrayIntGreaterOrEqualThan(nextInt, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case ">":
					boolEvaluator, err := ArrayIntLesserThan(nextInt, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case ">=":
					boolEvaluator, err := ArrayIntLesserOrEqualThan(nextInt, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "!=":
					boolEvaluator, err := ArrayIntEquals(nextInt, unary, opts, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, opts, state), obj.Pos, nil
				case "==":
					boolEvaluator, err := ArrayIntEquals(nextInt, unary, opts, state)
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
		case obj.Duration != nil:
			return &IntEvaluator{
				Value:      *obj.Duration,
				isDuration: true,
			}, obj.Pos, nil
		case obj.String != nil:
			return &StringEvaluator{
				Value:     *obj.String,
				valueType: ScalarValueType,
			}, obj.Pos, nil
		case obj.Pattern != nil:
			reg, err := patternToRegexp(*obj.Pattern)
			if err != nil {
				return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("invalid pattern '%s': %s", *obj.Pattern, err))
			}

			return &StringEvaluator{
				Value:     *obj.Pattern,
				isRegexp:  true,
				regexp:    reg,
				valueType: PatternValueType,
			}, obj.Pos, nil
		case obj.Regexp != nil:
			reg, err := regexp.Compile(*obj.Regexp)
			if err != nil {
				return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("invalid regexp '%s': %s", *obj.Regexp, err))
			}

			return &StringEvaluator{
				Value:     *obj.Regexp,
				isRegexp:  true,
				regexp:    reg,
				valueType: RegexpValueType,
			}, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression, opts, state)
		default:
			return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("unknown primary '%s'", reflect.TypeOf(obj)))
		}
	case *ast.Array:
		return arrayToEvaluator(obj, opts, state)
	}

	return nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}
