// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// SpanTypeCaseLabels returns the string case labels of the switch statements
// on a span type in the Go sources under root, test files and testdata
// excluded. A switch is on a span type when its tag is a Type field or a
// Type() method call, for example `switch span.Type {` or
// `switch s.Type() {`. Only string literal labels are returned, so switches on
// non-string Type fields are ignored, and so are labels that are constants,
// switches on another expression (such as a variable holding the type) and
// if/else chains comparing the type.
func SpanTypeCaseLabels(root string) ([]string, error) {
	labels := make(map[string]bool)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.SwitchStmt)
			if !ok || !isTypeSelector(s.Tag) {
				return true
			}
			for _, stmt := range s.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if v, err := strconv.Unquote(lit.Value); err == nil {
							labels[v] = true
						}
					}
				}
			}
			return true
		})
		return nil
	})
	return slices.Sorted(maps.Keys(labels)), err
}

// isTypeSelector reports whether e is x.Type or x.Type().
func isTypeSelector(e ast.Expr) bool {
	if call, ok := e.(*ast.CallExpr); ok && len(call.Args) == 0 {
		e = call.Fun
	}
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Type"
}

// MissingCorpusSpanTypes returns the labels that are not in CorpusSpanTypes.
func MissingCorpusSpanTypes(labels []string) []string {
	return slices.DeleteFunc(slices.Clone(labels), func(l string) bool {
		return slices.Contains(CorpusSpanTypes, l)
	})
}
