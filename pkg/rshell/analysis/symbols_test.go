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
	"strings"
	"testing"
)

// allowedSymbolsConfig configures a single run of the allowed-symbols check.
type allowedSymbolsConfig struct {
	// Symbols is the allowlist to enforce (e.g. builtinAllowedSymbols).
	Symbols []string
	// TargetDir is the directory to scan, relative to the repo root.
	TargetDir string
	// CollectFiles walks TargetDir and returns the absolute paths of Go files
	// to check. It receives the absolute path to TargetDir.
	CollectFiles func(dir string) ([]string, error)
	// ExemptImport returns true for import paths that are auto-allowed and
	// should not be checked against the allowlist.
	ExemptImport func(importPath string) bool
	// ListName is used in error messages (e.g. "builtinAllowedSymbols").
	ListName string
	// MinFiles is the minimum number of files expected (sanity check).
	MinFiles int
	// RepoRootOverride, if set, is used instead of auto-detecting the repo
	// root from os.Getwd(). Used by verification tests that operate on a
	// temp copy.
	RepoRootOverride string
	// Errors, if non-nil, collects error messages instead of calling t.Errorf.
	// Used by verification tests to inspect specific errors.
	Errors *[]string
}

// checkAllowedSymbols enforces symbol-level import restrictions on a set of
// Go source files. It verifies that every imported symbol is in the allowlist,
// that no permanently banned packages are imported, and that every symbol in
// the allowlist is actually used.
//
// The core checking logic is provided by the analyzer helpers in analyzer.go
// (checkFileImports, checkFileSelectors, reportUnused) — this function handles
// file discovery, AST parsing, and test-framework integration.
func checkAllowedSymbols(t *testing.T, cfg allowedSymbolsConfig) {
	t.Helper()

	// Build lookup sets from the allowlist.
	allowedSyms, allowedPkgs := buildAllowlistSets(cfg.Symbols)
	usedSymbols := make(map[string]bool, len(cfg.Symbols))

	// Validate allowlist entries are well-formed.
	for _, entry := range cfg.Symbols {
		dot := strings.LastIndexByte(entry, '.')
		if dot <= 0 {
			t.Fatalf("malformed allowlist entry (no dot): %q", entry)
		}
	}

	// reportErr collects errors into cfg.Errors when set, otherwise calls t.Errorf.
	reportErr := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if cfg.Errors != nil {
			*cfg.Errors = append(*cfg.Errors, msg)
		} else {
			t.Errorf("%s", msg)
		}
	}

	// Determine the repo root.
	var root string
	if cfg.RepoRootOverride != "" {
		root = cfg.RepoRootOverride
	} else {
		// This package lives in analysis/, so the repo root is one level up.
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Dir(dir)
	}
	targetDir := filepath.Join(root, cfg.TargetDir)

	goFiles, err := cfg.CollectFiles(targetDir)
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	checked := 0
	for _, path := range goFiles {
		rel, _ := filepath.Rel(targetDir, path)
		checked++

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			reportErr("%s: parse error: %v", rel, err)
			continue
		}

		// Use analyzer helpers for import checking and selector walking.
		reporter := fileLineReporter(fset, rel, reportErr)
		localToPath := checkFileImports(f, allowedPkgs, cfg.ExemptImport, reporter)
		checkFileSelectors(f, localToPath, allowedSyms, usedSymbols, reporter)

		// Structural rules — applied to every checked file.
		checkFileScannerBuffer(f, reporter)
		checkFileOpenFileClose(f, reporter)
	}

	if checked < cfg.MinFiles {
		t.Fatalf("expected at least %d files in %s, found %d", cfg.MinFiles, cfg.TargetDir, checked)
	}

	// Verify every symbol in the allowlist is actually used by at least one
	// file. Unused entries should be removed to keep the allowlist minimal.
	reportUnused(cfg.Symbols, usedSymbols, func(entry string) {
		reportErr("allowlist symbol %q is not used by any file in %s — remove it from %s",
			entry, cfg.TargetDir, cfg.ListName)
	})
}

