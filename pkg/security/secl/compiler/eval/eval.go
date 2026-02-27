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
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"

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

var (
	arraySubscriptFindRE    = regexp.MustCompile(`\[([^\]]*)\]`)
	arraySubscriptReplaceRE = regexp.MustCompile(`(.+)\[[^\]]+\](.*)`)
	arrayIndexRE            = regexp.MustCompile(`^(.+)\[(\d+)\]$`)
)

// extractField extracts field information and handles different subscript notations
// Returns:
//   - resField: the resolved field name (base field without subscript)
//   - itField: the iterator field name (for iterator syntax only)
//   - regID: the register ID for iterator syntax (e.g., "x" in field[x])
//   - arrayIndex: the numeric index for array access (e.g., 0 in field[0])
//   - isArrayAccess: true if this is a numeric array index access (field[0])
//   - error: any parsing error
//
// Supported syntaxes:
//   - field[0], field[1], etc. → numeric array index access
//   - field[x], field[y], etc. → iterator with register variable
//   - field → plain field access
func extractField(field string) (Field, Field, RegisterID, int, bool, error) {
	// First check if this is a numeric array index access like field[0]
	if matches := arrayIndexRE.FindStringSubmatch(field); len(matches) == 3 {
		baseField := matches[1]
		index, err := strconv.Atoi(matches[2])
		if err != nil {
			return "", "", "", 0, false, fmt.Errorf("invalid array index in field: %s", field)
		}
		// Return base field with index information
		return baseField, baseField, "", index, true, nil
	}

	// Otherwise, check for iterator register syntax like field[x]
	var regID RegisterID
	ids := arraySubscriptFindRE.FindStringSubmatch(field)

	switch len(ids) {
	case 0:
		return field, "", "", 0, false, nil
	case 2:
		regID = ids[1]
	default:
		return "", "", "", 0, false, fmt.Errorf("wrong register format for fields: %s", field)
	}

	resField := arraySubscriptReplaceRE.ReplaceAllString(field, `$1$2`)
	itField := arraySubscriptReplaceRE.ReplaceAllString(field, `$1`)

	return resField, itField, regID, 0, false, nil
}

// ExtractArrayIndexAccess extracts array index information from a field like "field[0]"
// Returns: baseField, index, isArrayAccess, error
func ExtractArrayIndexAccess(field string) (string, int, bool, error) {
	if matches := arrayIndexRE.FindStringSubmatch(field); len(matches) == 3 {
		baseField := matches[1]
		index, err := strconv.Atoi(matches[2])
		if err != nil {
			return "", 0, false, fmt.Errorf("invalid array index in field: %s", field)
		}
		return baseField, index, true, nil
	}
	return field, 0, false, nil
}

// WrapEvaluatorWithArrayIndex wraps an array evaluator to return a specific index
func WrapEvaluatorWithArrayIndex(evaluator interface{}, index int, field Field) (Evaluator, error) {
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

type ident struct {
	Pos   lexer.Position
	Ident *string
}

func identToEvaluator(obj *ident, opts *Opts, state *State) (interface{}, lexer.Position, error) {
	if accessor, ok := opts.Constants[*obj.Ident]; ok {
		return accessor, obj.Pos, nil
	}

	if state.macros != nil {
		if macro, ok := state.macros.GetMacroEvaluator(*obj.Ident); ok {
			return macro.Value, obj.Pos, nil
		}
	}

	field, itField, regID, arrayIndex, isArrayAccess, err := extractField(*obj.Ident)
	if err != nil {
		return nil, obj.Pos, err
	}

	// transform extracted field to support legacy SECL fields
	if opts.LegacyFields != nil {
		if newField, ok := opts.LegacyFields[field]; ok {
			field = newField
		}
	}

	evaluator, err := state.model.GetEvaluator(field, regID, obj.Pos.Offset)
	if err != nil {
		return nil, obj.Pos, err
	}

	state.UpdateFields(field)

	// Handle numeric array index access (e.g., field[0])
	if isArrayAccess {
		wrappedEvaluator, err := WrapEvaluatorWithArrayIndex(evaluator, arrayIndex, field)
		if err != nil {
			return nil, obj.Pos, NewError(obj.Pos, "failed to create array index accessor: %v", err)
		}
		return wrappedEvaluator, obj.Pos, nil
	}

	if regID != "" {
		// avoid wildcard register for the moment
		if regID == "_" {
			return nil, obj.Pos, NewError(obj.Pos, "`_` can't be used as a iterator variable name")
		}

		// avoid using the same register on two different fields
		if slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID && r.Field != itField
		}) {
			return nil, obj.Pos, NewError(obj.Pos, "iterator variable used by different fields '%s'", regID)
		}

		if !slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID
		}) {
			state.registers = append(state.registers, Register{ID: regID, Field: itField})
		}
	}

	return evaluator, obj.Pos, nil
}

