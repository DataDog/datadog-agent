// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package symdb_test

import (
	"github.com/stretchr/testify/require"
	_ "net/http/pprof"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestSymDB(t *testing.T) {
	cfgs, err := testprogs.GetCommonConfigs()
	require.NoError(t, err)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			binaryPath, err := testprogs.GetBinary("simple", cfg)
			require.NoError(t, err)
			t.Logf("exploring binary: %s", binaryPath)
			file, err := object.OpenElfFile(binaryPath)
			require.NoError(t, err)
			symBuilder, err := symdb.NewSymDBBuilder(file)
			require.NoError(t, err)
			symbols, err := symBuilder.ExtractSymbols()
			require.NoError(t, err, "failed to extract symbols from %s", binaryPath)
			require.NotEmpty(t, symbols.Packages)

			// Look at a couple of symbols as a smoke test.
			pkg, ok := findPackage(symbols, "main")
			require.Truef(t, ok, "package 'main' not found in %s", binaryPath)
			fn, ok := findFunction(pkg, "stringArg")
			require.Truef(t, ok, "function 'stringArg' not found in package 'main' in %s", binaryPath)
			require.NotZero(t, fn.StartLine)
			require.NotZero(t, fn.EndLine)
			require.Less(t, fn.StartLine, fn.EndLine)
			v, ok := findVariable(fn.Scope, "s")
			require.Truef(t, ok, "variable 's' not found in function 'stringArg' in package 'main' in %s", binaryPath)
			require.True(t, v.FunctionArgument)
			require.NotZero(t, v.DeclLine)
			require.NotEmpty(t, v.AvailableLineRanges)
		})
	}
}

func findPackage(s symdb.Symbols, pkgName string) (symdb.Package, bool) {
	for _, pkg := range s.Packages {
		if pkg.Name == pkgName {
			return pkg, true
		}
	}
	return symdb.Package{}, false
}

func findFunction(pkg symdb.Package, fnName string) (symdb.Function, bool) {
	for _, fn := range pkg.Functions {
		if fn.Name == fnName {
			return fn, true
		}
	}
	return symdb.Function{}, false
}

func findVariable(scope symdb.Scope, varName string) (symdb.Variable, bool) {
	for _, variable := range scope.Variables {
		if variable.Name == varName {
			return variable, true
		}
	}
	return symdb.Variable{}, false
}
