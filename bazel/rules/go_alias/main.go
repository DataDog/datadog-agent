// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main generates a Go source file that forwards every exported
// symbol of a source package to a caller-chosen import path, using Go 1.9
// type aliases (type X = pkg.X), Go 1.24 generic type aliases
// (type X[T any] = pkg.X[T]), and value aliases (var/const F = pkg.F).
// Generic functions can't be aliased as values, so they get a forwarding
// wrapper function with the same generic signature instead.
//
// This produces a drop-in compatibility shim: a package whose exported API
// is byte-for-byte compatible with the source package, and whose types are
// exactly the same compiled types (not copies), so values flow between the
// two import paths without conversion. It's meant for migrating an archived
// or deprecated import path onto a maintained fork without touching any
// existing callers of the old path.
//
// The module embedding the generated file must declare `go 1.24` or later
// for the generic type alias syntax to be accepted.
package main

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"go/format"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

func main() {
	src := flag.String("src", "", "import path of the source package to forward to")
	pkgName := flag.String("pkg", "", "package name for the generated shim file")
	alias := flag.String("alias", "real", "identifier used for the imported source package in generated code")
	out := flag.String("out", "", "output file path")
	goBinary := flag.String("go", "", "path to the go binary used to resolve the source package (defaults to PATH)")
	goRoot := flag.String("goroot", "", "GOROOT for the go binary given via -go")
	flag.Parse()

	if *src == "" || *pkgName == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: aliasgen -src=<import path> -pkg=<package name> -out=<file path> [-alias=<name>] [-go=<path>] [-goroot=<path>]")
		os.Exit(2)
	}

	code, skipped, varNames, err := generate(*src, *pkgName, *alias, *goBinary, *goRoot)
	if err != nil {
		log.Fatalf("aliasgen: %v", err)
	}
	if len(skipped) > 0 {
		for _, s := range skipped {
			fmt.Fprintf(os.Stderr, "aliasgen: cannot forward %s (%s)\n", s.name, s.reason)
		}
		log.Fatalf("aliasgen: refusing to write an incomplete shim; %d symbol(s) could not be forwarded", len(skipped))
	}
	if len(varNames) > 0 {
		fmt.Fprintf(os.Stderr, "aliasgen: %v are aliased by value, not by reference: mutations to the source package's var are not visible through the shim and vice versa\n", varNames)
	}

	if err := os.WriteFile(*out, code, 0o644); err != nil {
		log.Fatalf("aliasgen: writing %s: %v", *out, err)
	}
}

type skippedSymbol struct {
	name   string
	reason string
}

// qualifiedImports assigns a stable import identifier to every package a
// qualifier call encounters, so printed type expressions (constraints,
// signatures) can reference types from packages other than the source
// package.
type qualifiedImports struct {
	alias     string // identifier for src itself
	srcPath   string
	aliasOf   map[string]string // import path -> identifier
	usedNames map[string]bool
}

func newQualifiedImports(srcPath, alias string) *qualifiedImports {
	return &qualifiedImports{
		alias:     alias,
		srcPath:   srcPath,
		aliasOf:   map[string]string{srcPath: alias},
		usedNames: map[string]bool{alias: true},
	}
}

