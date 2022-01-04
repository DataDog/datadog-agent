// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Field name
type Field = string

// FieldValueType represents the type of the value of a field
type FieldValueType int

// Field value types
const (
	ScalarValueType   FieldValueType = 1 << 0
	PatternValueType  FieldValueType = 1 << 1
	RegexpValueType   FieldValueType = 1 << 2
	BitmaskValueType  FieldValueType = 1 << 3
	VariableValueType FieldValueType = 1 << 4
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

var (
	variableRegex = regexp.MustCompile(`\${[^}]*}`)
)

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType

	Regexp *regexp.Regexp
}

// VariableValue describes secl variable
type VariableValue struct {
	IntFnc    func(ctx *Context) int
	StringFnc func(ctx *Context) string
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

	isPartial bool

	fieldValues []FieldValue

	// cache
	scalars map[string]bool
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

	// transform extracted field to support legacy SECL fields
	if opts.LegacyFields != nil {
		if newField, ok := opts.LegacyFields[field]; ok {
			field = newField
		}
		if newField, ok := opts.LegacyFields[field]; ok {
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
		var se StringArrayEvaluator

		for _, member := range array.StringMembers {
			if member.Pattern != nil {
				reg, err := PatternToRegexp(*member.Pattern)
				if err != nil {
					return nil, array.Pos, NewError(array.Pos, fmt.Sprintf("invalid pattern `%s`: %s", *member.Pattern, err))
				}
				se.Values = append(se.Values, *member.Pattern)
				se.regexps = append(se.regexps, reg)
				se.fieldValues = append(se.fieldValues, FieldValue{
					Value:  *member.Pattern,
					Type:   PatternValueType,
					Regexp: reg,
				})
			} else if member.Regexp != nil {
				reg, err := regexp.Compile(*member.Regexp)
				if err != nil {
					return nil, array.Pos, NewError(array.Pos, fmt.Sprintf("invalid regexp `%s`: %s", *member.Regexp, err))
				}
				se.Values = append(se.Values, *member.Regexp)
				se.regexps = append(se.regexps, reg)

				se.fieldValues = append(se.fieldValues, FieldValue{
					Value:  *member.Regexp,
					Type:   RegexpValueType,
					Regexp: reg,
				})
			} else {
				if se.scalars == nil {
					se.scalars = make(map[string]bool)
				}
				se.Values = append(se.Values, *member.String)
				se.scalars[*member.String] = true
				se.fieldValues = append(se.fieldValues, FieldValue{
					Value: *member.String,
					Type:  ScalarValueType,
				})
			}
		}
		return &se, array.Pos, nil
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

func isVariableName(str string) (string, bool) {
	if strings.HasPrefix(str, "${") && strings.HasSuffix(str, "}") {
		return str[2 : len(str)-1], true
	}
	return "", false
}

func intEvaluatorFromVariable(varname string, pos lexer.Position, opts *Opts) (interface{}, lexer.Position, error) {
	value, exists := opts.Variables[varname]
	if !exists {
		return nil, pos, NewError(pos, fmt.Sprintf("variable '%s' doesn't exist", varname))
	}

	if value.IntFnc == nil {
		return nil, pos, NewError(pos, fmt.Sprintf("variable type not supported '%s'", varname))
	}
	return &IntEvaluator{
		EvalFnc: func(ctx *Context) int {
			return value.IntFnc(ctx)
		},
	}, pos, nil
}

func stringEvaluatorFromVariable(str string, pos lexer.Position, opts *Opts) (interface{}, lexer.Position, error) {
	var evaluators []*StringEvaluator

	doLoc := func(sub string) error {
		if varname, ok := isVariableName(sub); ok {
			value, exists := opts.Variables[varname]
			if !exists {
				return NewError(pos, fmt.Sprintf("variable '%s' doesn't exist", varname))
			}
			if value.IntFnc != nil {
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return strconv.FormatInt(int64(value.IntFnc(ctx)), 10)
					},
				})
			} else if value.StringFnc != nil {
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return value.StringFnc(ctx)
					},
				})
			} else {
				return NewError(pos, fmt.Sprintf("variable type not supported '%s'", varname))
			}
		} else {
			evaluators = append(evaluators, &StringEvaluator{Value: sub})
		}

		return nil
	}

	var last int
	for _, loc := range variableRegex.FindAllIndex([]byte(str), -1) {
		if loc[0] > 0 {
			if err := doLoc(str[last:loc[0]]); err != nil {
				return nil, pos, err
			}
		}
		if err := doLoc(str[loc[0]:loc[1]]); err != nil {
			return nil, pos, err
		}
		last = loc[1]
	}
	if last < len(str) {
		if err := doLoc(str[last:]); err != nil {
			return nil, pos, err
		}
	}

	return &StringEvaluator{
		valueType: VariableValueType,
		EvalFnc: func(ctx *Context) string {
			var result string
			for _, evaluator := range evaluators {
				if evaluator.EvalFnc != nil {
					result += evaluator.EvalFnc(ctx)
				} else {
					result += evaluator.Value
				}
			}
			return result
		},
	}, pos, nil
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
				switch nextInt := next.(type) {
				case *IntEvaluator:
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
		case obj.NumberVariable != nil:
			varname, ok := isVariableName(*obj.NumberVariable)
			if !ok {
				return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("internal variable error '%s'", varname))
			}

			return intEvaluatorFromVariable(varname, obj.Pos, opts)
		case obj.Duration != nil:
			return &IntEvaluator{
				Value:      *obj.Duration,
				isDuration: true,
			}, obj.Pos, nil
		case obj.String != nil:
			str := *obj.String

			// contains variables
			if len(variableRegex.FindAllIndex([]byte(str), -1)) > 0 {
				return stringEvaluatorFromVariable(str, obj.Pos, opts)
			}

			return &StringEvaluator{
				Value:     str,
				valueType: ScalarValueType,
			}, obj.Pos, nil
		case obj.Pattern != nil:
			reg, err := PatternToRegexp(*obj.Pattern)
			if err != nil {
				return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("invalid pattern '%s': %s", *obj.Pattern, err))
			}

			return &StringEvaluator{
				Value:     *obj.Pattern,
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
