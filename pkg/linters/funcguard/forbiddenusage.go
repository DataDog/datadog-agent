// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package funcguard provides is a linter to detect unwanted/deprecated function calls in the code base.
package funcguard

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func init() {
	register.Plugin("funcguard", New)
}

type forbiddenUsagePlugin struct {
	rules map[string]string
}

// New returns a new forbiddenUsage linter plugin
func New(settings any) (register.LinterPlugin, error) {
	rulesConfig := []map[string]string{}
	err := mapstructure.Decode(settings, &rulesConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration for funcguard linter: %s", err)
	}

	rules := map[string]string{}
	for _, rule := range rulesConfig {
		rules[rule["function"]] = rule["message"]
	}

	return &forbiddenUsagePlugin{rules: rules}, nil
}

// BuildAnalyzers returns the analyzers for the plugin
func (f *forbiddenUsagePlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name:     "funcguard",
			Doc:      "detect unwanted/deprecated function calls",
			Run:      f.run,
			Requires: []*analysis.Analyzer{inspect.Analyzer},
		},
	}, nil
}

// GetLoadMode returns the load mode for the plugin
func (f *forbiddenUsagePlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (f *forbiddenUsagePlugin) run(pass *analysis.Pass) (interface{}, error) {
	inspector := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// filter needed nodes: imports and function calls
	nodeFilter := []ast.Node{
		(*ast.ImportSpec)(nil),
		(*ast.CallExpr)(nil),
	}

	importsAliases := map[string]string{}

	inspector.Preorder(nodeFilter, func(node ast.Node) {
		// Preorder traverse the tree from top to bottom guaranteeing we'll parse imports before function calls.

		// Is the node an import
		imp, ok := node.(*ast.ImportSpec)
		if ok {
			importPath := strings.Trim(imp.Path.Value, "\"")

			// Does the import has an alias
			if imp.Name != nil {
				importsAliases[imp.Name.Name] = importPath
			} else {
				importName := importPath[strings.LastIndex(importPath, "/")+1:]
				importsAliases[importName] = importPath
			}
			return
		}

		// Is the node a function call
		call, ok := node.(*ast.CallExpr)
		if ok {
			// We are only looking in 'package.function' call (ie: SelectorExpr)
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if ok {
				// We check for Indent to not match call like a.b.c() (ie: a single SelectorExpr)
				packageName, okPackage := sel.X.(*ast.Ident)
				if okPackage {
					methodName := sel.Sel.Name
					packageImport := importsAliases[packageName.Name]

					if message, ok := f.rules[packageImport+"."+methodName]; ok {
						pass.Reportf(node.Pos(), "funcguard: %s", message)
					}
				}
			}
			return
		}
	})

	return nil, nil
}
