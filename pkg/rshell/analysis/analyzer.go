// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package analysis provides a go/analysis.Analyzer that enforces
// symbol-level import restrictions on Go source files.
//
// The analyzer checks that every imported symbol is in a given allowlist, that
// no permanently banned packages are imported, and that every symbol in the
// allowlist is actually used. It reports violations with file:line:col
// diagnostics.
package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerConfig configures a single instance of the allowed-symbols analyzer.
type AnalyzerConfig struct {
	// Symbols is the allowlist to enforce (e.g. builtinAllowedSymbols).
	// Each entry must be in "importpath.Symbol" form.
	Symbols []string
	// ExemptImport returns true for import paths that are auto-allowed and
	// should not be checked against the allowlist (e.g. same-module imports).
	ExemptImport func(importPath string) bool
	// ListName is used in diagnostic messages (e.g. "builtinAllowedSymbols").
	ListName string
}

// NewAnalyzer returns a go/analysis.Analyzer that enforces the symbol-level
// import restrictions described by cfg. Violations are reported via
// pass.Reportf and appear as diagnostics with proper file:line:col positions.
//
// NewAnalyzer panics if any entry in cfg.Symbols is malformed (no dot
// separator), matching the behaviour of the test-harness variant.
//
// NOTE: This analyzer only enforces symbol-level allowlist restrictions. For
// full static analysis coverage, callers should also register
// ScannerBufferAnalyzer and OpenFileCloseAnalyzer alongside this one. The
// test-harness path (checkAllowedSymbols) already applies all three checks
// automatically.
func NewAnalyzer(cfg AnalyzerConfig) *analysis.Analyzer {
	for _, entry := range cfg.Symbols {
		if strings.LastIndexByte(entry, '.') <= 0 {
			panic(fmt.Sprintf("analysis.NewAnalyzer: malformed allowlist entry (no dot): %q", entry))
		}
	}

	run := func(pass *analysis.Pass) (any, error) {
		allowedSyms, allowedPkgs := buildAllowlistSets(cfg.Symbols)
		usedSymbols := make(map[string]bool, len(cfg.Symbols))

		for _, f := range pass.Files {
			localToPath := checkFileImports(f, allowedPkgs, cfg.ExemptImport,
				func(pos token.Pos, format string, args ...any) {
					pass.Reportf(pos, format, args...)
				})

			checkFileSelectors(f, localToPath, allowedSyms, usedSymbols,
				func(pos token.Pos, format string, args ...any) {
					pass.Reportf(pos, format, args...)
				})
		}

		if len(pass.Files) > 0 {
			reportUnused(cfg.Symbols, usedSymbols, func(entry string) {
				pass.Reportf(pass.Files[0].Package,
					"allowlist symbol %q is not used by any file — remove it from %s",
					entry, cfg.ListName)
			})
		}

		return nil, nil
	}

	return &analysis.Analyzer{
		Name: "analysis",
		Doc:  "enforces symbol-level import restrictions via an allowlist",
		Run:  run,
	}
}

// buildAllowlistSets constructs lookup maps from a flat allowlist.
// It returns (allowedSymbols, allowedPackages) where allowedSymbols maps
// "pkg.Symbol" → true and allowedPackages maps "importpath" → true.
func buildAllowlistSets(symbols []string) (map[string]bool, map[string]bool) {
	allowedSyms := make(map[string]bool, len(symbols))
	allowedPkgs := make(map[string]bool)
	for _, entry := range symbols {
		dot := strings.LastIndexByte(entry, '.')
		if dot <= 0 {
			continue
		}
		allowedSyms[entry] = true
		allowedPkgs[entry[:dot]] = true
	}
	return allowedSyms, allowedPkgs
}

// checkFileImports validates each import in f against the permanently banned
// list, the exempt predicate, and allowedPkgs. It calls report for each
// violation and returns a localName→importPath map for the file's valid,
// non-exempt imports.
//
// report is called with a token.Pos and a pre-formatted message. Callers that
// use the file:line string form (e.g. fileLineReporter) set pos to
// token.NoPos; callers that surface diagnostics via pass.Reportf pass the
// actual source position.
func checkFileImports(
	f *ast.File,
	allowedPkgs map[string]bool,
	exemptImport func(string) bool,
	report func(pos token.Pos, format string, args ...any),
) map[string]string {
	localToPath := make(map[string]string)

	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		banned := false
		for key, reason := range permanentlyBanned {
			if strings.HasSuffix(key, "/") {
				if strings.HasPrefix(importPath, key) {
					report(imp.Pos(), "import of %q is permanently banned (%s)", importPath, reason)
					banned = true
					break
				}
			} else if importPath == key {
				report(imp.Pos(), "import of %q is permanently banned (%s)", importPath, reason)
				banned = true
				break
			}
		}
		if banned {
			continue
		}

		if exemptImport != nil && exemptImport(importPath) {
			continue
		}

		var localName string
		if imp.Name != nil {
			localName = imp.Name.Name
		} else {
			parts := strings.Split(importPath, "/")
			localName = parts[len(parts)-1]
		}

		if localName == "_" || localName == "." {
			report(imp.Pos(), "blank/dot import of %q is not allowed", importPath)
			continue
		}

		if !allowedPkgs[importPath] {
			report(imp.Pos(), "import of %q is not in the allowlist", importPath)
			continue
		}

		localToPath[localName] = importPath
	}

	return localToPath
}

// checkFileSelectors walks all ast.SelectorExpr nodes in f and checks each
// package-qualified symbol against allowedSyms. Allowed symbols are recorded
// in usedSymbols. Violations are sent to report.
func checkFileSelectors(
	f *ast.File,
	localToPath map[string]string,
	allowedSyms map[string]bool,
	usedSymbols map[string]bool,
	report func(pos token.Pos, format string, args ...any),
) {
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := localToPath[ident.Name]
		if !ok {
			return true // not a package-level selector
		}
		key := importPath + "." + sel.Sel.Name
		if !allowedSyms[key] {
			report(sel.Pos(), "%s is not in the allowlist", key)
		} else {
			usedSymbols[key] = true
		}
		return true
	})
}

// reportUnused calls onUnused for each symbol in symbols that is not present
// in usedSymbols.
func reportUnused(symbols []string, usedSymbols map[string]bool, onUnused func(entry string)) {
	for _, entry := range symbols {
		if !usedSymbols[entry] {
			onUnused(entry)
		}
	}
}
