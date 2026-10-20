// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

// ScannerBufferAnalyzer checks that every bufio.NewScanner call in the
// analyzed package has a corresponding .Buffer() call on the returned value
// within the same function scope. Without Buffer(), the scanner uses a fixed
// 64 KiB internal buffer and fails on lines longer than that — a reliability
// and DoS risk for builtins that must handle arbitrary input.
var ScannerBufferAnalyzer = &analysis.Analyzer{
	Name: "scannerbuffer",
	Doc:  "checks that bufio.NewScanner results have Buffer() called to set a bounded read buffer",
	Run:  runScannerBuffer,
}

// OpenFileCloseAnalyzer checks that every callCtx.OpenFile call result that
// is assigned to a variable has a corresponding .Close() call (direct or via
// defer) within the same function scope. Unclosed file handles exhaust file
// descriptors over repeated script executions.
var OpenFileCloseAnalyzer = &analysis.Analyzer{
	Name: "openfileclose",
	Doc:  "checks that callCtx.OpenFile results are always closed within the same function",
	Run:  runOpenFileClose,
}

func runScannerBuffer(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		checkFileScannerBuffer(f, func(pos token.Pos, format string, args ...any) {
			pass.Reportf(pos, format, args...)
		})
	}
	return nil, nil
}

func runOpenFileClose(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		checkFileOpenFileClose(f, func(pos token.Pos, format string, args ...any) {
			pass.Reportf(pos, format, args...)
		})
	}
	return nil, nil
}

// checkFileScannerBuffer enforces the Scanner.Buffer() rule on a single file.
// It is also called directly by the test harness in symbols_test.go.
func checkFileScannerBuffer(f *ast.File, report func(pos token.Pos, format string, args ...any)) {
	forEachFuncBody(f, func(body *ast.BlockStmt) {
		type scannerVar struct {
			pos  token.Pos
			name string
		}
		var scanners []scannerVar
		buffered := make(map[string]bool)

		inspectBody(body, func(n ast.Node) {
			switch node := n.(type) {
			case *ast.AssignStmt:
				// Detect: x := bufio.NewScanner(...)
				for i, rhs := range node.Rhs {
					if !isCall(rhs, "bufio", "NewScanner") {
						continue
					}
					if i < len(node.Lhs) {
						if id, ok := node.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
							scanners = append(scanners, scannerVar{pos: rhs.Pos(), name: id.Name})
						}
					}
				}
			case *ast.CallExpr:
				// Detect: x.Buffer(...)
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Buffer" {
					if id, ok := sel.X.(*ast.Ident); ok {
						buffered[id.Name] = true
					}
				}
			}
		})

		for _, sc := range scanners {
			if !buffered[sc.name] {
				report(sc.pos,
					"bufio.NewScanner result %q must have .Buffer() called to cap the maximum line size (see analysis/README.md §Structural Rules)",
					sc.name)
			}
		}
	})
}

// checkFileOpenFileClose enforces the OpenFile-must-be-closed rule on a
// single file. It is also called directly by the test harness.
//
// The rule accounts for the common hand-off pattern:
//
//	f, err := callCtx.OpenFile(...)
//	rc = f          // hand off to rc
//	defer rc.Close() // closes f transitively
func checkFileOpenFileClose(f *ast.File, report func(pos token.Pos, format string, args ...any)) {
	forEachFuncBody(f, func(body *ast.BlockStmt) {
		type openVar struct {
			pos  token.Pos
			name string
		}
		// NOTE: opens is keyed on variable name as a string. If the same name is
		// reused for two successive OpenFile calls without closing the first, a
		// single Close() call will satisfy both entries and the second leak will
		// not be flagged. Use distinct variable names for successive file opens
		// in the same scope to ensure reliable detection.
		var opens []openVar
		closed := make(map[string]bool)
		// handOff maps a "holder" variable name to the original variable it was
		// assigned from. Closing the holder counts as closing the original.
		handOff := make(map[string]string) // holder → original
		// returned tracks variables that appear in any return statement.
		// Returning a file handle transfers ownership to the caller, so no
		// Close() is required in the current scope.
		//
		// NOTE: This check is path-insensitive. If a variable appears in a
		// return statement on one branch (e.g. the happy path) but is leaked on
		// another branch (e.g. a subsequent early-return), this checker will NOT
		// flag it. Full path-sensitive analysis requires CFG-based data flow,
		// which is beyond the scope of this AST-only checker.
		returned := make(map[string]bool)

		inspectBody(body, func(n ast.Node) {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for i, rhs := range node.Rhs {
					// Detect: f, err := <anything>.OpenFile(...)
					if call, ok := rhs.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "OpenFile" {
							if i < len(node.Lhs) {
								if id, ok := node.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
									opens = append(opens, openVar{pos: rhs.Pos(), name: id.Name})
								}
							}
						}
					}
					// Detect: rc = f (hand-off from OpenFile result to another var).
					// Works for both := and = assignments.
					if rhsId, ok := rhs.(*ast.Ident); ok && i < len(node.Lhs) {
						if lhsId, ok := node.Lhs[i].(*ast.Ident); ok && lhsId.Name != "_" {
							handOff[lhsId.Name] = rhsId.Name
						}
					}
				}
			case *ast.ReturnStmt:
				// Detect: return f, nil — caller takes ownership, no Close needed here.
				for _, result := range node.Results {
					if id, ok := result.(*ast.Ident); ok {
						returned[id.Name] = true
					}
				}
			case *ast.CallExpr:
				// Detect: f.Close() or defer f.Close() (the defer wrapper is
				// stripped by inspectBody before the CallExpr reaches here).
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Close" {
					if id, ok := sel.X.(*ast.Ident); ok {
						closed[id.Name] = true
					}
				}
			}
		})

		for _, ov := range opens {
			if !isClosedTransitive(ov.name, closed, handOff) && !returned[ov.name] {
				report(ov.pos,
					"OpenFile result %q must be closed via defer or explicit Close() call (see analysis/README.md §Structural Rules)",
					ov.name)
			}
		}
	})
}

