// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"strings"
	"testing"
)

// interpCheckConfig returns the allowedSymbolsConfig used to enforce
// symbol-level import restrictions on interp/. Verification tests reuse
// this function to ensure they test the exact same configuration.
func interpCheckConfig() allowedSymbolsConfig {
	return allowedSymbolsConfig{
		Symbols:   interpAllowedSymbols,
		TargetDir: "interp",
		CollectFiles: func(dir string) ([]string, error) {
			return collectFlatGoFiles(dir)
		},
		ExemptImport: func(importPath string) bool {
			return strings.HasPrefix(importPath, "github.com/DataDog/datadog-agent/pkg/rshell/")
		},
		ListName: "interpAllowedSymbols",
		MinFiles: 1,
	}
}

// TestInterpAllowedSymbols enforces symbol-level import restrictions on
// non-test Go files in interp/. Every imported symbol must be explicitly
// listed in interpAllowedSymbols. Internal module imports
// (github.com/DataDog/datadog-agent/pkg/rshell/*) are auto-allowed.
func TestInterpAllowedSymbols(t *testing.T) {
	checkAllowedSymbols(t, interpCheckConfig())
}

// interpPerModeCheckConfig returns the perInterpModeConfig used to enforce
// per-mode symbol restrictions on interp/. Verification tests reuse this
// function to ensure they test the exact same configuration.
func interpPerModeCheckConfig() perInterpModeConfig {
	return perInterpModeConfig{
		GlobalSymbols:  interpAllowedSymbols,
		PerModeSymbols: interpPerModeSymbols,
		TargetDir:      "interp",
		ExemptImport: func(importPath string) bool {
			return strings.HasPrefix(importPath, "github.com/DataDog/datadog-agent/pkg/rshell/")
		},
	}
}

// TestInterpPerModeSymbols enforces per-mode symbol restrictions on interp/.
// Read-only files may only use interpPerModeSymbols["read-only"]; remediation
// files may additionally use interpPerModeSymbols["remediation"].
func TestInterpPerModeSymbols(t *testing.T) {
	checkInterpPerModeSymbols(t, interpPerModeCheckConfig())
}

// internalPerPackageCheckConfig returns the perBuiltinConfig for testing
// per-package symbol restrictions on builtins/internal/ packages.
func internalPerPackageCheckConfig() perBuiltinConfig {
	return perBuiltinConfig{
		CommonSymbols:     internalAllowedSymbols,
		PerCommandSymbols: internalPerPackageSymbols,
		TargetDir:         "builtins/internal",
		ExemptImport: func(importPath string) bool {
			return importPath == "github.com/DataDog/datadog-agent/pkg/rshell/builtins"
		},
		SkipDirs: map[string]bool{},
	}
}

// TestInternalPerPackageSymbols enforces per-package symbol restrictions on
// builtins/internal/ packages. Each package subdirectory may only use the
// symbols declared in its internalPerPackageSymbols entry, which must be a
// subset of internalAllowedSymbols.
func TestInternalPerPackageSymbols(t *testing.T) {
	checkPerBuiltinAllowedSymbols(t, internalPerPackageCheckConfig())
}
