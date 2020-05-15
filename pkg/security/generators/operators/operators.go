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

func {{ .FuncName }}(a *{{ .Arg1Type }}, b *{{ .Arg2Type }}, opts *Opts, state *state) *{{ .FuncReturnType }} {
	partialA, partialB := a.isPartial, b.isPartial

	if a.Eval == nil || (a.Field != "" && a.Field != state.field) {
		partialA = true
	}
	if b.Eval == nil || (b.Field != "" && b.Field != state.field) {
		partialB = true
	}
	isPartialLeaf := partialA && partialB

	if a.Field != "" && b.Field != "" {
		isPartialLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

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

		return &{{ .FuncReturnType }}{
			DebugEval: func(ctx *Context) {{ .EvalReturnType }} {
				ctx.evalDepth++
				op1, op2 := dea(ctx), deb(ctx)
				result := op1 {{.Op}} op2
				ctx.Logf("Evaluating %v {{ .Op }} %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) {{ .EvalReturnType }} {
				return ea(ctx) {{ .Op }} eb(ctx)
			},
			isPartial: isPartialLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
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
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		if a.Field != "" {
			state.UpdateFieldValues(a.Field, FieldValue{Value: eb, Type: ScalarValueType})
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

		return &{{ .FuncReturnType }}{
			DebugEval: func(ctx *Context) {{ .EvalReturnType }} {
				ctx.evalDepth++
				op1, op2 := dea(ctx), eb
				result := op1 {{ .Op }} op2
				ctx.Logf("Evaluating %v {{.Op}} %v => %v", op1, op2, result)
				ctx.evalDepth--
				return result
			},
			Eval: func(ctx *Context) {{ .EvalReturnType }} {
				return ea(ctx) {{ .Op }} eb
			},
			isPartial: isPartialLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	if b.Field != "" {
		state.UpdateFieldValues(b.Field, FieldValue{Value: ea, Type: ScalarValueType})
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

	return &{{ .FuncReturnType }}{
		DebugEval: func(ctx *Context) {{ .EvalReturnType }} {
			ctx.evalDepth++
			op1, op2 := ea, deb(ctx)
			result := op1 {{ .Op }} op2
			ctx.Logf("Evaluating %v {{ .Op }} %v => %v", op1, op2, result)
			ctx.evalDepth--
			return result
		},
		Eval: func(ctx *Context) {{ .EvalReturnType }} {
			return ea {{ .Op }} eb(ctx)
		},
		isPartial: isPartialLeaf,
	}
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
	}{
		{
			FuncName:       "Or",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "||",
		},
		{
			FuncName:       "And",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "&&",
		},
		{
			FuncName:       "IntEquals",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
		},
		{
			FuncName:       "IntNotEquals",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
		},
		{
			FuncName:       "IntAnd",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "&",
		},
		{
			FuncName:       "IntOr",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "|",
		},
		{
			FuncName:       "IntXor",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "IntEvaluator",
			EvalReturnType: "int",
			Op:             "^",
		},
		{
			FuncName:       "StringEquals",
			Arg1Type:       "StringEvaluator",
			Arg2Type:       "StringEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
		},
		{
			FuncName:       "StringNotEquals",
			Arg1Type:       "StringEvaluator",
			Arg2Type:       "StringEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
		},
		{
			FuncName:       "BoolEquals",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "==",
		},
		{
			FuncName:       "BoolNotEquals",
			Arg1Type:       "BoolEvaluator",
			Arg2Type:       "BoolEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "!=",
		},
		{
			FuncName:       "GreaterThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             ">",
		},
		{
			FuncName:       "GreaterOrEqualThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             ">=",
		},
		{
			FuncName:       "LesserThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "<",
		},
		{
			FuncName:       "LesserOrEqualThan",
			Arg1Type:       "IntEvaluator",
			Arg2Type:       "IntEvaluator",
			FuncReturnType: "BoolEvaluator",
			EvalReturnType: "bool",
			Op:             "<=",
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