// perBuiltinConfig holds the configuration for checkPerBuiltinAllowedSymbols.
type perBuiltinConfig struct {
	// CommonSymbols is the global ceiling list (e.g. builtinAllowedSymbols).
	CommonSymbols []string
	// PerCommandSymbols maps each builtin name to its per-command allowlist.
	PerCommandSymbols map[string][]string
	// TargetDir is the directory containing builtin subdirectories.
	TargetDir string
	// ExemptImport returns true for import paths that are auto-allowed.
	ExemptImport func(importPath string) bool
	// SkipDirs is the set of subdirectory names to skip entirely.
	SkipDirs map[string]bool
	// RepoRootOverride, if set, overrides auto-detection of the repo root.
	RepoRootOverride string
	// Errors, if non-nil, collects error messages instead of calling t.Errorf.
	Errors *[]string
}

// checkPerBuiltinAllowedSymbols enforces two-layer symbol restrictions:
//  1. Every symbol in each per-command list must be in the common list.
//  2. Each builtin subdirectory's files may only use symbols from its per-command list.
//  3. Every symbol in a per-command list must be used by at least one file in that builtin.
//  4. Every symbol in the common list must appear in at least one per-command list.
//  5. Every builtin subdirectory must have an entry in the per-command map.
func checkPerBuiltinAllowedSymbols(t *testing.T, cfg perBuiltinConfig) {
	t.Helper()

	reportErr := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if cfg.Errors != nil {
			*cfg.Errors = append(*cfg.Errors, msg)
		} else {
			t.Errorf("%s", msg)
		}
	}

	// Build common set for quick lookup.
	commonSet := make(map[string]bool, len(cfg.CommonSymbols))
	for _, s := range cfg.CommonSymbols {
		commonSet[s] = true
	}

	// Track which common symbols appear in at least one per-command list.
	commonUsed := make(map[string]bool)

	// 1. Validate per-command lists are subsets of the common list.
	for cmd, symbols := range cfg.PerCommandSymbols {
		for _, s := range symbols {
			if !commonSet[s] {
				reportErr("builtinPerCommandSymbols[%q]: symbol %q is not in builtinAllowedSymbols", cmd, s)
			}
			commonUsed[s] = true
		}
	}

	// Determine repo root.
	var root string
	if cfg.RepoRootOverride != "" {
		root = cfg.RepoRootOverride
	} else {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Dir(dir)
	}
	targetDir := filepath.Join(root, cfg.TargetDir)

	// 2 & 3. Per-builtin file check + unused check.
	for cmd, symbols := range cfg.PerCommandSymbols {
		cmdDir := filepath.Join(targetDir, cmd)
		info, err := os.Stat(cmdDir)
		if err != nil || !info.IsDir() {
			reportErr("builtinPerCommandSymbols[%q]: directory %s does not exist", cmd, cmd)
			continue
		}

		goFiles, err := collectFlatGoFiles(cmdDir)
		if err != nil {
			t.Fatalf("builtinPerCommandSymbols[%q]: %v", cmd, err)
		}

		// Run checkAllowedSymbols scoped to this builtin's per-command list.
		var cmdErrs []string
		checkAllowedSymbols(t, allowedSymbolsConfig{
			Symbols:   symbols,
			TargetDir: filepath.Join(cfg.TargetDir, cmd),
			CollectFiles: func(dir string) ([]string, error) {
				return goFiles, nil
			},
			ExemptImport:     cfg.ExemptImport,
			ListName:         fmt.Sprintf("builtinPerCommandSymbols[%q]", cmd),
			MinFiles:         0,
			RepoRootOverride: root,
			Errors:           &cmdErrs,
		})

		for _, e := range cmdErrs {
			reportErr("%s", e)
		}
	}

	// 4. Common list coverage: every common symbol must appear in at least one per-command list.
	for _, s := range cfg.CommonSymbols {
		if !commonUsed[s] {
			reportErr("builtinAllowedSymbols symbol %q is not in any builtin's per-command list — remove it from builtinAllowedSymbols or add it to the appropriate builtin", s)
		}
	}

	// 5. Missing builtin check: every builtin subdirectory must have an entry.
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || cfg.SkipDirs[e.Name()] {
			continue
		}
		if _, ok := cfg.PerCommandSymbols[e.Name()]; !ok {
			reportErr("builtin subdirectory %q has no entry in builtinPerCommandSymbols", e.Name())
		}
	}
}