func arrayToEvaluator(array *ast.Array, opts *Opts, state *State) (interface{}, lexer.Position, error) {
	if len(array.Numbers) != 0 {
		var evaluator IntArrayEvaluator
		evaluator.AppendValues(array.Numbers...)
		return &evaluator, array.Pos, nil
	} else if len(array.StringMembers) != 0 {
		var evaluator StringValuesEvaluator
		evaluator.AppendMembers(array.StringMembers...)
		return &evaluator, array.Pos, nil
	} else if array.Ident != nil {
		if state.macros != nil {
			if macro, ok := state.macros.GetMacroEvaluator(*array.Ident); ok {
				return macro.Value, array.Pos, nil
			}
		}

		// could be an iterator
		return identToEvaluator(&ident{Pos: array.Pos, Ident: array.Ident}, opts, state)
	} else if len(array.Idents) != 0 {
		// Only "Constants" idents are supported, and only string, int and boolean constants are expected.
		// Determine the type with the first ident
		switch reflect.TypeOf(opts.Constants[array.Idents[0]]) {
		case reflect.TypeOf(&IntEvaluator{}):
			var evaluator IntArrayEvaluator
			for _, item := range array.Idents {
				itemEval, ok := opts.Constants[item].(*IntEvaluator)
				if !ok {
					return nil, array.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type int", item)
				}
				evaluator.AppendValues(itemEval.Value)
			}
			return &evaluator, array.Pos, nil
		case reflect.TypeOf(&StringEvaluator{}):
			var evaluator StringValuesEvaluator
			for _, item := range array.Idents {
				itemEval, ok := opts.Constants[item].(*StringEvaluator)
				if !ok {
					return nil, array.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type string", item)
				}
				evaluator.AppendMembers(ast.StringMember{String: &itemEval.Value})
			}
			return &evaluator, array.Pos, nil
		case reflect.TypeOf(&BoolEvaluator{}):
			var evaluator BoolArrayEvaluator
			for _, item := range array.Idents {
				itemEval, ok := opts.Constants[item].(*BoolEvaluator)
				if !ok {
					return nil, array.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type bool", item)
				}
				evaluator.AppendValues(itemEval.Value)
			}
			return &evaluator, array.Pos, nil
		}
		return nil, array.Pos, fmt.Errorf("array of unsupported identifiers (ident type: `%s`)", reflect.TypeOf(opts.Constants[array.Idents[0]]))
	} else if array.Variable != nil {
		varName, ok := isVariableName(*array.Variable)
		if !ok {
			return nil, array.Pos, NewError(array.Pos, "invalid variable name '%s'", *array.Variable)
		}
		return evaluatorFromVariable(varName, array.Pos, opts)
	} else if array.FieldReference != nil {
		fieldName, ok := isFieldReferenceName(*array.FieldReference)
		if !ok {
			return nil, array.Pos, NewError(array.Pos, "invalid field reference '%s'", *array.FieldReference)
		}
		return evaluatorFromFieldReference(fieldName, array.Pos, state)
	} else if array.CIDR != nil {
		var values CIDRValues
		if err := values.AppendCIDR(*array.CIDR); err != nil {
			return nil, array.Pos, NewError(array.Pos, "invalid CIDR '%s'", *array.CIDR)
		}

		evaluator := &CIDRValuesEvaluator{
			Value:     values,
			ValueType: IPNetValueType,
			Offset:    array.Pos.Offset,
		}
		return evaluator, array.Pos, nil
	} else if len(array.CIDRMembers) != 0 {
		var values CIDRValues
		for _, member := range array.CIDRMembers {
			if member.CIDR != nil {
				if err := values.AppendCIDR(*member.CIDR); err != nil {
					return nil, array.Pos, NewError(array.Pos, "invalid CIDR '%s'", *member.CIDR)
				}
			} else if member.IP != nil {
				if err := values.AppendIP(*member.IP); err != nil {
					return nil, array.Pos, NewError(array.Pos, "invalid IP '%s'", *member.IP)
				}
			}
		}

		evaluator := &CIDRValuesEvaluator{
			Value:     values,
			ValueType: IPNetValueType,
			Offset:    array.Pos.Offset,
		}
		return evaluator, array.Pos, nil
	}

	return nil, array.Pos, NewError(array.Pos, "unknown array element type")
}

