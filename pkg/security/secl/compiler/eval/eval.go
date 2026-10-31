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
	"regexp"
	"slices"
	"strconv"
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

// identToEvaluator resolves an identifier: a constant, a macro or a field.
func identToEvaluator(name string, pos ast.Position, opts *Opts, state *State) (interface{}, ast.Position, error) {
	if accessor, ok := opts.Constants[name]; ok {
		return accessor, pos, nil
	}

	if state.macros != nil {
		if macro, ok := state.macros.GetMacroEvaluator(name); ok {
			return macro.Value, pos, nil
		}
	}

	field, itField, regID, arrayIndex, isArrayAccess, err := extractField(name)
	if err != nil {
		return nil, pos, err
	}

	// transform extracted field to support legacy SECL fields
	if opts.LegacyFields != nil {
		if newField, ok := opts.LegacyFields[field]; ok {
			field = newField
		}
	}

	evaluator, err := state.model.GetEvaluator(field, regID, pos.Offset)
	if err != nil {
		return nil, pos, err
	}

	state.UpdateFields(field)

	// Handle numeric array index access (e.g., field[0])
	if isArrayAccess {
		wrappedEvaluator, err := WrapEvaluatorWithArrayIndex(evaluator, arrayIndex, field)
		if err != nil {
			return nil, pos, NewError(pos, "failed to create array index accessor: %v", err)
		}
		return wrappedEvaluator, pos, nil
	}

	if regID != "" {
		// avoid wildcard register for the moment
		if regID == "_" {
			return nil, pos, NewError(pos, "`_` can't be used as a iterator variable name")
		}

		// avoid using the same register on two different fields
		if slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID && r.Field != itField
		}) {
			return nil, pos, NewError(pos, "iterator variable used by different fields '%s'", regID)
		}

		if !slices.ContainsFunc(state.registers, func(r Register) bool {
			return r.ID == regID
		}) {
			state.registers = append(state.registers, Register{ID: regID, Field: itField})
		}
	}

	return evaluator, pos, nil
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

func evaluatorFromLengthHanlder(fieldname string, pos ast.Position, state *State) (interface{}, ast.Position, error) {
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
func evaluatorFromFieldReference(fieldname string, pos ast.Position, state *State) (interface{}, ast.Position, error) {
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

func evaluatorFromVariable(varname string, pos ast.Position, opts *Opts) (interface{}, ast.Position, error) {
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

func stringEvaluatorFromVariable(str string, pos ast.Position, opts *Opts, state *State) (interface{}, ast.Position, error) {
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
