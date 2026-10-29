// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"strings"
	"testing"
)

// builtinsCheckConfig returns the allowedSymbolsConfig used to enforce
// symbol-level import restrictions on builtins/. Verification tests reuse
// this function to ensure they test the exact same configuration.
func builtinsCheckConfig() allowedSymbolsConfig {
	return allowedSymbolsConfig{
		Symbols:   builtinAllowedSymbols,
		TargetDir: "builtins",
		CollectFiles: func(dir string) ([]string, error) {
			// "internal" has its own dedicated check (TestInternalAllowedSymbols).
			return collectGoFilesRecursive(dir, map[string]bool{"testutil": true, "internal": true}, func(rel string) bool {
				// builtins.go is the package framework and is exempt. Every
				// other top-level file (e.g. proc_provider.go, features.go) is
				// audited against builtinAllowedSymbols.
				return rel == "builtins.go"
			})
		},
		ExemptImport: func(importPath string) bool {
			return importPath == "github.com/DataDog/datadog-agent/pkg/rshell/builtins" ||
				strings.HasPrefix(importPath, "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/")
		},
		ListName: "builtinAllowedSymbols",
		MinFiles: 1,
	}
}

// TestBuiltinAllowedSymbols enforces symbol-level import restrictions on
// command implementation files in builtins/. builtins.go is exempt as
// the package framework. Every other file's imports and pkg.Symbol references
// must be explicitly listed in builtinAllowedSymbols.
func TestBuiltinAllowedSymbols(t *testing.T) {
	checkAllowedSymbols(t, builtinsCheckConfig())
}

// builtinsPerCommandCheckConfig returns the perBuiltinConfig for testing
// per-command symbol restrictions. Verification tests reuse this function.
func builtinsPerCommandCheckConfig() perBuiltinConfig {
	return perBuiltinConfig{
		CommonSymbols:     builtinAllowedSymbols,
		PerCommandSymbols: builtinPerCommandSymbols,
		TargetDir:         "builtins",
		ExemptImport: func(importPath string) bool {
			return importPath == "github.com/DataDog/datadog-agent/pkg/rshell/builtins" ||
				strings.HasPrefix(importPath, "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/")
		},
		SkipDirs: map[string]bool{"testutil": true, "tests": true, "internal": true},
	}
}

// TestBuiltinPerCommandSymbols enforces per-builtin symbol restrictions.
// Each builtin subdirectory may only use the symbols declared in its
// builtinPerCommandSymbols entry, which must be a subset of
// builtinAllowedSymbols.
func TestBuiltinPerCommandSymbols(t *testing.T) {
	checkPerBuiltinAllowedSymbols(t, builtinsPerCommandCheckConfig())
}

// builtinsCallCtxCheckConfig returns the callCtxFieldConfig for enforcing
// per-builtin CallContext field access restrictions. Verification tests reuse
// this function to ensure they test the exact same configuration.
func builtinsCallCtxCheckConfig() callCtxFieldConfig {
	return callCtxFieldConfig{
		AllFields:        callCtxAllFields,
		PerCommandFields: builtinPerCommandCallContextFields,
		TargetDir:        "builtins",
		SkipDirs:         map[string]bool{"testutil": true, "tests": true, "internal": true},
	}
}

// TestBuiltinCallContextFields enforces per-builtin CallContext field access
// restrictions. Each builtin may only make direct selector-expression accesses
// to the CallContext fields declared in builtinPerCommandCallContextFields.
//
// The primary security invariant: only the "truncate" builtin may access
// callCtx.Truncate. No other builtin may access a write-capable field it has
// not explicitly declared.
func TestBuiltinCallContextFields(t *testing.T) {
	checkCallCtxFields(t, builtinsCallCtxCheckConfig())
}

// internalCheckConfig returns the allowedSymbolsConfig used to enforce
// symbol-level import restrictions on builtins/internal/ helper packages.
func internalCheckConfig() allowedSymbolsConfig {
	return allowedSymbolsConfig{
		Symbols:   internalAllowedSymbols,
		TargetDir: "builtins/internal",
		CollectFiles: func(dir string) ([]string, error) {
			return collectGoFilesRecursive(dir, nil, nil)
		},
		ExemptImport: func(importPath string) bool {
			return importPath == "github.com/DataDog/datadog-agent/pkg/rshell/builtins"
		},
		ListName: "internalAllowedSymbols",
		MinFiles: 1,
	}
}

// TestInternalAllowedSymbols enforces symbol-level import restrictions on
// builtins/internal/ helper packages. unsafe.Pointer is explicitly permitted
// here for the narrow iphlpapi.dll DLL call in winnet/winnet_windows.go.
func TestInternalAllowedSymbols(t *testing.T) {
	checkAllowedSymbols(t, internalCheckConfig())
}