// isClosedTransitive returns true if name is closed directly or transitively
// through a chain of hand-off assignments. It performs a breadth-first search
// over the handOff graph to handle chains of arbitrary depth, e.g.:
//
//	f → a → b → Close()   (handOff["a"]="f", handOff["b"]="a", closed["b"]=true)
func isClosedTransitive(name string, closed map[string]bool, handOff map[string]string) bool {
	visited := make(map[string]bool)
	queue := []string{name}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		if closed[cur] {
			return true
		}
		// Enqueue all variables that were assigned from cur.
		for holder, orig := range handOff {
			if orig == cur && !visited[holder] {
				queue = append(queue, holder)
			}
		}
	}
	return false
}

// forEachFuncBody calls fn for every function body in f — both top-level
// FuncDecl bodies and FuncLit bodies — so each scope is checked independently.
func forEachFuncBody(f *ast.File, fn func(*ast.BlockStmt)) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Body != nil {
				fn(node.Body)
			}
		case *ast.FuncLit:
			fn(node.Body)
		}
		return true
	})
}

// inspectBody walks body without recursing into nested FuncLit nodes (which
// are treated as independent scopes by forEachFuncBody). fn is called for
// every non-FuncLit node encountered.
func inspectBody(body *ast.BlockStmt, fn func(ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false // handled as its own scope by forEachFuncBody
		}
		fn(n)
		return true
	})
}

// checkFileCallCtxFields walks the AST of f and reports any selector
// expression that reads a tracked CallContext field without that field being
// declared in allowedFields.
//
// Three access patterns are detected, all using the same holders set:
//
//   - Depth-1: <bareIdent>.<field> where <bareIdent> is a known *CallContext
//     holder (function parameter or struct field typed *CallContext, or a local
//     variable assigned from one).
//     Example: callCtx.Truncate
//
//   - Depth-N: <expr>.<holder>.<field> (at any nesting depth) where <holder>
//     is a name in the holders set.
//     Example: ec.callCtx.Truncate (evalContext has a *CallContext field "callCtx")
//
//   - Local alias: cc := callCtx; cc.Truncate — handled by expanding holders
//     with findCallCtxLocalAliases before calling this function.
//
// holders should be built from findCallCtxHolderNames (declaration-based) plus
// findCallCtxLocalAliases (assignment-based) for the file being checked.
func checkFileCallCtxFields(
	f *ast.File,
	allFields map[string]bool,
	holders map[string]bool,
	allowedFields map[string]bool,
	usedFields map[string]bool,
	report func(pos token.Pos, format string, args ...any),
) {
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		fieldName := sel.Sel.Name
		if !allFields[fieldName] {
			return true // not a tracked CallContext field
		}

		isCallCtxAccess := false
		if ident, ok := sel.X.(*ast.Ident); ok {
			// Depth-1: bare identifier — flag only if it is a known
			// *CallContext holder, not an arbitrary local variable.
			isCallCtxAccess = holders[ident.Name]
		} else {
			// Depth-N: walk the selector chain; flag if any intermediate
			// selector name is a known holder.
			isCallCtxAccess = selectorChainHasHolder(sel.X, holders)
		}
		if !isCallCtxAccess {
			return true
		}

		if !allowedFields[fieldName] {
			report(sel.Pos(),
				"CallContext.%s is accessed but not declared in this builtin's builtinPerCommandCallContextFields entry",
				fieldName)
		} else if usedFields != nil {
			usedFields[fieldName] = true
		}
		return true
	})
}

