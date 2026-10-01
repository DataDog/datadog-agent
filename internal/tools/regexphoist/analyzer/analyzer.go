// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package analyzer checks for constant regular expressions compiled inside loops.
package analyzer

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// Analyzer reports constant regexp.MustCompile calls inside loops that should be package-level variables.
var Analyzer = &analysis.Analyzer{
	Name:             "regexphoist",
	Doc:              "flag constant regexp.MustCompile calls inside loops that should be hoisted to package-level variables",
	Run:              run,
	RunDespiteErrors: true,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var stack []struct {
				node   ast.Node
				inLoop bool
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if node == nil {
					stack = stack[:len(stack)-1]
					return true
				}
				inLoop := false
				if len(stack) > 0 {
					parent := stack[len(stack)-1]
					inLoop = parent.inLoop
					switch loop := parent.node.(type) {
					case *ast.ForStmt:
						inLoop = inLoop || node == loop.Body
					case *ast.RangeStmt:
						inLoop = inLoop || node == loop.Body
					}
				}
				if _, ok := node.(*ast.FuncLit); ok {
					inLoop = false
				}
				stack = append(stack, struct {
					node   ast.Node
					inLoop bool
				}{node: node, inLoop: inLoop})
				if call, ok := node.(*ast.CallExpr); ok && inLoop && isConstRegexpCompile(pass, call) {
					pass.Reportf(call.Pos(), "regexp.MustCompile with a constant pattern inside a loop should be hoisted to a package-level var so it compiles once, not on every iteration")
				}
				return true
			})
		}
	}
	return nil, nil
}

func isConstRegexpCompile(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || pass.TypesInfo.Types[call.Args[0]].Value == nil {
		return false
	}
	callee := typeutil.StaticCallee(pass.TypesInfo, call)
	if callee == nil || callee.Pkg() == nil || callee.Pkg().Path() != "regexp" {
		return false
	}
	return callee.FullName() == "regexp.MustCompile" || callee.FullName() == "regexp.MustCompilePOSIX"
}
