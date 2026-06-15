// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main provides a linter that checks healthplatform.Issue struct literals
// for IssueName field values that don't follow snake_case convention
// (must match ^[a-z][a-z0-9_]*$).
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
		fmt.Fprintf(os.Stderr, "\n%d IssueName violation(s) found\n", violations)
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

	// Resolve which local identifier refers to the healthplatform package.
	alias := healthPlatformAlias(f)
	if alias == "" {
		return 0, nil // file doesn't import the package
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
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "IssueName" {
				continue
			}
			// Only validate string literals — variable/const references are
			// checked by the registry's runtime validation.
			val, ok := kv.Value.(*ast.BasicLit)
			if !ok || val.Kind != token.STRING {
				continue
			}
			name := strings.Trim(val.Value, `"`)
			if !issueNamePattern.MatchString(name) {
				pos := fset.Position(kv.Pos())
				fmt.Printf("%s:%d: IssueName %q violates convention — must match %s (snake_case, no hyphens or uppercase)\n",
					pos.Filename, pos.Line, name, issueNamePattern)
				violations++
			}
		}
		return true
	})
	return violations, nil
}

// healthPlatformAlias returns the local identifier used for the healthplatform
// import in file f, or "" if it is not imported.
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
		// Default local name: last path segment.
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
