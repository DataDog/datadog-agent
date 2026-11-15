// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builtinsVerifyCfg returns a builtinsCheckConfig with RepoRootOverride and
// Errors set for verification testing.
func builtinsVerifyCfg(tempRoot string, errs *[]string) allowedSymbolsConfig {
	cfg := builtinsCheckConfig()
	cfg.RepoRootOverride = tempRoot
	cfg.Errors = errs
	return cfg
}

// builtinsPerCmdVerifyCfg returns a perBuiltinConfig with overrides for
// verification testing.
func builtinsPerCmdVerifyCfg(tempRoot string, errs *[]string) perBuiltinConfig {
	cfg := builtinsPerCommandCheckConfig()
	cfg.RepoRootOverride = tempRoot
	cfg.Errors = errs
	return cfg
}

func TestVerificationBuiltinsCleanPass(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if len(errs) > 0 {
		t.Errorf("expected no errors on clean copy, got:\n%s", strings.Join(errs, "\n"))
	}
}

func TestVerificationBuiltinsBannedPackageExact(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	injectImport(t, target, `"os/exec"`, "var _ = exec.Command")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "permanently banned") {
		t.Errorf("expected 'permanently banned' error for os/exec, got: %v", errs)
	}
}

func TestVerificationBuiltinsBannedPackagePrefix(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	injectImport(t, target, `"net/http"`, "var _ = http.Get")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "permanently banned") {
		t.Errorf("expected 'permanently banned' error for net/http, got: %v", errs)
	}
}

func TestVerificationBuiltinsUnlistedSymbol(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	injectUnlistedSymbol(t, target)

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "os.Setenv") || !errContains(errs, "not in the allowlist") {
		t.Errorf("expected 'not in the allowlist' error for os.Setenv, got: %v", errs)
	}
}

func TestVerificationBuiltinsUnlistedPackage(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	injectImport(t, target, `"crypto/rand"`, "var _ = rand.Read")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "not in the allowlist") {
		t.Errorf("expected 'not in the allowlist' error for crypto/rand, got: %v", errs)
	}
}

func TestVerificationBuiltinsBlankImport(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	injectImport(t, target, `_ "encoding/json"`, "")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "blank/dot import") {
		t.Errorf("expected 'blank/dot import' error, got: %v", errs)
	}
}

func TestVerificationBuiltinsExemptImport(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := findFirstSubdirGoFile(t, filepath.Join(tmp, "builtins"))
	// builtins/internal/* imports are exempt — should not trigger an error.
	injectImport(t, target, `internalfoo "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/fakepkg"`, "var _ = internalfoo.Foo")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if errContains(errs, "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/fakepkg") {
		t.Errorf("exempt import should not be flagged, got: %v", errs)
	}
}

func TestVerificationBuiltinsSkipsTopLevel(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Inject a banned import into builtins.go (which is skipped by skipTopLevel).
	target := filepath.Join(tmp, "builtins", "builtins.go")
	injectImport(t, target, `"os/exec"`, "var _ = exec.Command")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if errContains(errs, "os/exec") {
		t.Errorf("builtins.go should be skipped, but got error: %v", errs)
	}
}

// TestVerificationBuiltinsChecksNonExemptTopLevel confirms that top-level
// files in builtins/ other than builtins.go are audited. This guards against
// regression of the vuln-hunt F-1 finding (2026-05-18-initial-audit /
// builtin-import-allowlist), where a subdir-only filter in the collector
// silently excluded proc_provider.go and features.go from the allowlist check.
func TestVerificationBuiltinsChecksNonExemptTopLevel(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	target := filepath.Join(tmp, "builtins", "proc_provider.go")
	injectImport(t, target, `"os/exec"`, "var _ = exec.Command")

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if !errContains(errs, "permanently banned") || !errContains(errs, "os/exec") {
		t.Errorf("expected 'permanently banned' error for os/exec in proc_provider.go, got: %v", errs)
	}
}