// selectorChainHasHolder walks a chained selector expression and returns true
// if any intermediate selector name appears in holders.
//
// For ec.callCtx.Truncate, the call is selectorChainHasHolder(ec.callCtx, holders)
// where holders contains "callCtx". The walk finds the inner SelectorExpr
// ec.callCtx, sees Sel.Name "callCtx", and returns true.
//
// For ec.now.Truncate (time.Time.Truncate), "now" is not in holders, and the
// walk terminates at Ident("ec") returning false.
func selectorChainHasHolder(expr ast.Expr, holders map[string]bool) bool {
	cur := expr
	for {
		inner, ok := cur.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if holders[inner.Sel.Name] {
			return true
		}
		cur = inner.X
	}
}

// findCallCtxHolderNames returns the set of all identifier names in f that
// are statically known to hold a *CallContext value:
//
//   - Function parameters (in FuncDecl and FuncLit) typed *CallContext or
//     *<pkg>.CallContext.
//   - Struct fields of the same types ("bridge" fields used for depth-N
//     access such as ec.callCtx.Field).
//
// The package name component is not validated — any type named "CallContext"
// behind a pointer is accepted, so import aliases for the builtins package
// are handled correctly.
//
// This set is used by checkFileCallCtxFields to make depth-1 detection
// precise: only names in this set are flagged as CallContext accesses, which
// eliminates false-positives for local variables that coincidentally share a
// method name with a tracked field (e.g. dh.ReadDir on an fs.ReadDirFile).
func findCallCtxHolderNames(f *ast.File) map[string]bool {
	holders := make(map[string]bool)

	collectField := func(fieldType ast.Expr, names []*ast.Ident) {
		if !isStarCallContext(fieldType) {
			return
		}
		for _, name := range names {
			holders[name.Name] = true
		}
	}

	// Struct fields typed *CallContext.
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				collectField(field.Type, field.Names)
			}
		}
	}

	// Function parameters typed *CallContext (both named functions and literals).
	ast.Inspect(f, func(n ast.Node) bool {
		var params *ast.FieldList
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Type != nil {
				params = fn.Type.Params
			}
		case *ast.FuncLit:
			if fn.Type != nil {
				params = fn.Type.Params
			}
		}
		if params != nil {
			for _, field := range params.List {
				collectField(field.Type, field.Names)
			}
		}
		return true
	})

	return holders
}

// findCallCtxLocalAliases scans all assignment statements in f and returns
// the set of local variable names that are directly assigned from a
// *CallContext value expression. Iteration continues until no new aliases are
// discovered, so transitive chains are also covered:
//
//	cc := callCtx     → "cc" added (bare holder ident)
//	dd := cc          → "dd" added on the next pass (cc is now a holder)
//	ec := outer.callCtx → "ec" added (selector whose Sel is a bridge name)
//
// The returned aliases are intended to be merged with the per-file holders
// set before calling checkFileCallCtxFields.
func findCallCtxLocalAliases(f *ast.File, holders map[string]bool) map[string]bool {
	aliases := make(map[string]bool)
	for {
		// Build the combined set for this iteration.
		combined := make(map[string]bool, len(holders)+len(aliases))
		for k := range holders {
			combined[k] = true
		}
		for k := range aliases {
			combined[k] = true
		}

		newFound := false
		ast.Inspect(f, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range assign.Rhs {
				if i >= len(assign.Lhs) {
					break
				}
				lhsIdent, ok := assign.Lhs[i].(*ast.Ident)
				if !ok || lhsIdent.Name == "_" || combined[lhsIdent.Name] {
					continue
				}
				if isCallCtxValueExpr(rhs, combined) {
					aliases[lhsIdent.Name] = true
					newFound = true
				}
			}
			return true
		})
		if !newFound {
			break
		}
	}
	return aliases
}

// isCallCtxValueExpr reports whether expr evaluates to a *CallContext value:
//   - A bare identifier that is a known holder.
//   - A selector expression whose field name is a known holder (i.e. a struct
//     field typed *CallContext), at any chain depth.
//
// This deliberately does NOT match selectors where the terminal field is in
// allFields (those are tracked function-typed fields, not *CallContext values),
// so statRef := callCtx.LstatFile does not make statRef a holder.
func isCallCtxValueExpr(expr ast.Expr, holders map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return holders[e.Name]
	case *ast.SelectorExpr:
		// Match x.holderField at any depth — e.g. outer.ec.callCtx where
		// "callCtx" is a bridge field typed *CallContext.
		return holders[e.Sel.Name]
	}
	return false
}

// isStarCallContext reports whether expr is the AST form of *CallContext or
// *<pkg>.CallContext (a pointer to a type named "CallContext").
func isStarCallContext(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch t := star.X.(type) {
	case *ast.Ident:
		return t.Name == "CallContext"
	case *ast.SelectorExpr:
		return t.Sel.Name == "CallContext"
	}
	return false
}

// isCall returns true if expr is a call to pkg.Name (using the local package
// alias name, e.g. "bufio" for import "bufio").
func isCall(expr ast.Expr, localPkg, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == localPkg && sel.Sel.Name == name
}
