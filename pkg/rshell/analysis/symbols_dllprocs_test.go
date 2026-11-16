// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// dllCallees name a DLL via their first argument; NewProc names a procedure.
var dllCallees = map[string]bool{"NewLazySystemDLL": true, "NewLazyDLL": true}

// TestInternalDLLProcsArePinned asserts the DLL/procedure name literals passed
// to NewLazySystemDLL/NewLazyDLL/NewProc under builtins/internal match
// internalPerPackageDLLProcs. It parses source text, so it covers the
// Windows-only ntfsmft code on Linux CI too.
func TestInternalDLLProcsArePinned(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(filepath.Dir(wd), "builtins", "internal")

	files, err := collectGoFilesRecursive(targetDir, map[string]bool{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	foundDLLs := map[string]map[string]bool{}
	foundProcs := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range files {
		rel, _ := filepath.Rel(targetDir, path)
		pkg := firstPathSegment(rel)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, violation := range indirectDLLProcUses(fset, f) {
			t.Errorf("%s: %s", rel, violation)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			lit, ok := firstStringLitArg(call)
			if !ok {
				return true
			}
			switch {
			case dllCallees[sel.Sel.Name]:
				addToSet(foundDLLs, pkg, lit)
			case sel.Sel.Name == "NewProc":
				addToSet(foundProcs, pkg, lit)
			}
			return true
		})
	}

	// Every package with a found call must have an entry, and its found DLLs /
	// procs must match the pinned set exactly.
	pkgs := map[string]bool{}
	for p := range foundDLLs {
		pkgs[p] = true
	}
	for p := range foundProcs {
		pkgs[p] = true
	}
	for p := range internalPerPackageDLLProcs {
		pkgs[p] = true
	}

	for pkg := range pkgs {
		want, pinned := internalPerPackageDLLProcs[pkg]
		gotDLLs := setToSortedSlice(foundDLLs[pkg])
		gotProcs := setToSortedSlice(foundProcs[pkg])
		if !pinned {
			t.Errorf("package %q dynamically loads DLLs/procs %v / %v but has no internalPerPackageDLLProcs entry — add one after review", pkg, gotDLLs, gotProcs)
			continue
		}
		wantDLLs := append([]string(nil), want.DLLs...)
		wantProcs := append([]string(nil), want.Procs...)
		sort.Strings(wantDLLs)
		sort.Strings(wantProcs)
		if !equalStringSlices(gotDLLs, wantDLLs) {
			t.Errorf("package %q DLLs = %v, want %v (a new/changed DLL must be reviewed in internalPerPackageDLLProcs)", pkg, gotDLLs, wantDLLs)
		}
		if !equalStringSlices(gotProcs, wantProcs) {
			t.Errorf("package %q procedures = %v, want %v (a new/changed proc must be reviewed in internalPerPackageDLLProcs)", pkg, gotProcs, wantProcs)
		}
	}
}

// indirectDLLProcUses rejects taking a DLL loader or NewProc method as a
// function value, and rejects computed names. Either form would evade the
// literal pin set while retaining the native-code-loading capability.
func indirectDLLProcUses(fset *token.FileSet, file *ast.File) []string {
	var violations []string
	var ancestors []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			ancestors = ancestors[:len(ancestors)-1]
			return false
		}
		sel, watched := node.(*ast.SelectorExpr)
		if watched && (dllCallees[sel.Sel.Name] || sel.Sel.Name == "NewProc") {
			parent, directCall := lastNode(ancestors).(*ast.CallExpr)
			if !directCall || parent.Fun != sel {
				violations = append(violations, fmt.Sprintf("%s at line %d must be called directly, not used as a function value", sel.Sel.Name, fset.Position(sel.Pos()).Line))
			} else if _, literal := firstStringLitArg(parent); !literal {
				violations = append(violations, fmt.Sprintf("%s at line %d must use a string literal name", sel.Sel.Name, fset.Position(sel.Pos()).Line))
			}
		}
		ancestors = append(ancestors, node)
		return true
	})
	return violations
}

func TestInternalDLLProcPolicyRejectsIndirectionAndComputedNames(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package ntfsmft
func f() {
	loader := windows.NewLazySystemDLL
	proc := kernel32.NewProc
	name := "kernel32.dll"
	_ = windows.NewLazySystemDLL(name)
	_, _ = loader, proc
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := indirectDLLProcUses(fset, file); len(got) != 3 {
		t.Fatalf("indirect or computed DLL/proc uses = %v, want three violations", got)
	}
}

// firstPathSegment returns the first path component of a relative path, i.e. the
// builtins/internal subpackage directory name.
func firstPathSegment(rel string) string {
	rel = filepath.ToSlash(rel)
	if i := indexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return rel
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// firstStringLitArg returns the unquoted value of a call's first argument when
// it is a string literal.
func firstStringLitArg(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func addToSet(m map[string]map[string]bool, key, val string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][val] = true
}

func setToSortedSlice(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