func (q *qualifiedImports) qualifier(p *types.Package) string {
	if name, ok := q.aliasOf[p.Path()]; ok {
		return name
	}
	name, base := p.Name(), p.Name()
	for i := 2; q.usedNames[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	q.usedNames[name] = true
	q.aliasOf[p.Path()] = name
	return name
}

// extraImports returns the non-source packages referenced while printing
// types, sorted by import path for deterministic output.
func (q *qualifiedImports) extraImports() []struct{ name, path string } {
	var extra []struct{ name, path string }
	for path, name := range q.aliasOf {
		if path == q.srcPath {
			continue
		}
		extra = append(extra, struct{ name, path string }{name, path})
	}
	slices.SortFunc(extra, func(a, b struct{ name, path string }) int { return cmp.Compare(a.path, b.path) })
	return extra
}

func generate(src, pkgName, alias, goBinary, goRoot string) ([]byte, []skippedSymbol, []string, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes}
	if goBinary != "" {
		// go/packages resolves the "go" command via exec.LookPath against this
		// process's own PATH, not cfg.Env (which only applies to the spawned
		// subprocess's environment), so PATH must be set here directly.
		goBinary, err := filepath.Abs(goBinary)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("resolving -go: %w", err)
		}
		if err := os.Setenv("PATH", filepath.Dir(goBinary)); err != nil {
			return nil, nil, nil, fmt.Errorf("setting PATH: %w", err)
		}
		// Resolve against the root module's own go.mod/go.sum only: go.work
		// mode would require every workspace member's go.mod to be present.
		if err := os.Setenv("GOWORK", "off"); err != nil {
			return nil, nil, nil, fmt.Errorf("setting GOWORK: %w", err)
		}
		// A sandboxed action's environment has no HOME, which the module
		// tooling needs to compute GOPATH/GOMODCACHE defaults.
		home, err := os.MkdirTemp("", "aliasgen-home")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("creating scratch HOME: %w", err)
		}
		if err := os.Setenv("HOME", home); err != nil {
			return nil, nil, nil, fmt.Errorf("setting HOME: %w", err)
		}
		if goRoot != "" {
			goRoot, err := filepath.Abs(goRoot)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("resolving -goroot: %w", err)
			}
			if err := os.Setenv("GOROOT", goRoot); err != nil {
				return nil, nil, nil, fmt.Errorf("setting GOROOT: %w", err)
			}
		}
	}

	pkgs, err := packages.Load(cfg, src)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading %s: %w", src, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, nil, nil, fmt.Errorf("loading %s: see errors above", src)
	}
	if len(pkgs) != 1 {
		return nil, nil, nil, fmt.Errorf("expected exactly one package for %s, got %d", src, len(pkgs))
	}
	pkg := pkgs[0]

	scope := pkg.Types.Scope()
	names := scope.Names()
	slices.Sort(names)

	imports := newQualifiedImports(src, alias)
	var body bytes.Buffer
	var skipped []skippedSymbol
	var varNames []string

	for _, name := range names {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}

		switch o := obj.(type) {
		case *types.Const:
			fmt.Fprintf(&body, "\nconst %s = %s.%s\n", name, alias, name)
		case *types.Func:
			sig := o.Type().(*types.Signature)
			if sig.TypeParams().Len() == 0 {
				fmt.Fprintf(&body, "\nvar %s = %s.%s\n", name, alias, name)
				continue
			}
			writeForwardingFunc(&body, name, sig, alias, imports.qualifier)
		case *types.TypeName:
			named, ok := o.Type().(*types.Named)
			if !ok || named.TypeParams().Len() == 0 {
				fmt.Fprintf(&body, "\ntype %s = %s.%s\n", name, alias, name)
				continue
			}
			params, args := typeParamLists(named.TypeParams(), imports.qualifier)
			fmt.Fprintf(&body, "\ntype %s[%s] = %s.%s[%s]\n", name, params, alias, name, args)
		case *types.Var:
			varNames = append(varNames, name)
			fmt.Fprintf(&body, "\nvar %s = %s.%s\n", name, alias, name)
		default:
			skipped = append(skipped, skippedSymbol{name, fmt.Sprintf("unsupported object kind %T", obj)})
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by aliasgen from %s; DO NOT EDIT.\n\n", src)
	fmt.Fprintf(&buf, "package %s\n\n", pkgName)
	buf.WriteString("import (\n")
	fmt.Fprintf(&buf, "\t%s %q\n", alias, src)
	for _, extra := range imports.extraImports() {
		fmt.Fprintf(&buf, "\t%s %q\n", extra.name, extra.path)
	}
	buf.WriteString(")\n")
	buf.Write(body.Bytes())

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("formatting generated source: %w", err)
	}
	return formatted, skipped, varNames, nil
}

// typeParamLists renders a type parameter list as both its declaration form
// ("T any, K comparable") and its use form ("T, K").
func typeParamLists(tparams *types.TypeParamList, qualifier types.Qualifier) (decl, use string) {
	var declParts, useParts []string
	for tp := range tparams.TypeParams() {
		declParts = append(declParts, fmt.Sprintf("%s %s", tp.Obj().Name(), types.TypeString(tp.Constraint(), qualifier)))
		useParts = append(useParts, tp.Obj().Name())
	}
	return strings.Join(declParts, ", "), strings.Join(useParts, ", ")
}

// writeForwardingFunc emits a real function (not an alias) that reproduces a
// generic function's signature and calls straight through to it, since
// generic functions can't be assigned to a var without instantiation.
func writeForwardingFunc(w *bytes.Buffer, name string, sig *types.Signature, alias string, qualifier types.Qualifier) {
	tparamsDecl, tparamsUse := typeParamLists(sig.TypeParams(), qualifier)

	params := sig.Params()
	variadic := sig.Variadic()
	var paramDecls, argNames []string
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		pname := p.Name()
		if pname == "" || pname == "_" {
			pname = fmt.Sprintf("p%d", i)
		}
		typeStr := types.TypeString(p.Type(), qualifier)
		if variadic && i == params.Len()-1 {
			typeStr = "..." + types.TypeString(p.Type().(*types.Slice).Elem(), qualifier)
			argNames = append(argNames, pname+"...")
		} else {
			argNames = append(argNames, pname)
		}
		paramDecls = append(paramDecls, fmt.Sprintf("%s %s", pname, typeStr))
	}

	results := sig.Results()
	var resultTypes []string
	for r := range results.Variables() {
		resultTypes = append(resultTypes, types.TypeString(r.Type(), qualifier))
	}
	resultDecl := strings.Join(resultTypes, ", ")
	if results.Len() > 1 {
		resultDecl = "(" + resultDecl + ")"
	}

	call := fmt.Sprintf("%s.%s[%s](%s)", alias, name, tparamsUse, strings.Join(argNames, ", "))
	fmt.Fprintf(w, "\nfunc %s[%s](%s) %s {\n", name, tparamsDecl, strings.Join(paramDecls, ", "), resultDecl)
	if results.Len() > 0 {
		fmt.Fprintf(w, "\treturn %s\n", call)
	} else {
		fmt.Fprintf(w, "\t%s\n", call)
	}
	w.WriteString("}\n")
}
