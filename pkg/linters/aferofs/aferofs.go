// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aferofs implements a lint that checks for simultaneous use
// of methods from both afero mockable filesystem interface and the
// `os` package it is designed to mock.
package aferofs

import (
	"go/ast"
	"go/types"
	"slices"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("aferofs", New)
}

type plugin struct {
}

var Analyzer = &analysis.Analyzer{
	Name: "aferofs",
	Doc:  "Check for mixed use of aferofs and os packages",
	URL: "https://github.com/datadog/datadog-agent/tree/main/pkg/linters/aferofs/",
	Run:  run,
}

// New
func New(any) (register.LinterPlugin, error) {
	return &plugin{}, nil
}

// BuildAnalyzers
func (f *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

// GetLoadMode returns the load mode for the plugin
func (f *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		// For each function we check the body if it calls both one of
		// the filesystem related methods in the os package and
		// anything from the afero package at all.

		for _, d := range file.Decls {
			if fun, ok := d.(*ast.FuncDecl); ok {
				ck := &funcChecker{pass: pass}
				ast.Walk(ck, fun)
				if ck.afero != nil && ck.os != nil {
					pass.Report(analysis.Diagnostic{
						Pos:     ck.os.Pos(),
						End:     ck.os.End(),
						Message: "calls both os and spf13/afero methods",
					})
				}
			}
		}
	}

	var _ types.Pointer

	return nil, nil
}

type funcChecker struct {
	pass  *analysis.Pass
	afero ast.Node
	os    ast.Node
}

const aferoPath = "github.com/spf13/afero"
const osPath = "os"

// list of package level methods
var osFuncs = []string{
	"Chdir",
	"Chmod",
	"Chown",
	"Chtimes",
	"CopyFS",
	"DirFS",
	"Getwd",
	"Lchown",
	"Link",
	"Mkdir",
	"MkdirAll",
	"MkdirTemp",
	"WriteFile",
	// File constructors
	"Create",
	"CreateTemp",
	"Open",
	"OpenFile",
	// FileInfo constructors
	"Lstat",
	"Stat",
	// DirEntry constructors
	"ReadDir",
}

// and any method of these types
var osTypes = []string{
	"File",
	"DirEntry",
}

// Visit
func (c *funcChecker) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}

	ce, ok := n.(*ast.CallExpr)
	if !ok {
		return c
	}

	se, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return c
	}

	// pkg.method(...)
	// check package path
	if id, ok := se.X.(*ast.Ident); ok {
		obj := c.pass.TypesInfo.Uses[id]

		if pkg, ok := obj.(*types.PkgName); ok {
			switch pkg.Imported().Path() {
			case aferoPath:
				c.afero = n
			case osPath:
				if slices.Contains(osFuncs, se.Sel.Name) {
					c.os = n
				}
			}
		}
	}

	// expr.method(...)
	// check the expr type's package and name
	if tv, ok := c.pass.TypesInfo.Types[se.X]; ok {
		ty := baseOf(tv.Type)

		if named, ok := ty.(*types.Named); ok {
			packagePath, tyName := typePackage(named)
			switch packagePath {
			case aferoPath:
				c.afero = n
			case osPath:
				if slices.Contains(osTypes, tyName) {
					c.os = n
				}
			}
		}
	}

	return c
}

// Get the (transitive) base type of a pointer.
func baseOf(ty types.Type) types.Type {
	for {
		if p, ok := ty.(*types.Pointer); ok {
			ty = p.Elem()
		} else {
			return ty
		}
	}
}

// typePackage returns a type's package path and name as strings.
func typePackage(ty *types.Named) (string, string) {
	if ty == nil {
		return "", ""
	}
	name := ty.Obj()
	if name == nil {
		return "", ""
	}
	pkg := name.Pkg()
	if pkg == nil {
		return "", ""
	}
	return pkg.Path(), name.Name()
}