// perInterpModeConfig holds the configuration for checkInterpPerModeSymbols.
type perInterpModeConfig struct {
	// GlobalSymbols is the ceiling list (interpAllowedSymbols).
	GlobalSymbols []string
	// PerModeSymbols maps each mode name to its allowlist.
	// Must have exactly "read-only" and "remediation" keys.
	PerModeSymbols map[string][]string
	// TargetDir is the directory to scan, relative to the repo root (e.g. "interp").
	TargetDir string
	// ExemptImport returns true for import paths that are auto-allowed.
	ExemptImport func(importPath string) bool
	// RepoRootOverride, if set, overrides auto-detection of the repo root.
	RepoRootOverride string
	// Errors, if non-nil, collects error messages instead of calling t.Errorf.
	Errors *[]string
}

// checkInterpPerModeSymbols enforces two-layer per-mode symbol restrictions on interp/:
//  1. Every symbol in any per-mode list must be in GlobalSymbols (ceiling check).
//  2. Read-only files (.go not ending in _remediation.go) may only use "read-only" symbols.
//     Remediation files (_remediation.go) may use "read-only" ∪ "remediation" symbols.
//  3. Every symbol in each per-mode list must be used by at least one file in that mode.
//     The unused-per-mode check is skipped when no remediation files exist (MinFiles: 0).
//  4. Every symbol in GlobalSymbols must appear in at least one per-mode list.
//  5. Both "read-only" and "remediation" keys must exist in PerModeSymbols.
func checkInterpPerModeSymbols(t *testing.T, cfg perInterpModeConfig) {
	t.Helper()

	reportErr := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if cfg.Errors != nil {
			*cfg.Errors = append(*cfg.Errors, msg)
		} else {
			t.Errorf("%s", msg)
		}
	}

	// Check 5: required keys must exist.
	readOnlySymbols, hasReadOnly := cfg.PerModeSymbols["read-only"]
	remediationSymbols, hasRemediation := cfg.PerModeSymbols["remediation"]
	if !hasReadOnly {
		reportErr(`interpPerModeSymbols must have a "read-only" key`)
	}
	if !hasRemediation {
		reportErr(`interpPerModeSymbols must have a "remediation" key`)
	}
	if !hasReadOnly || !hasRemediation {
		return
	}

	// Build global ceiling set.
	globalSet := make(map[string]bool, len(cfg.GlobalSymbols))
	for _, s := range cfg.GlobalSymbols {
		globalSet[s] = true
	}

	// Track which global symbols appear in at least one per-mode list (for check 4).
	globalUsed := make(map[string]bool)

	// Check 1: every per-mode symbol must be in the global list.
	for mode, symbols := range cfg.PerModeSymbols {
		for _, s := range symbols {
			if !globalSet[s] {
				reportErr("interpPerModeSymbols[%q]: symbol %q is not in interpAllowedSymbols", mode, s)
			}
			globalUsed[s] = true
		}
	}

	// Determine repo root.
	var root string
	if cfg.RepoRootOverride != "" {
		root = cfg.RepoRootOverride
	} else {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Dir(dir)
	}
	targetDir := filepath.Join(root, cfg.TargetDir)

	// Collect and partition files into read-only and remediation.
	allFiles, err := collectFlatGoFiles(targetDir)
	if err != nil {
		t.Fatalf("checkInterpPerModeSymbols: %v", err)
	}

	var readOnlyFiles, remediationFiles []string
	for _, f := range allFiles {
		if strings.HasSuffix(filepath.Base(f), "_remediation.go") {
			remediationFiles = append(remediationFiles, f)
		} else {
			readOnlyFiles = append(readOnlyFiles, f)
		}
	}

	// Check 2+3 for read-only files via checkAllowedSymbols.
	var readOnlyErrs []string
	checkAllowedSymbols(t, allowedSymbolsConfig{
		Symbols:   readOnlySymbols,
		TargetDir: cfg.TargetDir,
		CollectFiles: func(_ string) ([]string, error) {
			return readOnlyFiles, nil
		},
		ExemptImport:     cfg.ExemptImport,
		ListName:         `interpPerModeSymbols["read-only"]`,
		MinFiles:         1,
		RepoRootOverride: root,
		Errors:           &readOnlyErrs,
	})
	for _, e := range readOnlyErrs {
		reportErr("%s", e)
	}

	// Check 2+3 for remediation files.
	// Effective allowlist = "read-only" ∪ "remediation" (containment).
	// Unused check applies only to remediation-specific symbols (not the full union).
	// MinFiles: 0 — skipped when no _remediation.go files exist yet.
	if len(remediationFiles) > 0 {
		unionSymbols := make([]string, 0, len(readOnlySymbols)+len(remediationSymbols))
		unionSymbols = append(unionSymbols, readOnlySymbols...)
		unionSymbols = append(unionSymbols, remediationSymbols...)

		unionSet, unionPkgs := buildAllowlistSets(unionSymbols)
		usedInRemediation := make(map[string]bool, len(unionSymbols))

		fset := token.NewFileSet()
		for _, path := range remediationFiles {
			rel, _ := filepath.Rel(targetDir, path)
			f, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				reportErr("%s: parse error: %v", rel, parseErr)
				continue
			}
			reporter := fileLineReporter(fset, rel, reportErr)
			localToPath := checkFileImports(f, unionPkgs, cfg.ExemptImport, reporter)
			checkFileSelectors(f, localToPath, unionSet, usedInRemediation, reporter)
			checkFileScannerBuffer(f, reporter)
			checkFileOpenFileClose(f, reporter)
		}

		// Check 3: each remediation-specific symbol must be used by at least one remediation file.
		reportUnused(remediationSymbols, usedInRemediation, func(entry string) {
			reportErr(`interpPerModeSymbols["remediation"] symbol %q is not used by any _remediation.go file — remove it from interpPerModeSymbols["remediation"] or add it to a _remediation.go file`, entry)
		})
	} else if len(remediationSymbols) > 0 {
		// Remediation symbols listed but no files exist to consume them.
		for _, s := range remediationSymbols {
			reportErr(`interpPerModeSymbols["remediation"] symbol %q is not used by any _remediation.go file — remove it from interpPerModeSymbols["remediation"] or add it to a _remediation.go file`, s)
		}
	}

	// Check 4: every global symbol must appear in at least one per-mode list.
	for _, s := range cfg.GlobalSymbols {
		if !globalUsed[s] {
			reportErr("interpAllowedSymbols symbol %q is not in any per-mode list — remove it from interpAllowedSymbols or add it to interpPerModeSymbols", s)
		}
	}
}

