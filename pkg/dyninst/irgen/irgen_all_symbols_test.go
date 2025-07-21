// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestIRGenAllProbes(t *testing.T) {
	programs := testprogs.MustGetPrograms(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	var objcopy string
	{
		if objcopyPath, err := exec.LookPath("objcopy"); err == nil {
			objcopy = objcopyPath
		}
	}

	for _, pkg := range programs {
		switch pkg {
		case "simple", "sample":
		default:
			t.Logf("skipping %s", pkg)
			continue
		}
		t.Run(pkg, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					bin := testprogs.MustGetBinary(t, pkg, cfg)
					testAllProbes(t, bin)
					version, ok := object.ParseGoVersion(cfg.GOTOOLCHAIN)
					require.True(t, ok)
					if version.Minor >= 25 {
						return // already uses loclists
					}
					t.Run("bogus loclist", func(t *testing.T) {
						tempDir, cleanup := dyninsttest.PrepTmpDir(t, "irgen_all_symbols_test")
						defer cleanup()
						modified, err := addLoclistSection(bin, objcopy, tempDir)
						if err != nil {
							t.Skipf("failed to objcopy a loclist section for %s: %v", cfg.String(), err)
						}
						testAllProbes(t, modified)
					})
				})
			}
		})
	}
}

func addLoclistSection(binPath, objcopy, tmpDir string) (modifiedBinPath string, err error) {
	junkDataFile := path.Join(tmpDir, "junk.data")
	junkData := make([]byte, 1024)
	if _, err := io.ReadFull(rand.Reader, junkData); err != nil {
		return "", fmt.Errorf("failed to generate junk data: %w", err)
	}
	if err := os.WriteFile(junkDataFile, junkData, 0644); err != nil {
		return "", fmt.Errorf("failed to write junk data: %w", err)
	}
	modifiedBinPath = filepath.Join(tmpDir, "modified.bin")
	if output, err := exec.Command(objcopy,
		"--add-section", ".debug_loclists="+junkDataFile,
		"--set-section-flags", ".debug_loclists=alloc,readonly,debug",
		binPath,
		modifiedBinPath,
	).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to objcopy: %w\n%s", err, string(output))
	}
	return modifiedBinPath, nil
}

func testAllProbes(t *testing.T, binPath string) {
	binary, err := os.Open(binPath)
	require.NoError(t, err)
	elf, err := safeelf.NewFile(binary)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()
	var probes []ir.ProbeDefinition
	symbols, err := elf.Symbols()
	require.NoError(t, err)
	for i, s := range symbols {
		// These automatically generated symbols cause problems.
		if strings.HasPrefix(s.Name, "type:.") {
			continue
		}
		if strings.HasPrefix(s.Name, "runtime.vdso") {
			continue
		}

		// Speed things up by skipping some symbols.
		probes = append(probes, &rcjson.SnapshotProbe{
			LogProbeCommon: rcjson.LogProbeCommon{
				ProbeCommon: rcjson.ProbeCommon{
					ID:    fmt.Sprintf("probe_%d", i),
					Where: &rcjson.Where{MethodName: s.Name},
				},
			},
		})
	}

	obj, err := object.OpenElfFile(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	v, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, v)
	// TODO: Validate more properties of the IR.
}
