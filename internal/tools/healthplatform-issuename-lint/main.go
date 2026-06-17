// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main provides a linter that checks healthplatform.Issue struct literals
// for the following conventions:
//
//   - IssueName must be present and non-empty
//   - IssueName must be a static value (string literal, named constant, or
//     qualified identifier) — dynamic expressions such as concatenation or
//     fmt.Sprintf are rejected to prevent variadic issue names at runtime
//   - IssueName string literals must match snake_case (^[a-z][a-z0-9_]*$)
//   - Title must be present and, when a string literal, non-empty
//
// Usage:
//
//	go run ./internal/tools/healthplatform-issuename-lint --path=./comp/healthplatform
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const healthPlatformImportPath = "github.com/DataDog/agent-payload/v5/healthplatform"

var issueNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var path = flag.String("path", ".", "Root directory to scan (recursive)")

func main() {
	flag.Parse()

	violations, err := lintDir(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if violations > 0 {
		fmt.Fprintf(os.Stderr, "\n%d violation(s) found\n", violations)
		os.Exit(1)
	}
}

func lintDir(root string) (int, error) {
	total := 0
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		n, err := lintFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error %s: %v\n", p, err)
		}
		total += n
		return nil
	})
	return total, err
}

func lintFile(filePath string) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return 0, err
	}

	alias := healthPlatformAlias(f)
	if alias == "" {
		return 0, nil
	}

	violations := 0
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if !isIssueType(lit.Type, alias) {
			return true
		}
		violations += checkIssueLit(fset, lit)
		return true
	})
	return violations, nil
}

func checkIssueLit(fset *token.FileSet, lit *ast.CompositeLit) int {
	violations := 0
	var issueNameExpr ast.Expr
	var titleExpr ast.Expr

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "IssueName":
			issueNameExpr = kv.Value
		case "Title":
			titleExpr = kv.Value
		}
	}

	pos := fset.Position(lit.Pos())
	if issueNameExpr == nil {
		fmt.Printf("%s:%d: healthplatform.Issue missing IssueName field\n", pos.Filename, pos.Line)
		violations++
	} else {
		violations += checkIssueName(fset, issueNameExpr)
	}

	if titleExpr == nil {
		fmt.Printf("%s:%d: healthplatform.Issue missing Title field\n", pos.Filename, pos.Line)
		violations++
	} else if bl, ok := titleExpr.(*ast.BasicLit); ok && bl.Kind == token.STRING {
		if strings.Trim(bl.Value, `"`) == "" {
			p := fset.Position(bl.Pos())
			fmt.Printf("%s:%d: healthplatform.Issue Title must not be empty\n", p.Filename, p.Line)
			violations++
		}
	}

	return violations
}

func checkIssueName(fset *token.FileSet, expr ast.Expr) int {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			pos := fset.Position(v.Pos())
			fmt.Printf("%s:%d: IssueName must be a string\n", pos.Filename, pos.Line)
			return 1
		}
		name := strings.Trim(v.Value, `"`)
		if name == "" {
			pos := fset.Position(v.Pos())
			fmt.Printf("%s:%d: IssueName must not be empty\n", pos.Filename, pos.Line)
			return 1
		}
		if !issueNamePattern.MatchString(name) {
			pos := fset.Position(v.Pos())
			fmt.Printf("%s:%d: IssueName %q violates convention — must match %s (snake_case, no hyphens or uppercase)\n",
				pos.Filename, pos.Line, name, issueNamePattern)
			return 1
		}
		return 0
	case *ast.Ident, *ast.SelectorExpr:
		// Named constant or qualified identifier — allowed; value is validated at registration time.
		return 0
	default:
		// Dynamic expression (e.g. concatenation, fmt.Sprintf): variadic issue names are forbidden.
		pos := fset.Position(expr.Pos())
		fmt.Printf("%s:%d: IssueName must be a static value (constant or string literal), not a dynamic expression\n",
			pos.Filename, pos.Line)
		return 1
	}
}

// healthPlatformAlias returns the local identifier used for the healthplatform
// import in file f, or "" if the package is not imported.
func healthPlatformAlias(f *ast.File) string {
	for _, imp := range f.Imports {
		if imp.Path == nil {
			continue
		}
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath != healthPlatformImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		parts := strings.Split(importPath, "/")
		return parts[len(parts)-1]
	}
	return ""
}

// isIssueType reports whether expr refers to <alias>.Issue or *<alias>.Issue.
func isIssueType(expr ast.Expr, alias string) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == alias && sel.Sel.Name == "Issue"
}