// callCtxFieldConfig holds the configuration for checkCallCtxFields.
type callCtxFieldConfig struct {
	// AllFields is the complete set of tracked CallContext function field names.
	AllFields []string
	// PerCommandFields maps each builtin name to its allowed CallContext fields.
	PerCommandFields map[string][]string
	// TargetDir is the directory containing builtin subdirectories, relative to
	// the repo root.
	TargetDir string
	// SkipDirs is the set of subdirectory names to skip entirely.
	SkipDirs map[string]bool
	// RepoRootOverride, if set, overrides auto-detection of the repo root.
	RepoRootOverride string
	// Errors, if non-nil, collects error messages instead of calling t.Errorf.
	Errors *[]string
}

// checkCallCtxFields enforces per-builtin CallContext field access restrictions:
//  1. Every field in each per-command list must be in AllFields (ceiling check).
//  2. Each builtin's files may only access CallContext fields in its per-command
//     list. Both depth-1 (callCtx.Field) and depth-N (ec.callCtx.Field) accesses
//     are detected. Depth-N detection uses a two-phase approach: first scan all
//     files in the builtin for struct fields typed *builtins.CallContext ("bridge"
//     fields), then flag selectors where any intermediate name is a bridge.
//  3. Every builtin subdirectory must have an entry in PerCommandFields.
func checkCallCtxFields(t *testing.T, cfg callCtxFieldConfig) {
	t.Helper()

	reportErr := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if cfg.Errors != nil {
			*cfg.Errors = append(*cfg.Errors, msg)
		} else {
			t.Errorf("%s", msg)
		}
	}

	// Build the global field set for quick lookup.
	allFieldsSet := make(map[string]bool, len(cfg.AllFields))
	for _, f := range cfg.AllFields {
		allFieldsSet[f] = true
	}

	// 1. Validate per-command lists are subsets of AllFields.
	for cmd, fields := range cfg.PerCommandFields {
		for _, f := range fields {
			if !allFieldsSet[f] {
				reportErr("builtinPerCommandCallContextFields[%q]: field %q is not in callCtxAllFields", cmd, f)
			}
		}
	}

	// Determine repo root.
	var root string
	if cfg.RepoRootOverride != "" {
		root = cfg.RepoRootOverride
	} else {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Dir(dir)
	}
	targetDir := filepath.Join(root, cfg.TargetDir)

	// 2. Per-builtin file check (two-phase: bridge discovery, then field access check).
	for cmd, allowedList := range cfg.PerCommandFields {
		cmdDir := filepath.Join(targetDir, cmd)
		info, err := os.Stat(cmdDir)
		if err != nil || !info.IsDir() {
			reportErr("builtinPerCommandCallContextFields[%q]: directory %s does not exist", cmd, cmd)
			continue
		}

		goFiles, err := collectFlatGoFiles(cmdDir)
		if err != nil {
			t.Fatalf("builtinPerCommandCallContextFields[%q]: %v", cmd, err)
		}

		allowedSet := make(map[string]bool, len(allowedList))
		for _, f := range allowedList {
			allowedSet[f] = true
		}

		// Phase 1: parse all files and collect *CallContext holder names.
		// A holder is a function parameter or struct field typed *builtins.CallContext.
		// The full holder set is needed before checking any file so that bridge fields
		// declared in one file are recognised when accessed in another.
		type parsedFile struct {
			f   *ast.File
			rel string
		}
		holders := make(map[string]bool)
		fset := token.NewFileSet()
		var parsed []parsedFile
		for _, path := range goFiles {
			rel, _ := filepath.Rel(targetDir, path)
			f, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				reportErr("%s: parse error: %v", rel, parseErr)
				continue
			}
			parsed = append(parsed, parsedFile{f: f, rel: rel})
			for name := range findCallCtxHolderNames(f) {
				holders[name] = true
			}
		}

		// Phase 2: check every file against the allowlist using per-file holders.
		// Per-file holders = declaration-based holders (params + struct fields,
		// collected above across all files in the builtin) plus local variable
		// aliases found within this specific file (cc := callCtx patterns).
		for _, pf := range parsed {
			fileHolders := make(map[string]bool, len(holders))
			for k := range holders {
				fileHolders[k] = true
			}
			for k := range findCallCtxLocalAliases(pf.f, holders) {
				fileHolders[k] = true
			}
			reporter := fileLineReporter(fset, pf.rel, reportErr)
			var used map[string]bool // unused check not enforced
			checkFileCallCtxFields(pf.f, allFieldsSet, fileHolders, allowedSet, used, reporter)
		}
	}

	// 3. Check that every builtin directory has an entry.
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || cfg.SkipDirs[e.Name()] {
			continue
		}
		if _, ok := cfg.PerCommandFields[e.Name()]; !ok {
			reportErr("builtin subdirectory %q has no entry in builtinPerCommandCallContextFields", e.Name())
		}
	}
}

// collectGoFilesRecursive walks a directory tree and returns all non-test .go
// files, including those at the top level. Directories named in skipDirs are
// pruned. Top-level files (rel has no separator) are excluded only when
// skipTopLevel is non-nil and returns true for their relative path.
func collectGoFilesRecursive(dir string, skipDirs map[string]bool, skipTopLevel func(rel string) bool) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if skipTopLevel != nil && skipTopLevel(rel) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// collectFlatGoFiles returns all non-test .go files directly in dir (not
// in subdirectories).
func collectFlatGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files, nil
}

// fileLineReporter returns a report function suitable for use with
// checkFileImports and checkFileSelectors in the test harness. It translates
// token.Pos into file:line strings using fset and forwards messages via errorf.
// When pos is token.NoPos, only the format+args message is emitted without a
// location prefix.
func fileLineReporter(fset *token.FileSet, relPath string, errorf func(string, ...any)) func(token.Pos, string, ...any) {
	return func(pos token.Pos, format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if pos != token.NoPos && fset != nil {
			p := fset.Position(pos)
			errorf("%s:%d: %s", relPath, p.Line, msg)
		} else {
			errorf("%s: %s", relPath, msg)
		}
	}
}
