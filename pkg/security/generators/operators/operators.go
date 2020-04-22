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

import ("fmt")

{{ range . }}

func {{ .FuncName }}(a *{{ .Arg1Type }}, b *{{ .Arg2Type }}, opts *Opts, state *State) *{{ .FuncReturnType }} {
	{{ $bool := "false" }}
	{{ if eq .FuncName "And" }}
		{{ $bool = "true" }}
	{{ end }}

	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
		fmt.Printf("YYYYYYYYYYYYYYYYYYYYYYY\n")
	{{ end }}

	var isOpLeaf bool
	if opts.PartialField != "" && (a.ModelField != "" || b.ModelField != "") {
		isOpLeaf = true
	}

	if a.Eval != nil && b.Eval != nil {
		ea, eb := a.Eval, b.Eval
		dea, deb := a.DebugEval, b.DebugEval

		eval := func(ctx *Context) {{ .EvalReturnType }} {
			return ea(ctx) {{ .Op }} eb(ctx)
		}

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if opts.PartialField != "" {
				if a.IsOpLeaf && !b.IsOpLeaf {
					eval = func(ctx *Context) {{ .EvalReturnType }} {
						return ea(ctx) {{ .Op }} {{ $bool }}
					}
				} else if !a.IsOpLeaf && b.IsOpLeaf {
					eval = func(ctx *Context) {{ .EvalReturnType }} {
						return {{ $bool }} {{ .Op }} eb(ctx)
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
			Eval: eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	if a.Eval == nil && b.Eval == nil {
		return &{{ .FuncReturnType }}{
			Value: a.Value {{ .Op }} b.Value,
		}
	}

	if a.Eval != nil {
		ea, eb := a.Eval, b.Value
		dea := a.DebugEval

		eval := func(ctx *Context) {{ .EvalReturnType }} {
			return ea(ctx) {{ .Op }} eb
		}

		{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
			if opts.PartialField != "" {
				if !a.IsOpLeaf {
					eval = func(ctx *Context) {{ .EvalReturnType }} {
						return {{ $bool }} {{ .Op }} eb
					}
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
			Eval: eval,
			IsOpLeaf: isOpLeaf,
		}
	}

	ea, eb := a.Value, b.Eval
	deb := b.DebugEval

	eval := func(ctx *Context) {{ .EvalReturnType }} {
		return ea {{ .Op }} eb(ctx)
	}

	{{ if or (eq .FuncName "Or") (eq .FuncName "And") }}
		if opts.PartialField != "" {
			if !b.IsOpLeaf {
				eval = func(ctx *Context) {{ .EvalReturnType }} {
					return ea {{ .Op }} {{ $bool }}
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
		Eval: eval,
		IsOpLeaf: isOpLeaf,
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
