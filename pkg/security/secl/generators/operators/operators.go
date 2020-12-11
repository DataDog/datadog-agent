// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package main

import (
	"flag"
	"os"
	"os/exec"
	"text/template"
)

var (
	output string
)

func main() {
	tmpl := template.Must(template.New("header").Parse(`

// Code generated - DO NOT EDIT.

package	eval

{{ range . }}

func {{ .FuncName }}(a *{{ .Arg1Type }}, b *{{ .Arg2Type }}, opts *Opts, state *state) (*{{ .FuncReturnType }}, error) {
	partialA, partialB := a.isPartial, b.isPartial

	if a.EvalFnc == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.EvalFnc == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.EvalFnc != nil && b.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.EvalFnc

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if state.field != "" {
				if a.isPartial {
					ea = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
				if b.isPartial {
					eb = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
			}
		{{ end }}

		// optimize the evaluation if needed, moving the evaluation with more weight at the right
		{{ if .Commutative }}
			if a.Weight > b.Weight {
				tmp := ea
				ea = eb
				eb = tmp
			}
		{{ end }}

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return ea(ctx) {{ .Op }} eb(ctx)
		}

		return &{{ .FuncReturnType }}{
			EvalFnc: evalFnc,
			Weight: a.Weight + b.Weight,
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc == nil && b.EvalFnc == nil {
		ea, eb := a.Value, b.Value

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
		if state.field != "" {
			if a.isPartial {
				ea = true
			}
			if b.isPartial {
				eb = true
			}
		}
		{{ end }}

		return &{{ .FuncReturnType }}{
			Value: ea {{ .Op }} eb,
			isPartial: isPartialLeaf,
		}, nil
	}

	if a.EvalFnc != nil {
		ea, eb := a.EvalFnc, b.Value

		if a.Field != "" {
			if err := state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: {{ .ValueType }}}); err != nil {
				return nil, err
			}
		}

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if state.field != "" {
				if a.isPartial {
					ea = func(ctx *Context) {{ .EvalReturnType }} {
						return true
					}
				}
				if b.isPartial {
					eb = true
				}
			}
		{{ end }}

		evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
			return ea(ctx) {{ .Op }} eb
		}

		return &{{ .FuncReturnType }}{
			EvalFnc: evalFnc,
			Weight: a.Weight,
			isPartial: isPartialLeaf,
		}, nil
	}

	ea, eb := a.Value, b.EvalFnc

	if b.Field != "" {
		if err := state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: {{ .ValueType }}}); err != nil {
			return nil, err
		}
	}

	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
		if state.field != "" {
			if a.isPartial {
				ea = true
			}
			if b.isPartial {
				eb = func(ctx *Context) {{ .EvalReturnType }} {
					return true
				}
			}
		}
	{{ end }}

	evalFnc := func(ctx *Context) {{ .EvalReturnType }} {
		return ea {{ .Op }} eb(ctx)
	}

	return &{{ .FuncReturnType }}{
		EvalFnc: evalFnc,
		Weight: b.Weight,
		isPartial: isPartialLeaf,
	}, nil
}
{{ end }}
`))

	outputFile, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	operators := []struct {
		FuncName       string
		Arg1Type       string
		Arg2Type       string
		FuncReturnType string
		EvalReturnType string
		Op             string
		ValueType      string
		Commutative    bool
	}{
		{
			FuncName:       "Or",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "||",
			ValueType:      "ScalarValueType",
			Commutative:    true,
		},
		{
			FuncName:       "And",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "&&",
			ValueType:      "ScalarValueType",
			Commutative:    true,
		},
		{
			FuncName:       "IntEquals",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "IntNotEquals",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "IntAnd",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "&",
			ValueType:      "BitmaskValueType",
		},
		{
			FuncName:       "IntOr",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "|",
			ValueType:      "BitmaskValueType",
		},
		{
			FuncName:       "IntXor",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "^",
			ValueType:      "BitmaskValueType",
		},
		{
			FuncName:       "StringEquals",
			Arg1Type:       "StringEvaluator",
			Arg2Type:       "StringEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "StringNotEquals",
			Arg1Type:       "StringEvaluator",
			Arg2Type:       "StringEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "BoolEquals",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "BoolNotEquals",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "GreaterThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             ">",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "GreaterOrEqualThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             ">=",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "LesserThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "<",
			ValueType:      "ScalarValueType",
		},
		{
			FuncName:       "LesserOrEqualThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "<=",
			ValueType:      "ScalarValueType",
		},
	}

	if err := tmpl.Execute(outputFile, operators); err != nil {
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
