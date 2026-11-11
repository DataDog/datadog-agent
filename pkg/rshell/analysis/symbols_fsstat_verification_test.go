// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"path/filepath"
	"testing"
)

func TestVerificationFSStatUnlistedSymbol(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	src := filepath.Join(root, "allowedpaths", "internal", "fsstat")
	dst := filepath.Join(tmp, "allowedpaths", "internal", "fsstat")
	copyDir(t, src, dst)

	target := filepath.Join(dst, "fsstat.go")
	injectUnlistedSymbol(t, target)

	cfg := fsstatCheckConfig()
	cfg.RepoRootOverride = tmp
	var errs []string
	cfg.Errors = &errs
	checkAllowedSymbols(t, cfg)

	if !errContains(errs, "os.Setenv") || !errContains(errs, "not in the allowlist") {
		t.Errorf("expected fsstat allowlist to reject os.Setenv, got: %v", errs)
	}
}