func TestVerificationBuiltinsSkipsTestutilDir(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Create a violating file inside testutil/ (which is skipped by skipDirs).
	writeGoFile(t,
		filepath.Join(tmp, "builtins", "testutil", "bad.go"),
		"testutil",
		[]string{`"os/exec"`},
		"var _ = exec.Command\n",
	)

	var errs []string
	checkAllowedSymbols(t, builtinsVerifyCfg(tmp, &errs))

	if errContains(errs, "os/exec") {
		t.Errorf("testutil/ should be skipped, but got error: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// Per-command verification tests
// ---------------------------------------------------------------------------

func TestVerificationPerCmdCleanPass(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	var errs []string
	checkPerBuiltinAllowedSymbols(t, builtinsPerCmdVerifyCfg(tmp, &errs))

	if len(errs) > 0 {
		t.Errorf("expected no errors on clean copy, got:\n%s", strings.Join(errs, "\n"))
	}
}

func TestVerificationPerCmdSymbolNotInCommonList(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Override the per-command config to inject a symbol not in the common list.
	cfg := builtinsPerCmdVerifyCfg(tmp, nil)
	cfg.PerCommandSymbols = copyPerCommandMap(cfg.PerCommandSymbols)
	cfg.PerCommandSymbols["echo"] = append(cfg.PerCommandSymbols["echo"], "os.Remove")

	var errs []string
	cfg.Errors = &errs
	checkPerBuiltinAllowedSymbols(t, cfg)

	if !errContains(errs, "os.Remove") || !errContains(errs, "not in builtinAllowedSymbols") {
		t.Errorf("expected error about os.Remove not in common list, got: %v", errs)
	}
}

func TestVerificationPerCmdSymbolNotInPerCommandList(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Find a builtin that uses fmt.Sprintf (e.g. "ls") and remove it from its per-command list.
	cfg := builtinsPerCmdVerifyCfg(tmp, nil)
	cfg.PerCommandSymbols = copyPerCommandMap(cfg.PerCommandSymbols)
	// Remove "fmt.Sprintf" from ls's list.
	filtered := make([]string, 0, len(cfg.PerCommandSymbols["ls"]))
	for _, s := range cfg.PerCommandSymbols["ls"] {
		if s != "fmt.Sprintf" {
			filtered = append(filtered, s)
		}
	}
	cfg.PerCommandSymbols["ls"] = filtered

	var errs []string
	cfg.Errors = &errs
	checkPerBuiltinAllowedSymbols(t, cfg)

	// When fmt.Sprintf is the only fmt symbol, removing it makes the entire
	// fmt package unlisted, so the error may mention the package or the symbol.
	if !errContains(errs, "fmt") || !errContains(errs, "not in the allowlist") {
		t.Errorf("expected error about fmt not allowed for ls, got: %v", errs)
	}
}

func TestVerificationPerCmdUnusedSymbolFlagged(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Add an unused (but common-list-valid) symbol to echo's per-command list.
	cfg := builtinsPerCmdVerifyCfg(tmp, nil)
	cfg.PerCommandSymbols = copyPerCommandMap(cfg.PerCommandSymbols)
	cfg.PerCommandSymbols["echo"] = append(cfg.PerCommandSymbols["echo"], "regexp.Compile")

	var errs []string
	cfg.Errors = &errs
	checkPerBuiltinAllowedSymbols(t, cfg)

	if !errContains(errs, "regexp.Compile") || !errContains(errs, "not used") {
		t.Errorf("expected error about unused regexp.Compile in echo, got: %v", errs)
	}
}

func TestVerificationPerCmdMissingBuiltinEntry(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Remove "echo" from the per-command map.
	cfg := builtinsPerCmdVerifyCfg(tmp, nil)
	cfg.PerCommandSymbols = copyPerCommandMap(cfg.PerCommandSymbols)
	delete(cfg.PerCommandSymbols, "echo")

	var errs []string
	cfg.Errors = &errs
	checkPerBuiltinAllowedSymbols(t, cfg)

	if !errContains(errs, "echo") || !errContains(errs, "no entry in builtinPerCommandSymbols") {
		t.Errorf("expected error about missing echo entry, got: %v", errs)
	}
}

// copyPerCommandMap returns a shallow copy of a per-command symbols map so
// that verification tests can mutate it without affecting the original.
func copyPerCommandMap(m map[string][]string) map[string][]string {
	cp := make(map[string][]string, len(m))
	for k, v := range m {
		dup := make([]string, len(v))
		copy(dup, v)
		cp[k] = dup
	}
	return cp
}

// ---------------------------------------------------------------------------
// CallContext field verification tests
// ---------------------------------------------------------------------------

// builtinsCallCtxVerifyCfg returns a callCtxFieldConfig with RepoRootOverride
// and Errors set for verification testing.
func builtinsCallCtxVerifyCfg(tempRoot string, errs *[]string) callCtxFieldConfig {
	cfg := builtinsCallCtxCheckConfig()
	cfg.RepoRootOverride = tempRoot
	cfg.Errors = errs
	return cfg
}

// injectCallCtxFieldAccess appends a syntactically valid Go function to the
// file at path that contains a depth-1 CallContext field access. The parameter
// is typed *builtins.CallContext so that findCallCtxHolderNames recognises it
// as a holder and checkFileCallCtxFields flags the access.
func injectCallCtxFieldAccess(t *testing.T, path, fieldName string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	snippet := "\nfunc _callCtxFieldProbe(callCtxProbe *builtins.CallContext) { _ = callCtxProbe." + fieldName + " }\n"
	if err := os.WriteFile(path, append(data, []byte(snippet)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationCallCtxCleanPass(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	var errs []string
	checkCallCtxFields(t, builtinsCallCtxVerifyCfg(tmp, &errs))

	if len(errs) > 0 {
		t.Errorf("expected no errors on clean copy, got:\n%s", strings.Join(errs, "\n"))
	}
}

func TestVerificationCallCtxUnauthorizedAccess(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Inject callCtx.Truncate access into the cat builtin, which is not
	// permitted to use the write-capable Truncate field.
	target := findFirstFlatGoFile(t, filepath.Join(tmp, "builtins", "cat"))
	injectCallCtxFieldAccess(t, target, "Truncate")

	var errs []string
	checkCallCtxFields(t, builtinsCallCtxVerifyCfg(tmp, &errs))

	if !errContains(errs, "Truncate") || !errContains(errs, "not declared") {
		t.Errorf("expected error about unauthorized Truncate access in cat, got: %v", errs)
	}
}

func TestVerificationCallCtxUnlistedField(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Inject callCtx.SetVar access into the cat builtin. SetVar is in
	// callCtxAllFields but not in cat's per-command entry.
	target := findFirstFlatGoFile(t, filepath.Join(tmp, "builtins", "cat"))
	injectCallCtxFieldAccess(t, target, "SetVar")

	var errs []string
	checkCallCtxFields(t, builtinsCallCtxVerifyCfg(tmp, &errs))

	if !errContains(errs, "SetVar") || !errContains(errs, "not declared") {
		t.Errorf("expected error about unauthorized SetVar access in cat, got: %v", errs)
	}
}

func TestVerificationCallCtxFieldNotInAllFields(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Add a non-existent field name to echo's per-command list.
	cfg := builtinsCallCtxVerifyCfg(tmp, nil)
	cfg.PerCommandFields = copyPerCommandMap(cfg.PerCommandFields)
	cfg.PerCommandFields["echo"] = append(cfg.PerCommandFields["echo"], "NonExistentField")

	var errs []string
	cfg.Errors = &errs
	checkCallCtxFields(t, cfg)

	if !errContains(errs, "NonExistentField") || !errContains(errs, "not in callCtxAllFields") {
		t.Errorf("expected error about NonExistentField not in callCtxAllFields, got: %v", errs)
	}
}

func TestVerificationCallCtxMissingBuiltinEntry(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Remove "echo" from the per-command map.
	cfg := builtinsCallCtxVerifyCfg(tmp, nil)
	cfg.PerCommandFields = copyPerCommandMap(cfg.PerCommandFields)
	delete(cfg.PerCommandFields, "echo")

	var errs []string
	cfg.Errors = &errs
	checkCallCtxFields(t, cfg)

	if !errContains(errs, "echo") || !errContains(errs, "no entry in builtinPerCommandCallContextFields") {
		t.Errorf("expected error about missing echo entry, got: %v", errs)
	}
}

// TestVerificationCallCtxLocalAlias verifies that the checker catches
// CallContext field accesses made through a local variable alias of a known
// *CallContext holder (e.g. cc := callCtx; cc.Truncate).
func TestVerificationCallCtxLocalAlias(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Inject into cat a function that aliases the *CallContext parameter to a
	// local variable and accesses an unauthorized field through the alias.
	target := findFirstFlatGoFile(t, filepath.Join(tmp, "builtins", "cat"))
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	snippet := "\nfunc _aliasProbe(callCtxProbe *builtins.CallContext) { cc := callCtxProbe; _ = cc.Truncate }\n"
	if err := os.WriteFile(target, append(data, []byte(snippet)...), 0o644); err != nil {
		t.Fatal(err)
	}

	var errs []string
	checkCallCtxFields(t, builtinsCallCtxVerifyCfg(tmp, &errs))

	if !errContains(errs, "Truncate") || !errContains(errs, "not declared") {
		t.Errorf("expected local alias Truncate access to be detected in cat, got: %v", errs)
	}
}

// TestVerificationCallCtxDepth2 verifies that the checker catches depth-N
// CallContext field accesses (e.g. ec.callCtx.Truncate) when the intermediate
// field "callCtx" is a struct field typed *builtins.CallContext.
//
// Without bridge-field discovery, ec.callCtx.Truncate would not be detected
// because the immediate receiver is a SelectorExpr (not a bare Ident). This
// test guards against regression of that capability.
func TestVerificationCallCtxDepth2(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "builtins"), filepath.Join(tmp, "builtins"))

	// Inject a file into the cat builtin that:
	//   1. Declares a struct with a *builtins.CallContext field named "callCtx".
	//   2. Accesses the Truncate field through that bridge (cat is not allowed Truncate).
	//
	// The AST parser does not type-check, so the lack of an import for
	// "github.com/DataDog/datadog-agent/pkg/rshell/builtins" does not prevent parsing. The
	// bridge-discovery phase looks for the syntactic form *<pkg>.CallContext
	// (any package name), so this is sufficient.
	injected := filepath.Join(tmp, "builtins", "cat", "zz_depth2_probe.go")
	writeGoFile(t, injected, "cat", nil,
		`type _probeCtx struct { callCtx *builtins.CallContext }
func _depth2Probe(ec _probeCtx) { _ = ec.callCtx.Truncate }
`,
	)

	var errs []string
	checkCallCtxFields(t, builtinsCallCtxVerifyCfg(tmp, &errs))

	if !errContains(errs, "Truncate") || !errContains(errs, "not declared") {
		t.Errorf("expected depth-2 Truncate access to be detected in cat, got: %v", errs)
	}
}