func isVariableName(str string) (string, bool) {
	if strings.HasPrefix(str, "${") && strings.HasSuffix(str, "}") {
		return str[2 : len(str)-1], true
	}
	return "", false
}

// isFieldReferenceName checks if a string is a valid field reference (%{...})
func isFieldReferenceName(str string) (string, bool) {
	if strings.HasPrefix(str, "%{") && strings.HasSuffix(str, "}") {
		return strings.TrimSuffix(strings.TrimPrefix(str, "%{"), "}"), true
	}
	return "", false
}

func evaluatorFromLengthHanlder(fieldname string, pos lexer.Position, state *State) (interface{}, lexer.Position, error) {
	evaluator, err := state.model.GetEvaluator(fieldname, "", 0)
	if err != nil {
		return nil, pos, NewError(pos, "field '%s' doesn't exist", fieldname)
	}

	// Return length evaluator based on field type
	switch fieldEval := evaluator.(type) {
	case *StringArrayEvaluator:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				v := fieldEval.Eval(ctx)
				return len(v.([]string))
			},
		}, pos, nil
	case *StringEvaluator:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				v := fieldEval.Eval(ctx)
				return len(v.(string))
			},
		}, pos, nil
	case *IntArrayEvaluator:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				v := fieldEval.Eval(ctx)
				return len(v.([]int))
			},
		}, pos, nil
	default:
		return nil, pos, NewError(pos, "'length' cannot be used on field '%s'", fieldname)
	}
}

// evaluatorFromFieldReference resolves a field reference (%{field})
// This ONLY checks fields, never variables - providing explicit field access syntax
func evaluatorFromFieldReference(fieldname string, pos lexer.Position, state *State) (interface{}, lexer.Position, error) {
	// Handle .length suffix for fields
	if before, ok := strings.CutSuffix(fieldname, ".length"); ok {
		return evaluatorFromLengthHanlder(before, pos, state)
	}

	if before, ok := strings.CutSuffix(fieldname, ".root_domain"); ok {
		return evaluatorFromRootDomainHandler(before, pos, state)
	}

	// Only try to resolve as a field (no variable lookup)
	evaluator, err := state.model.GetEvaluator(fieldname, "", 0)
	if err != nil {
		return nil, pos, NewError(pos, "field '%s' doesn't exist", fieldname)
	}

	return evaluator, pos, nil
}

func evaluatorFromVariable(varname string, pos lexer.Position, opts *Opts) (interface{}, lexer.Position, error) {
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
	// Use %{field} syntax for fields
	return nil, pos, NewError(pos, "variable '%s' doesn't exist", varname)
}

