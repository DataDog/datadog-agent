// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"text/template"
)

var (
	output string
)

// Operator defines an operator
type Operator struct {
	FuncName       string
	Arg1Type       string
	Arg2Type       string
	FuncReturnType string
	EvalReturnType string
	Op             func(a string, b string) string
	ArrayType      string
	ValueType      string
	Commutative    bool
}

func main() {
	tmpl := template.Must(template.New("header").Parse(`

// Code generated - DO NOT EDIT.

package	eval

import (
	"errors"
)

{{ range .Operators }}

func {{ .FuncName }}(a *{{ .Arg1Type }}, b *{{ .Arg2Type }}, state *State) (*{{ .FuncReturnType }}, error) {
	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
	isDc := a.IsDeterministicFor(state.field) || b.IsDeterministicFor(state.field)
	{{ else }}
	isDc := isArithmDeterministic(a, b, state)
	{{ end }}

	if a.Field != "" {
		if err := state.UpdateFieldValues(a.Field, FieldValue{Value: b.Value, Type: {{ .ValueType }}}); err != nil {
			return nil, err
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: {{ .ValueType }}}); err != nil {
			return nil, err
		}
	}

	{{ if eq .ValueType "BitmaskValueType" }}
	if a.EvalFnc != nil && b.EvalFnc != nil {
		return nil, errors.New("full dynamic bitmask operation not supported")
	}
	{{ else }}
	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if state.field != "" {
				if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
					ea = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
				if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
					eb = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
			}
		{{ end }}

		{{/* optimize the evaluation if needed, moving the evaluation with more weight at the right */}}
		{{ if .Commutative }}
			if a.Weight > b.Weight {
				tmp := ea
				ea = eb
				eb = tmp
			}
		{{ end }}

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return {{ call .Op "ea(ctx)" "eb(ctx)" }}
		}

		return &{{ .FuncReturnType }}{
			EvalFnc: evalFnc,
			Weight: a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}
	{{ end }}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		ctx := NewContext(nil)
		_ = ctx

		return &{{ .FuncReturnType }}{
			Value: {{ call .Op "ea" "eb" }},
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if state.field != "" {
				if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
					ea = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
				if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
					eb = true
				}
			}
		{{ end }}

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return {{ call .Op "ea(ctx)" "eb" }}
		}

		return &{{ .FuncReturnType }}{
			EvalFnc: evalFnc,
			Field: a.Field,
			Weight: a.Weight,
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
		if state.field != "" {
			if !a.IsDeterministicFor(state.field) && !a.IsStatic() {
				ea = true
			}
			if !b.IsDeterministicFor(state.field) && !b.IsStatic() {
				eb = func(ctx *Context) {{ .EvalReturnType }} {
					return true
				}
			}
		}
	{{ end }}

	evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
		return {{ call .Op "ea" "eb(ctx)" }}
	}

	return &{{ .FuncReturnType }}{
		EvalFnc: evalFnc,
		Field: b.Field,
		Weight: b.Weight,
		isDeterministic: isDc,
	}, nil
}
{{ end }}

{{ range .ArrayOperators }}

func {{ .FuncName }}(a *{{ .Arg1Type }}, b *{{ .Arg2Type }}, state *State) (*{{ .FuncReturnType }}, error) {
	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
	isDc := a.IsDeterministicFor(state.field) || b.IsDeterministicFor(state.field)
	{{ else }}
	isDc := isArithmDeterministic(a, b, state)
	{{ end }}

	if a.Field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a {{ .ArrayType }}, b []{{ .ArrayType }}) bool {
		for _, v := range b {
			if {{ call .Op "a" "v" }} {
				return true
			}
		}
		return false
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return arrayOp(ctx, ea(ctx), eb(ctx))
		}

		return &{{ .FuncReturnType }}{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values

		ctx := NewContext(nil)
		_ = ctx

		return &{{ .FuncReturnType }}{
			Value:     arrayOp(ctx, ea, eb),
			Weight:    a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return arrayOp(ctx, ea(ctx), eb)
		}

		return &{{ .FuncReturnType }}{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
		return arrayOp(ctx, ea, eb(ctx))
	}

	return &{{ .FuncReturnType }}{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isDeterministic: isDc,
	}, nil
}
{{end}}
`))

	outputFile, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	stdCompare := func(op string) func(a string, b string) string {
		return func(a string, b string) string {
			return fmt.Sprintf("%s %s %s", a, op, b)
		}
	}

	durationCompare := func(op string) func(a string, b string) string {
		return func(a string, b string) string {
			return fmt.Sprintf("ctx.Now().UnixNano() - int64(%s) %s int64(%s)", a, op, b)
		}
	}

	data := struct {
		Operators      []Operator
		ArrayOperators []Operator
	}{
		Operators: []Operator{
			{
				FuncName:       "IntEquals",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("=="),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "IntAnd",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("&"),
				ValueType:      "BitmaskValueType",
			},
			{
				FuncName:       "IntOr",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("|"),
				ValueType:      "BitmaskValueType",
			},
			{
				FuncName:       "IntXor",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("^"),
				ValueType:      "BitmaskValueType",
			},
			{
				FuncName:       "BoolEquals",
				Arg1Type:       "BoolEvaluator",
				Arg2Type:       "BoolEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("=="),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "GreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">"),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "GreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">="),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "LesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<"),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "LesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<="),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<"),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<="),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">"),
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">="),
				ValueType:      "ScalarValueType",
			},
		},
		ArrayOperators: []Operator{
			{
				FuncName:       "IntArrayEquals",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("=="),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "BoolArrayEquals",
				Arg1Type:       "BoolEvaluator",
				Arg2Type:       "BoolArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("=="),
				ArrayType:      "bool",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "IntArrayGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">"),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "IntArrayGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">="),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "IntArrayLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<"),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "IntArrayLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<="),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationArrayLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<"),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationArrayLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<="),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationArrayGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">"),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
			{
				FuncName:       "DurationArrayGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">="),
				ArrayType:      "int",
				ValueType:      "ScalarValueType",
			},
		},
	}

	if err := tmpl.Execute(outputFile, data); err != nil {
		panic(err)
	}

	if err := outputFile.Close(); err != nil {
		panic(err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", output)
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func init() {
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
