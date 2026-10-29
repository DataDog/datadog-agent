// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"strings"
	"testing"
)

// allowedpathsCheckConfig returns the allowedSymbolsConfig used to enforce
// symbol-level import restrictions on allowedpaths/. Verification tests reuse
// this function to ensure they test the exact same configuration.
func allowedpathsCheckConfig() allowedSymbolsConfig {
	return allowedSymbolsConfig{
		Symbols:   allowedpathsAllowedSymbols,
		TargetDir: "allowedpaths",
		CollectFiles: func(dir string) ([]string, error) {
			return collectFlatGoFiles(dir)
		},
		ExemptImport: func(importPath string) bool {
			return strings.HasPrefix(importPath, "github.com/DataDog/datadog-agent/pkg/rshell/")
		},
		ListName: "allowedpathsAllowedSymbols",
		MinFiles: 1,
	}
}

// TestAllowedPathsAllowedSymbols enforces symbol-level import restrictions on
// non-test Go files in allowedpaths/. Every imported symbol must be explicitly
// listed in allowedpathsAllowedSymbols. Internal module imports
// (github.com/DataDog/datadog-agent/pkg/rshell/*) are auto-allowed.
func TestAllowedPathsAllowedSymbols(t *testing.T) {
	checkAllowedSymbols(t, allowedpathsCheckConfig())
}
