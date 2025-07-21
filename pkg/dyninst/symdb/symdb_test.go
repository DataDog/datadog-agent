// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package symdb_test

import (
	"flag"
	"github.com/stretchr/testify/require"
	_ "net/http/pprof"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbutil"
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
			symBuilder, err := symdb.NewSymDBBuilder(binaryPath)
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

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the snapshot files")

const snapshotDir = "testdata/snapshot"

var cases = []string{"sample"}

func TestSymDBSnapshot(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, caseName := range cases {
		t.Run(caseName, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					binaryPath := testprogs.MustGetBinary(t, caseName, cfg)
					t.Logf("exploring binary: %s", binaryPath)
					symBuilder, err := symdb.NewSymDBBuilder(binaryPath)
					require.NoError(t, err)
					symbols, err := symBuilder.ExtractSymbols()
					require.NoError(t, err, "failed to extract symbols from %s", binaryPath)
					require.NotEmpty(t, symbols.Packages)

					var sb strings.Builder
					symbols.Serialize(symdbutil.MakePanickingWriter(&sb),
						symdb.SerializationOptions{
							// Keep the size of the snapshot small by only
							// including the main module.
							OnlyMainModule: true,
							PackageSerializationOptions: symdb.PackageSerializationOptions{
								// Make the snapshot machine-independent by
								// removing local file paths (given that the
								// inspected binaries are built locally).
								StripLocalFilePrefix: true,
							},
						},
					)
					out := sb.String()

					outputFile := path.Join(snapshotDir, caseName+"."+cfg.String()+".out")
					if *rewrite {
						tmpFile, err := os.CreateTemp(snapshotDir, ".out")
						require.NoError(t, err)
						name := tmpFile.Name()
						defer func() { _ = os.Remove(name) }()
						_, err = tmpFile.WriteString(out)
						require.NoError(t, err)
						require.NoError(t, tmpFile.Close())
						require.NoError(t, os.Rename(name, outputFile))
					} else {
						expected, err := os.ReadFile(outputFile)
						require.NoError(t, err)
						require.Equal(t, string(expected), out)
					}
				})
			}
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
