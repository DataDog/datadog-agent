// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	// Info is composed of the type and binding of the symbol. Type is the lower 4 bits and binding is the upper 4 bits.
	// We are only interested in functions, which binding STB_GLOBAL (1) and type STT_FUNC (2).
	// Hence, we are interested in symbols with Info 18.
	infoFunction = byte(safeelf.STB_GLOBAL)<<4 | byte(safeelf.STT_FUNC)
)

func buildPCLNTABFixture(t *testing.T, tmpDir string) string {
	t.Helper()

	f, err := os.CreateTemp(tmpDir, "pclntab-fixture")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	exe := f.Name()
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "./testdata/pclntab_fixture.go") // #nosec G204
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build test binary with `%v`: %s\n%s", cmd.Args, err, out)
	return exe
}

func isBazelTest() bool {
	return os.Getenv("TEST_SRCDIR") != "" || os.Getenv("RUNFILES_DIR") != "" || os.Getenv("RUNFILES_MANIFEST_FILE") != ""
}

func bazelPCLNTABFixture(t *testing.T) string {
	t.Helper()

	loc := os.Getenv("BININSPECT_PCLNTAB_FIXTURE")
	require.NotEmpty(t, loc, "expected BININSPECT_PCLNTAB_FIXTURE to be set by the Bazel test rule")

	fixture, err := runfiles.Rlocation(loc)
	require.NoError(t, err)
	return fixture
}

func pclntabFixture(t *testing.T, tmpDir string) string {
	t.Helper()

	if isBazelTest() {
		return bazelPCLNTABFixture(t)
	}
	return buildPCLNTABFixture(t, tmpDir)
}

// TestGetPCLNTABSymbolParser tests the GetPCLNTABSymbolParser function with strings set symbol filter.
// We are looking to find all symbols of a Go fixture executable and check if they are found in the PCLNTAB.
func TestGetPCLNTABSymbolParser(t *testing.T) {
	exe := pclntabFixture(t, t.TempDir())
	f, err := safeelf.Open(exe)
	require.NoError(t, err)
	symbolSet := make(common.StringSet)
	staticSymbols, _ := f.Symbols()
	dynamicSymbols, _ := f.DynamicSymbols()
	for _, symbols := range [][]safeelf.Symbol{staticSymbols, dynamicSymbols} {
		for _, sym := range symbols {
			if sym.Info != infoFunction {
				continue
			}
			// Skipping types, runtime functions and ABI0 functions
			if strings.HasPrefix(sym.Name, "type:") || strings.HasPrefix(sym.Name, "runtime") || strings.HasSuffix(sym.Name, ".abi0") {
				continue
			}
			symbolSet[sym.Name] = struct{}{}
		}
	}

	got, err := GetPCLNTABSymbolParser(f, newStringSetSymbolFilter(symbolSet))
	assert.NoError(t, err)
	if err != nil {
		for sym := range symbolSet {
			if _, ok := got[sym]; !ok {
				t.Log("Missing symbol:", sym)
			}
		}
	}
}