func stringEvaluatorFromVariable(str string, pos lexer.Position, opts *Opts, state *State) (interface{}, lexer.Position, error) {
	var evaluators []*StringEvaluator

	doLoc := func(sub string) error {
		if varname, ok := isVariableName(sub); ok {
			evaluator, pos, err := evaluatorFromVariable(varname, pos, opts)
			if err != nil {
				return err
			}

			switch evaluator := evaluator.(type) {
			case *StringArrayEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return strings.Join(evaluator.EvalFnc(ctx), ",")
					}})
			case *IntArrayEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						var builder strings.Builder
						for i, number := range evaluator.EvalFnc(ctx) {
							if i != 0 {
								builder.WriteString(",")
							}
							builder.WriteString(strconv.FormatInt(int64(number), 10))
						}
						return builder.String()
					}})
			case *StringEvaluator:
				evaluators = append(evaluators, evaluator)
			case *IntEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return strconv.FormatInt(int64(evaluator.EvalFnc(ctx)), 10)
					}})
			default:
				return NewError(pos, "variable type not supported '%s'", varname)
			}
		} else if fieldname, ok := isFieldReferenceName(sub); ok {
			evaluator, pos, err := evaluatorFromFieldReference(fieldname, pos, state)
			if err != nil {
				return err
			}

			switch evaluator := evaluator.(type) {
			case *StringArrayEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return strings.Join(evaluator.EvalFnc(ctx), ",")
					}})
			case *IntArrayEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						var builder strings.Builder
						for i, number := range evaluator.EvalFnc(ctx) {
							if i != 0 {
								builder.WriteString(",")
							}
							builder.WriteString(strconv.FormatInt(int64(number), 10))
						}
						return builder.String()
					}})
			case *StringEvaluator:
				evaluators = append(evaluators, evaluator)
			case *IntEvaluator:
				evaluators = append(evaluators, &StringEvaluator{
					EvalFnc: func(ctx *Context) string {
						return strconv.FormatInt(int64(evaluator.EvalFnc(ctx)), 10)
					}})
			default:
				return NewError(pos, "field type not supported '%s'", fieldname)
			}
		} else {
			evaluators = append(evaluators, &StringEvaluator{Value: sub})
		}

		return nil
	}

	// Find all ${...} and %{...} matches and process them in order
	varMatches := variableRegex.FindAllIndex([]byte(str), -1)
	fieldMatches := fieldReferenceRegex.FindAllIndex([]byte(str), -1)

	// Merge and sort all matches by position
	allMatches := append(varMatches, fieldMatches...)
	slices.SortFunc(allMatches, func(a, b []int) int {
		return a[0] - b[0]
	})

	var last int
	for _, loc := range allMatches {
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
		Value:     str,
		ValueType: VariableValueType,
		EvalFnc: func(ctx *Context) string {
			var builder strings.Builder
			for _, evaluator := range evaluators {
				if evaluator.EvalFnc != nil {
					builder.WriteString(evaluator.EvalFnc(ctx))
				} else {
					builder.WriteString(evaluator.Value)
				}
			}
			return builder.String()
		},
	}, pos, nil
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

// NodeToEvaluator converts an AST expression to an evaluator
func NodeToEvaluator(obj interface{}, opts *Opts, state *State) (interface{}, lexer.Position, error) {
	return nodeToEvaluator(obj, opts, state)
}

