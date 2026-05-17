// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package pclntab_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/pclntab"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbolcopier"
)

func getGoToolChain(goMinorVersion int) string {
	suffix := ""
	if goMinorVersion >= 21 {
		suffix = ".0"
	}
	if goMinorVersion == 26 {
		suffix = "rc2"
	}
	return fmt.Sprintf("GOTOOLCHAIN=go1.%v%v", goMinorVersion, suffix)
}

func findSymbol(ef *pfelf.File, name string) *libpf.Symbol {
	var sym *libpf.Symbol
	_ = ef.VisitSymbols(func(s libpf.Symbol) bool {
		if string(s.Name) == name {
			sym = &s
			return false
		}
		return true
	})
	return sym
}

func getTextStart(ef *pfelf.File) uint64 {
	sym := findSymbol(ef, "runtime.text")
	if sym != nil {
		return uint64(sym.Address)
	}
	return 0
}

func getPIEStr(pie bool) string {
	if pie {
		return ".pie"
	}
	return ".pde"
}

func getStripDebugInfoStr(stripDebugInfo bool) string {
	if stripDebugInfo {
		return ".sw"
	}
	return ""
}

func getLinkexternalStr(linkexternal bool) string {
	if linkexternal {
		return ".linkexternal"
	}
	return ""
}

func checkGoPCLnTab(t *testing.T, ef *pfelf.File, goPCLnTabInfoRef *pclntab.GoPCLnTabInfo) {
	goPCLnTabInfo, err := pclntab.FindGoPCLnTabWithChecks(ef)
	require.NoError(t, err)
	assert.NotNil(t, goPCLnTabInfo)

	require.Equal(t, goPCLnTabInfoRef.GoFuncAddr, goPCLnTabInfo.GoFuncAddr)
	require.Equal(t, goPCLnTabInfoRef.Address, goPCLnTabInfo.Address)
	require.Equal(t, goPCLnTabInfoRef.TextStart.Address, goPCLnTabInfo.TextStart.Address)
	require.GreaterOrEqual(t, len(goPCLnTabInfo.Data), len(goPCLnTabInfoRef.Data))
	require.GreaterOrEqual(t, len(goPCLnTabInfo.GoFuncData), len(goPCLnTabInfoRef.GoFuncData))
}

func checkGoPCLnTabExtraction(t *testing.T, exe string, goMinorVersion int) {
	ef, err := pfelf.Open(exe)
	require.NoError(t, err)
	defer ef.Close()

	goPCLnTabInfo, err := pclntab.FindGoPCLnTabWithChecks(ef)
	require.NoError(t, err)
	assert.NotNil(t, goPCLnTabInfo)

	textStart := getTextStart(ef)

	if goMinorVersion >= 18 {
		require.NotNil(t, goPCLnTabInfo.GoFuncAddr)
		if textStart != 0 {
			require.Equal(t, textStart, goPCLnTabInfo.TextStart.Address)
		}
	}

	symbolFile := exe + ".dbg"
	err = symbolcopier.CopySymbols(t.Context(), exe, symbolFile, goPCLnTabInfo, nil, false)
	require.NoError(t, err)
	efSymbol, err := pfelf.Open(symbolFile)
	require.NoError(t, err)
	defer efSymbol.Close()

	checkGoPCLnTab(t, efSymbol, goPCLnTabInfo)

	exeStripped := exe + ".stripped"
	out, err := exec.CommandContext(t.Context(), "objcopy", "-S", exe, exeStripped).CombinedOutput() // #nosec G204
	require.NoError(t, err, "failed to rename section: %s\n%s", err, out)

	ef2, err := pfelf.Open(exeStripped)
	require.NoError(t, err)
	defer ef2.Close()

	goPCLnTabInfo2, err := pclntab.FindGoPCLnTabWithChecks(ef2)
	require.NoError(t, err)
	require.NotNil(t, goPCLnTabInfo2)

	checkGoPCLnTab(t, ef2, goPCLnTabInfo2)

	symbolFile2 := exeStripped + ".dbg"
	err = symbolcopier.CopySymbols(t.Context(), exeStripped, symbolFile2, goPCLnTabInfo2, nil, false)
	require.NoError(t, err)
	efSymbol2, err := pfelf.Open(symbolFile2)
	require.NoError(t, err)
	defer efSymbol2.Close()

	checkGoPCLnTab(t, efSymbol2, goPCLnTabInfo)
}

func TestGoPCLnTabExtraction(t *testing.T) {
	t.Parallel()
	pclntab.DisableRecoverFromPanic()
	testDataDir := "../testdata"
	srcFile := "helloworld.go"

	tmpDir := t.TempDir()
	goMinorVersions := []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26}
	for _, goMinorVersion := range goMinorVersions {
		for _, pie := range []bool{false, true} {
			for _, linkexternal := range []bool{false, true} {
				for _, stripDebugInfo := range []bool{false, true} {
					if pie && goMinorVersion <= 12 {
						continue
					}
					if runtime.GOARCH == "arm64" && goMinorVersion <= 8 {
						continue
					}
					if linkexternal && goMinorVersion <= 3 {
						continue
					}
					name := fmt.Sprintf("go1.%v%v%v%v",
						goMinorVersion, getPIEStr(pie), getStripDebugInfoStr(stripDebugInfo), getLinkexternalStr(linkexternal))

					t.Run(name, func(t *testing.T) {
						t.Parallel()
						exe := filepath.Join(tmpDir, "test."+name)
						buildArgs := []string{"build", "-o", exe}
						if pie {
							buildArgs = append(buildArgs, "-buildmode=pie")
						}
						ldflags := []string{}
						if stripDebugInfo {
							ldflags = append(ldflags, "-s", "-w")
						}
						if linkexternal {
							ldflags = append(ldflags, "-linkmode=external")
						}
						if len(ldflags) > 0 {
							buildArgs = append(buildArgs, "-ldflags="+strings.Join(ldflags, " "))
						}
						cmd := exec.CommandContext(t.Context(), "go", buildArgs...) // #nosec G204
						cmd.Args = append(cmd.Args, srcFile)
						cmd.Dir = testDataDir
						cmd.Env = append(cmd.Environ(), getGoToolChain(goMinorVersion), "GO111MODULE=off")
						out, err := cmd.CombinedOutput()
						require.NoError(t, err, "failed to build test binary with `%v`: %s\n%s", cmd.String(), err, out)

						checkGoPCLnTabExtraction(t, exe, goMinorVersion)
					})
				}
			}
		}
	}
}
