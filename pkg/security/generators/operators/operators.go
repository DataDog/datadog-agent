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
	RangeLimit     string
	StoreValue     bool
	OriginField    bool
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

	if field := a.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: b.Value, Type: {{ .ValueType }}}); err != nil {
			return nil, err
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: {{ .ValueType }}}); err != nil {
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
			{{- if and .StoreValue (eq .EvalReturnType "bool") }}
				va, vb := ea(ctx), eb(ctx)
				res := {{ call .Op "va" "vb" }}
				if res {
					ctx.AddMatchingSubExpr( MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
				}
				return res
			{{- else }}
				return {{ call .Op "ea(ctx)" "eb(ctx)" }}
			{{- end }}
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
			{{- if and .StoreValue (eq .EvalReturnType "bool") }}
				va, vb := ea(ctx), eb
				res := {{ call .Op "va" "vb" }}
				if res {
					ctx.AddMatchingSubExpr( MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vb, Offset: b.Offset})
				}
				return res
			{{- else }}
				return {{ call .Op "ea(ctx)" "eb" }}
			{{- end }}
		}

		return &{{ .FuncReturnType }}{
			EvalFnc: evalFnc,
			Weight: a.Weight,
			isDeterministic: isDc,
			{{- if .OriginField }}
			originField: a.OriginField(),
			{{- end }}
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
		{{- if and .StoreValue (eq .EvalReturnType "bool") }}
			va, vb := ea, eb(ctx)
			res := {{ call .Op "va" "vb" }}
			if res {
				ctx.AddMatchingSubExpr( MatchingValue{Value: va}, MatchingValue{Field: b.Field, Value: vb, Offset: b.Offset})
			}
			return res
		{{- else }}
			return {{ call .Op "ea" "eb(ctx)" }}
		{{- end }}
	}

	return &{{ .FuncReturnType }}{
		EvalFnc: evalFnc,
		Weight: b.Weight,
		isDeterministic: isDc,
		{{- if .OriginField }}
		originField: b.OriginField(),
		{{- end }}
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

	if field := a.OriginField(); field != "" {
		for _, value := range b.Values {
			if err := state.UpdateFieldValues(field, FieldValue{Value: value, Type: ScalarValueType}); err != nil {
				return nil, err
			}
		}
	}

	if field := b.OriginField(); field != "" {
		if err := state.UpdateFieldValues(field, FieldValue{Value: a.Value, Type: ScalarValueType}); err != nil {
			return nil, err
		}
	}

	arrayOp := func(ctx *Context, a {{ .ArrayType }}, b []{{ .ArrayType }}) (bool, {{ .ArrayType }}) {
		for _, v := range b {
			if {{ call .Op "a" "v" }} {
				return true, v
			}
		}
		return false, a
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			va, vb := ea(ctx), eb(ctx)
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr( MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &{{ .FuncReturnType }}{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + b.Weight,
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Values
		res, _ := arrayOp(nil, ea, eb)

		return &{{ .FuncReturnType }}{
			Value:     res,
			Weight:    a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Values

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			va, vb := ea(ctx), eb
			res, vm := arrayOp(ctx, va, vb)
			if res {
				ctx.AddMatchingSubExpr( MatchingValue{Field: a.Field, Value: va, Offset: a.Offset}, MatchingValue{Value: vm, Offset: b.Offset})
			}
			return res
		}

		return &{{ .FuncReturnType }}{
			EvalFnc:   evalFnc,
			Weight:    a.Weight + InArrayWeight*len(eb),
			isDeterministic: isDc,
			{{- if .OriginField }}
			originField: a.OriginField(),
			{{- end }}
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
		va, vb := ea, eb(ctx)
		res, vm := arrayOp(ctx, va, vb)
		if res {
			ctx.AddMatchingSubExpr( MatchingValue{Field: a.Field, Value: va}, MatchingValue{Field: b.Field, Value: vm, Offset: b.Offset})
		}
		return res
	}

	return &{{ .FuncReturnType }}{
		EvalFnc:   evalFnc,
		Weight:    b.Weight,
		isDeterministic: isDc,
		{{- if .OriginField }}
		originField: b.OriginField(),
		{{- end }}
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

	durationCompareArithmeticOperation := func(op string) func(a string, b string) string {
		return func(a string, b string) string {
			return fmt.Sprintf("int64(%s) %s int64(%s)", a, op, b)
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
				StoreValue:     true,
			},
			{
				FuncName:       "IntAnd",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("&"),
				ValueType:      "BitmaskValueType",
				OriginField:    true,
			},
			{
				FuncName:       "IntOr",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("|"),
				ValueType:      "BitmaskValueType",
				OriginField:    true,
			},
			{
				FuncName:       "IntXor",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("^"),
				ValueType:      "BitmaskValueType",
				OriginField:    true,
			},
			{
				FuncName:       "IntPlus",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("+"),
				ValueType:      "ScalarValueType",
				OriginField:    true,
			},
			{
				FuncName:       "IntMinus",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "IntEvaluator",
				EvalReturnType: "int",
				Op:             stdCompare("-"),
				ValueType:      "ScalarValueType",
				OriginField:    true,
			},
			{
				FuncName:       "BoolEquals",
				Arg1Type:       "BoolEvaluator",
				Arg2Type:       "BoolEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("=="),
				ValueType:      "ScalarValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "GreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "GreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "LesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "LesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationEqual",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("=="),
				ValueType:      "ScalarValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationLesserThanArithmeticOperation",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompareArithmeticOperation("<"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationLesserOrEqualThanArithmeticOperation",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompareArithmeticOperation("<="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationGreaterThanArithmeticOperation",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompareArithmeticOperation(">"),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationGreaterOrEqualThanArithmeticOperation",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompareArithmeticOperation(">="),
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationEqualArithmeticOperation",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompareArithmeticOperation("=="),
				ValueType:      "ScalarValueType",
				StoreValue:     true,
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
				StoreValue:     true,
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
				StoreValue:     true,
			},
			{
				FuncName:       "IntArrayGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">"),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "IntArrayGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare(">="),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "IntArrayLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<"),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "IntArrayLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             stdCompare("<="),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationArrayLesserThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<"),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationArrayLesserOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare("<="),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationArrayGreaterThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">"),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
			},
			{
				FuncName:       "DurationArrayGreaterOrEqualThan",
				Arg1Type:       "IntEvaluator",
				Arg2Type:       "IntArrayEvaluator",
				FuncReturnType: "BoolEvaluator",
				EvalReturnType: "bool",
				Op:             durationCompare(">="),
				ArrayType:      "int",
				ValueType:      "RangeValueType",
				StoreValue:     true,
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