func nodeToEvaluator(obj interface{}, opts *Opts, state *State) (interface{}, lexer.Position, error) {
	var err error
	var boolEvaluator *BoolEvaluator
	var pos lexer.Position
	var cmp, unary, next interface{}

	switch obj := obj.(type) {
	case *ast.BooleanExpression:
		return nodeToEvaluator(obj.Expression, opts, state)
	case *ast.Expression:
		cmp, pos, err = nodeToEvaluator(obj.Comparison, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			cmpBool, ok := cmp.(*BoolEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Bool)
			}

			next, pos, err = nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, pos, err
			}

			nextBool, ok := next.(*BoolEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Bool)
			}

			switch *obj.Op {
			case "||", "or":
				boolEvaluator, err = Or(cmpBool, nextBool, state)
				if err != nil {
					return nil, obj.Pos, err
				}
				return boolEvaluator, obj.Pos, nil
			case "&&", "and":
				boolEvaluator, err = And(cmpBool, nextBool, state)
				if err != nil {
					return nil, obj.Pos, err
				}
				return boolEvaluator, obj.Pos, nil
			}
			return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}

		if cmpBool, ok := cmp.(*BoolEvaluator); ok {
			cmp, err = Unary(cmpBool, state)
			if err != nil {
				return nil, obj.Pos, err
			}
		}

		return cmp, obj.Pos, nil
	case *ast.BitOperation:
		unary, pos, err = nodeToEvaluator(obj.Unary, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			bitInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
			}

			next, pos, err = nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, pos, err
			}

			nextInt, ok := next.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			switch *obj.Op {
			case "&":
				intEvaluator, err := IntAnd(bitInt, nextInt, state)
				if err != nil {
					return nil, pos, err
				}
				return intEvaluator, obj.Pos, nil
			case "|":
				IntEvaluator, err := IntOr(bitInt, nextInt, state)
				if err != nil {
					return nil, pos, err
				}
				return IntEvaluator, obj.Pos, nil
			case "^":
				IntEvaluator, err := IntXor(bitInt, nextInt, state)
				if err != nil {
					return nil, pos, err
				}
				return IntEvaluator, obj.Pos, nil
			}
			return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}
		return unary, obj.Pos, nil

	case *ast.ArithmeticOperation:
		// Process the first operand
		first, pos, err := nodeToEvaluator(obj.First, opts, state)
		if err != nil {
			return nil, pos, err
		}

		// If it's just one element (is a bitoperation: maybe a string, an int ....)
		if len(obj.Rest) == 0 {
			return first, obj.Pos, nil
		}

		// Else it's an operation, so it must be an int
		currInt, ok := first.(*IntEvaluator)
		if !ok {
			return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
		}

		// Process the remaining operations and operands
		for _, arithElem := range obj.Rest {
			// Handle the operand
			operand, pos, err := nodeToEvaluator(arithElem.Operand, opts, state)
			if err != nil {
				return nil, pos, err
			}
			operandInt, ok := operand.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			// Perform the operation on the current and next operands
			switch arithElem.Op {
			case "+":
				currInt, err = IntPlus(currInt, operandInt, state)
				if err != nil {
					return nil, pos, err
				}

			case "-":
				currInt, err = IntMinus(currInt, operandInt, state)
				if err != nil {
					return nil, pos, err
				}
			}
		}

		// Return the final result after processing all operations and operands
		currInt.isFromArithmeticOperation = true
		return currInt, obj.Pos, nil

	case *ast.Comparison:
		unary, pos, err = nodeToEvaluator(obj.ArithmeticOperation, opts, state)
		if err != nil {
			return nil, pos, err
		}

		if obj.ArrayComparison != nil {
			next, pos, err = nodeToEvaluator(obj.ArrayComparison, opts, state)
			if err != nil {
				return nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				switch nextBool := next.(type) {
				case *BoolArrayEvaluator:
					boolEvaluator, err = ArrayBoolContains(unary, nextBool, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Bool)
				}
			case *StringEvaluator:
				switch nextString := next.(type) {
				case *StringArrayEvaluator:
					boolEvaluator, err = StringArrayContainsWrapper(unary, nextString, state)
					if err != nil {
						return nil, pos, err
					}
				case *StringValuesEvaluator:
					boolEvaluator, err = stringValuesContainsWrapper(unary, nextString, state)
					if err != nil {
						return nil, pos, err
					}
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
				}
				if *obj.ArrayComparison.Op == "notin" {
					return Not(boolEvaluator, state), obj.Pos, nil
				}
				return boolEvaluator, obj.Pos, nil
			case *StringValuesEvaluator:
				switch nextStringArray := next.(type) {
				case *StringArrayEvaluator:
					boolEvaluator, err = stringArrayMatchesWrapper(nextStringArray, unary, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
				}
			case *StringArrayEvaluator:
				switch nextStringArray := next.(type) {
				case *StringValuesEvaluator:
					boolEvaluator, err = stringArrayMatchesWrapper(unary, nextStringArray, state)
					if err != nil {
						return nil, pos, err
					}
				case *StringArrayEvaluator:
					boolEvaluator, err = StringArrayMatchesStringArray(unary, nextStringArray, state)
					if err != nil {
						return nil, pos, err
					}
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
				}
				if *obj.ArrayComparison.Op == "notin" {
					return Not(boolEvaluator, state), obj.Pos, nil
				}
				return boolEvaluator, obj.Pos, nil
			case *IntEvaluator:
				switch nextInt := next.(type) {
				case *IntArrayEvaluator:
					boolEvaluator, err = IntArrayEquals(unary, nextInt, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Int)
				}
			case *IntArrayEvaluator:
				switch nextIntArray := next.(type) {
				case *IntArrayEvaluator:
					boolEvaluator, err = IntArrayMatches(unary, nextIntArray, state)
					if err != nil {
						return nil, pos, err
					}
					if *obj.ArrayComparison.Op == "notin" {
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return boolEvaluator, obj.Pos, nil
				default:
					return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Int)
				}
			case *CIDREvaluator:
				switch nextCIDR := next.(type) {
				case *CIDREvaluator:
					nextIP, ok := next.(*CIDREvaluator)
					if !ok {
						return nil, pos, NewTypeError(pos, reflect.TypeOf(CIDREvaluator{}).Kind())
					}

					boolEvaluator, err = CIDREquals(unary, nextIP, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					switch *obj.ArrayComparison.Op {
					case "in", "allin":
						return boolEvaluator, obj.Pos, nil
					case "notin":
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				case *CIDRValuesEvaluator:
					switch *obj.ArrayComparison.Op {
					case "in", "allin":
						boolEvaluator, err = CIDRValuesContains(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "notin":
						boolEvaluator, err = CIDRValuesContains(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				case *CIDRArrayEvaluator:
					switch *obj.ArrayComparison.Op {
					case "in", "allin":
						boolEvaluator, err = CIDRArrayContains(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "notin":
						boolEvaluator, err = CIDRArrayContains(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				default:
					return nil, pos, NewCIDRTypeError(pos, reflect.Array, next)
				}
			case *CIDRArrayEvaluator:
				switch nextCIDR := next.(type) {
				case *CIDRValuesEvaluator:
					switch *obj.ArrayComparison.Op {
					case "in":
						boolEvaluator, err = CIDRArrayMatches(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "allin":
						boolEvaluator, err = CIDRArrayMatchesAll(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "notin":
						boolEvaluator, err = CIDRArrayMatches(unary, nextCIDR, state)
						if err != nil {
							return nil, pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				default:
					return nil, pos, NewCIDRTypeError(pos, reflect.Array, next)
				}
			case *CIDRValuesEvaluator:
				switch nextCIDR := next.(type) {
				case *CIDREvaluator:
					switch *obj.ArrayComparison.Op {
					case "in", "allin":
						boolEvaluator, err = CIDRValuesContains(nextCIDR, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "notin":
						boolEvaluator, err = CIDRValuesContains(nextCIDR, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				case *CIDRArrayEvaluator:
					switch *obj.ArrayComparison.Op {
					case "allin":
						boolEvaluator, err = CIDRArrayMatchesAll(nextCIDR, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "in":
						boolEvaluator, err = CIDRArrayMatches(nextCIDR, unary, state)
						if err != nil {
							return nil, pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "notin":
						boolEvaluator, err = CIDRArrayMatches(nextCIDR, unary, state)
						if err != nil {
							return nil, pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)
				default:
					return nil, pos, NewCIDRTypeError(pos, reflect.Array, next)
				}
			default:
				return nil, pos, NewTypeError(pos, reflect.Array)
			}
		} else if obj.ScalarComparison != nil {
			next, pos, err = nodeToEvaluator(obj.ScalarComparison, opts, state)
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
					boolEvaluator, err = BoolEquals(unary, nextBool, state)
					if err != nil {
						return nil, pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "==":
					boolEvaluator, err = BoolEquals(unary, nextBool, state)
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
					boolEvaluator, err = BoolArrayEquals(nextBool, unary, state)
					if err != nil {
						return nil, pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "==":
					boolEvaluator, err = BoolArrayEquals(nextBool, unary, state)
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
					boolEvaluator, err = StringEqualsWrapper(unary, nextString, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "!~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					// force pattern if needed
					if nextString.ValueType == ScalarValueType {
						nextString.ValueType = PatternValueType
					}

					boolEvaluator, err = StringEqualsWrapper(unary, nextString, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "==":
					boolEvaluator, err = StringEqualsWrapper(unary, nextString, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "=~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					// force pattern if needed
					if nextString.ValueType == ScalarValueType {
						nextString.ValueType = PatternValueType
					}

					boolEvaluator, err = StringEqualsWrapper(unary, nextString, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *CIDREvaluator:
				switch nextIP := next.(type) {
				case *CIDREvaluator:
					switch *obj.ScalarComparison.Op {
					case "!=":
						boolEvaluator, err = CIDREquals(unary, nextIP, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					case "==":
						boolEvaluator, err = CIDREquals(unary, nextIP, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
				}

			case *CIDRArrayEvaluator:
				cidrEvaluator, ok := next.(*CIDREvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err = CIDRArrayMatchesCIDREvaluator(unary, cidrEvaluator, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "==":
					boolEvaluator, err = CIDRArrayMatchesCIDREvaluator(unary, cidrEvaluator, state)
					if err != nil {
						return nil, pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}

				return nil, pos, NewOpUnknownError(obj.Pos, *obj.ArrayComparison.Op)

			case *StringArrayEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					boolEvaluator, err = StringArrayContainsWrapper(nextString, unary, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "==":
					boolEvaluator, err = StringArrayContainsWrapper(nextString, unary, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				case "!~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					// force pattern if needed
					if nextString.ValueType == ScalarValueType {
						nextString.ValueType = PatternValueType
					}

					boolEvaluator, err = StringArrayContainsWrapper(nextString, unary, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return Not(boolEvaluator, state), obj.Pos, nil
				case "=~":
					if nextString.EvalFnc != nil {
						return nil, obj.Pos, &ErrNonStaticPattern{Field: nextString.Field}
					}

					// force pattern if needed
					if nextString.ValueType == ScalarValueType {
						nextString.ValueType = PatternValueType
					}

					boolEvaluator, err = StringArrayContainsWrapper(nextString, unary, state)
					if err != nil {
						return nil, obj.Pos, err
					}
					return boolEvaluator, obj.Pos, nil
				}
			case *IntEvaluator:
				switch nextInt := next.(type) {
				case *IntEvaluator:
					if nextInt.isDuration {
						if unary.isFromArithmeticOperation {
							switch *obj.ScalarComparison.Op {
							case "<":
								boolEvaluator, err = DurationLesserThanArithmeticOperation(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case "<=":
								boolEvaluator, err = DurationLesserOrEqualThanArithmeticOperation(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case ">":
								boolEvaluator, err = DurationGreaterThanArithmeticOperation(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case ">=":
								boolEvaluator, err = DurationGreaterOrEqualThanArithmeticOperation(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case "==":
								boolEvaluator, err = DurationEqualArithmeticOperation(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							}

						} else {
							switch *obj.ScalarComparison.Op {
							case "<":
								boolEvaluator, err = DurationLesserThan(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case "<=":
								boolEvaluator, err = DurationLesserOrEqualThan(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case ">":
								boolEvaluator, err = DurationGreaterThan(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case ">=":
								boolEvaluator, err = DurationGreaterOrEqualThan(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							case "==":
								boolEvaluator, err = DurationEqual(unary, nextInt, state)
								if err != nil {
									return nil, obj.Pos, err
								}
								return boolEvaluator, obj.Pos, nil
							}
						}
					} else {
						switch *obj.ScalarComparison.Op {
						case "<":
							boolEvaluator, err = LesserThan(unary, nextInt, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case "<=":
							boolEvaluator, err = LesserOrEqualThan(unary, nextInt, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case ">":
							boolEvaluator, err = GreaterThan(unary, nextInt, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case ">=":
							boolEvaluator, err = GreaterOrEqualThan(unary, nextInt, state)
							if err != nil {
								return nil, obj.Pos, err
							}
							return boolEvaluator, obj.Pos, nil
						case "!=":
							boolEvaluator, err = IntEquals(unary, nextInt, state)
							if err != nil {
								return nil, obj.Pos, err
							}

							return Not(boolEvaluator, state), obj.Pos, nil
						case "==":
							boolEvaluator, err = IntEquals(unary, nextInt, state)
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
						boolEvaluator, err = IntArrayLesserThan(unary, nextIntArray, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "<=":
						boolEvaluator, err = IntArrayLesserOrEqualThan(unary, nextIntArray, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">":
						boolEvaluator, err = IntArrayGreaterThan(unary, nextIntArray, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">=":
						boolEvaluator, err = IntArrayGreaterOrEqualThan(unary, nextIntArray, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "!=":
						boolEvaluator, err = IntArrayEquals(unary, nextIntArray, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					case "==":
						boolEvaluator, err = IntArrayEquals(unary, nextIntArray, state)
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

				if nextInt.isDuration {
					switch *obj.ScalarComparison.Op {
					case "<":
						boolEvaluator, err = DurationArrayLesserThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "<=":
						boolEvaluator, err = DurationArrayLesserOrEqualThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">":
						boolEvaluator, err = DurationArrayGreaterThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">=":
						boolEvaluator, err = DurationArrayGreaterOrEqualThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					}
				} else {
					switch *obj.ScalarComparison.Op {
					case "<":
						boolEvaluator, err = IntArrayGreaterThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "<=":
						boolEvaluator, err = IntArrayGreaterOrEqualThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">":
						boolEvaluator, err = IntArrayLesserThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case ">=":
						boolEvaluator, err = IntArrayLesserOrEqualThan(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					case "!=":
						boolEvaluator, err = IntArrayEquals(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return Not(boolEvaluator, state), obj.Pos, nil
					case "==":
						boolEvaluator, err = IntArrayEquals(nextInt, unary, state)
						if err != nil {
							return nil, obj.Pos, err
						}
						return boolEvaluator, obj.Pos, nil
					}
					return nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
				}
			}
		} else {
			return unary, pos, nil
		}

	case *ast.ArrayComparison:
		return nodeToEvaluator(obj.Array, opts, state)

	case *ast.ScalarComparison:
		return nodeToEvaluator(obj.Next, opts, state)

	case *ast.Unary:
		if obj.UnaryWithOp != nil {
			return nodeToEvaluator(obj.UnaryWithOp, opts, state)
		}

		return nodeToEvaluator(obj.Primary, opts, state)

	case *ast.UnaryWithOp:
		unary, pos, err = nodeToEvaluator(obj.Unary, opts, state)
		if err != nil {
			return nil, pos, err
		}

		switch *obj.Op {
		case "!", "not":
			unaryBool, ok := unary.(*BoolEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Bool)
			}

			return Not(unaryBool, state), obj.Pos, nil
		case "-":
			unaryInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			return Minus(unaryInt, state), pos, nil
		case "^":
			unaryInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			return IntNot(unaryInt, state), pos, nil
		}
		return nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)

	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			return identToEvaluator(&ident{Pos: obj.Pos, Ident: obj.Ident}, opts, state)
		case obj.Number != nil:
			return &IntEvaluator{
				Value:  *obj.Number,
				Offset: obj.Pos.Offset,
			}, obj.Pos, nil
		case obj.Variable != nil:
			varname, ok := isVariableName(*obj.Variable)
			if !ok {
				return nil, obj.Pos, NewError(obj.Pos, "internal variable error '%s'", varname)
			}

			return evaluatorFromVariable(varname, obj.Pos, opts)
		case obj.FieldReference != nil:
			fieldname, ok := isFieldReferenceName(*obj.FieldReference)
			if !ok {
				return nil, obj.Pos, NewError(obj.Pos, "internal field reference error '%s'", fieldname)
			}

			return evaluatorFromFieldReference(fieldname, obj.Pos, state)
		case obj.Duration != nil:
			return &IntEvaluator{
				Value:      *obj.Duration,
				Offset:     obj.Pos.Offset,
				isDuration: true,
			}, obj.Pos, nil
		case obj.String != nil:
			str := *obj.String

			// contains variables or field references
			hasVariables := len(variableRegex.FindAllIndex([]byte(str), -1)) > 0
			hasFieldReferences := len(fieldReferenceRegex.FindAllIndex([]byte(str), -1)) > 0
			if hasVariables || hasFieldReferences {
				return stringEvaluatorFromVariable(str, obj.Pos, opts, state)
			}

			return &StringEvaluator{
				Value:     str,
				ValueType: ScalarValueType,
				Offset:    obj.Pos.Offset,
			}, obj.Pos, nil
		case obj.Pattern != nil:
			evaluator := &StringEvaluator{
				Value:     *obj.Pattern,
				ValueType: PatternValueType,
				Weight:    PatternWeight,
				Offset:    obj.Pos.Offset,
			}
			return evaluator, obj.Pos, nil
		case obj.Regexp != nil:
			evaluator := &StringEvaluator{
				Value:     *obj.Regexp,
				ValueType: RegexpValueType,
				Weight:    RegexpWeight,
				Offset:    obj.Pos.Offset,
			}
			return evaluator, obj.Pos, nil
		case obj.IP != nil:
			ipnet, err := ParseCIDR(*obj.IP)
			if err != nil {
				return nil, obj.Pos, NewError(obj.Pos, "invalid IP '%s'", *obj.IP)
			}

			evaluator := &CIDREvaluator{
				Value:     *ipnet,
				ValueType: IPNetValueType,
				Offset:    obj.Pos.Offset,
			}
			return evaluator, obj.Pos, nil
		case obj.CIDR != nil:
			ipnet, err := ParseCIDR(*obj.CIDR)
			if err != nil {
				return nil, obj.Pos, NewError(obj.Pos, "invalid CIDR '%s'", *obj.CIDR)
			}

			evaluator := &CIDREvaluator{
				Value:     *ipnet,
				ValueType: IPNetValueType,
				Offset:    obj.Pos.Offset,
			}
			return evaluator, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression, opts, state)
		default:
			return nil, obj.Pos, NewError(obj.Pos, "unknown primary '%s'", reflect.TypeOf(obj))
		}
	case *ast.Array:
		return arrayToEvaluator(obj, opts, state)
	}

	return nil, lexer.Position{}, NewError(lexer.Position{}, "unknown entity '%s'", reflect.TypeOf(obj))
}
