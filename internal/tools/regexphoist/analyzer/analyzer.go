// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package analyzer checks for constant regular expressions compiled inside functions.
package analyzer

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// Analyzer reports constant regexp.MustCompile calls that should be package-level variables.
var Analyzer = &analysis.Analyzer{
	Name:             "regexphoist",
	Doc:              "hoist constant regexp.MustCompile calls to package-level variables",
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
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 || pass.TypesInfo.Types[call.Args[0]].Value == nil {
					return true
				}
				callee := typeutil.StaticCallee(pass.TypesInfo, call)
				if callee == nil || callee.Pkg() == nil || callee.Pkg().Path() != "regexp" {
					return true
				}
				if callee.FullName() == "regexp.MustCompile" || callee.FullName() == "regexp.MustCompilePOSIX" {
					pass.Reportf(call.Pos(), "regexp.MustCompile with a constant pattern should be a package-level var so it compiles once, not on every call")
				}
				return true
			})
		}
	}
	return nil, nil
}
