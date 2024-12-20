// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compimpl is a linter that checks if the component implementation follows the expected naming convention.
package compimpl

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func init() {
	RegisterLinter("compimpl", "Check the component implementation follows the naming convention", run)
}

type implFolder struct {
	hasProvides    bool
	HasRequires    bool
	HasConstructor bool
	file           *ast.File // The first file found in the folder. Used to display the diagnostic
}

func run(pass *analysis.Pass) (interface{}, error) {
	// The regexp to match path like comp/core/autodiscovery/noopimpl/autoconfig.go but not the folders inside noopimpl
	isCompRegexp := regexp.MustCompile("(.*/comp/.*/.*/.*impl)/[^/]*$")
	compimpls := make(map[string]*implFolder)

	for _, f := range pass.Files {
		implFolder := tryGetCompImpl(pass, f, isCompRegexp, compimpls)
		if implFolder == nil {
			continue
		}

		structTypeNames := getExportedStructTypeNames(f)
		implFolder.hasProvides = implFolder.hasProvides || structTypeNames["Provides"]
		implFolder.HasRequires = implFolder.HasRequires || structTypeNames["Requires"]
		implFolder.HasConstructor = implFolder.HasConstructor || hasComponentConstructor(f)
	}
	displayDiagnostic(pass, compimpls)
	return nil, nil
}

func displayDiagnostic(pass *analysis.Pass, compimpls map[string]*implFolder) {
	for folderName, implFolder := range compimpls {
		file := implFolder.file
		if !implFolder.hasProvides {
			pass.Report(analysis.Diagnostic{
				Pos:     file.Pos(),
				Message: missingProvides(folderName),
			})
		}
		if !implFolder.HasRequires {
			pass.Report(analysis.Diagnostic{
				Pos:     file.Pos(),
				Message: missingRequires(folderName),
			})
		}
		if !implFolder.HasConstructor {
			pass.Report(analysis.Diagnostic{
				Pos:     file.Pos(),
				Message: missingConstructor(folderName),
			})
		}
	}
}

func tryGetCompImpl(
	pass *analysis.Pass,
	file *ast.File,
	isCompRegexp *regexp.Regexp,
	compimpls map[string]*implFolder) *implFolder {
	path := pass.Fset.Position(file.Pos()).Filename
	capture := isCompRegexp.FindStringSubmatch(path)
	if len(capture) != 2 {
		return nil
	}
	compimplPath := filepath.Clean(capture[1])
	if _, ok := compimpls[compimplPath]; !ok {
		compimpls[compimplPath] = &implFolder{file: file}
	}

	return compimpls[compimplPath]
}

func missingProvides(folderName string) string {
	return missingEntity(folderName, "type 'Provides'")
}

func missingEntity(folderName string, missingEntityMsg string) string {
	return fmt.Sprintf("%v does not follow the component naming convention. It doesn't contain the %v", folderName, missingEntityMsg)
}

func missingRequires(folderName string) string {
	return missingEntity(folderName, "type 'Requires'")
}

func missingConstructor(folderName string) string {
	return missingEntity(folderName, "constructor of the component doesn't take as parameter 'Provides' and doesn't return 'Requires'")
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
