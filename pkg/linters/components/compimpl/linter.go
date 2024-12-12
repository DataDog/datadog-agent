// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compimpl is a linter that checks if the component implementation follows the expected naming convention.
package compimpl

import (
	"fmt"
	"go/ast"
	"os"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func init() {
	RegisterLinter("compimpl", "Check the component implementation follows the naming convention", run)
}

func run(pass *analysis.Pass) (interface{}, error) {
	r := regexp.MustCompile(".*comp/.*/.*/.*impl/.*")
	for _, f := range pass.Files {
		implFolder, err := doesFileImplementComp(pass, f, r)
		if !implFolder {
			continue
		}
		if err != nil {
			return nil, err
		}

		structTypeNames := getExportedStructTypeNames(f)
		if !structTypeNames["Provides"] {
			pass.Report(analysis.Diagnostic{
				Pos:     f.Pos(),
				Message: missingProvides(),
			})
		}

		if !structTypeNames["Requires"] {
			pass.Report(analysis.Diagnostic{
				Pos:     f.Pos(),
				Message: missingRequires(),
			})
		}
		if !hasComponentConstructor(f) {
			pass.Report(analysis.Diagnostic{
				Pos:     f.Pos(),
				Message: missingConstructor(),
			})
		}
	}

	return nil, nil
}

func doesFileImplementComp(pass *analysis.Pass, file *ast.File, r *regexp.Regexp) (bool, error) {
	path := pass.Fset.Position(file.Pos()).Filename
	if r.MatchString(path) {
		content, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		return strings.Contains(string(content), "func Module() fxutil.Module {"), nil
	}
	return false, nil
}

func missingProvides() string {
	return missingEntity("type 'Provides'")
}

func missingEntity(missingEntityMsg string) string {
	return fmt.Sprintf("This file does not follow the component naming convention. It doesn't contain the %v", missingEntityMsg)
}

func missingRequires() string {
	return missingEntity("type 'Requires'")
}

func missingConstructor() string {
	return missingEntity("constructor of the component doesn't take as parameter 'Provides' and doesn't return 'Requires'")
}

func getExportedStructTypeNames(f *ast.File) map[string]bool {
	structTypes := make(map[string]bool)
	ast.Inspect(f, func(node ast.Node) bool {
		switch t := node.(type) {
		case *ast.TypeSpec:
			if t.Name.IsExported() {
				_, ok := t.Type.(*ast.StructType)
				if ok {
					structTypes[t.Name.Name] = true
				}
			}
		}
		return true
	})
	return structTypes
}

func hasComponentConstructor(f *ast.File) bool {
	foundConstructor := false
	ast.Inspect(f, func(node ast.Node) bool {
		funcDecl, ok := node.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if !strings.HasPrefix(funcDecl.Name.Name, "New") {
			return true
		}

		params := funcDecl.Type.Params.List
		results := funcDecl.Type.Results.List
		if len(params) != 1 || len(results) < 1 || len(results) > 2 {
			return true

		}
		if containFieldsType(params, "Requires") && containFieldsType(results, "Provides") {
			foundConstructor = true
			return false
		}

		return false
	})
	return foundConstructor
}

func containFieldsType(fields []*ast.Field, typeName string) bool {
	for _, field := range fields {
		ident, ok := field.Type.(*ast.Ident)
		if !ok {
			continue
		}
		if ident.Name == typeName {
			return true
		}
	}
	return false
}
