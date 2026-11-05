// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"path/filepath"
	"strings"
	"testing"
)

// allowedpathsVerifyCfg returns an allowedpathsCheckConfig with
// RepoRootOverride and Errors set for verification testing.
func allowedpathsVerifyCfg(tempRoot string, errs *[]string) allowedSymbolsConfig {
	cfg := allowedpathsCheckConfig()
	cfg.RepoRootOverride = tempRoot
	cfg.Errors = errs
	return cfg
}

func TestVerificationAllowedpathsCleanPass(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "allowedpaths"), filepath.Join(tmp, "allowedpaths"))

	var errs []string
	checkAllowedSymbols(t, allowedpathsVerifyCfg(tmp, &errs))

	if len(errs) > 0 {
		t.Errorf("expected no errors on clean copy, got:\n%s", strings.Join(errs, "\n"))
	}
}

func TestVerificationAllowedpathsUnlistedSymbol(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "allowedpaths"), filepath.Join(tmp, "allowedpaths"))

	target := findFirstFlatGoFile(t, filepath.Join(tmp, "allowedpaths"))
	injectUnlistedSymbol(t, target)

	var errs []string
	checkAllowedSymbols(t, allowedpathsVerifyCfg(tmp, &errs))

	if !errContains(errs, "os.Setenv") || !errContains(errs, "not in the allowlist") {
		t.Errorf("expected 'not in the allowlist' error for os.Setenv, got: %v", errs)
	}
}

func TestVerificationAllowedpathsExemptImport(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, "allowedpaths"), filepath.Join(tmp, "allowedpaths"))

	target := findFirstFlatGoFile(t, filepath.Join(tmp, "allowedpaths"))
	// Internal module imports (github.com/DataDog/datadog-agent/pkg/rshell/*) are exempt.
	injectImport(t, target, `fakepkg "github.com/DataDog/datadog-agent/pkg/rshell/fakepkg"`, "var _ = fakepkg.Foo")

	var errs []string
	checkAllowedSymbols(t, allowedpathsVerifyCfg(tmp, &errs))

	if errContains(errs, "github.com/DataDog/datadog-agent/pkg/rshell/fakepkg") {
		t.Errorf("exempt import should not be flagged, got: %v", errs)
	}
}
