// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/require"
)

func TestStrippedObjectsPreserveBTFWithoutLineInfo(t *testing.T) {
	strippedAssets := strings.Fields(os.Getenv("STRIPPED_EBPF_TEST_ASSETS"))
	lineInfoAssets := strings.Fields(os.Getenv("LINE_INFO_EBPF_TEST_ASSETS"))
	if len(strippedAssets) == 0 || len(lineInfoAssets) == 0 {
		t.Skip("STRIPPED_EBPF_TEST_ASSETS / LINE_INFO_EBPF_TEST_ASSETS not set, run bazel test //pkg/ebpf:ebpf_test")
	}

	lineInfoByKey := make(map[string]string, len(lineInfoAssets))
	for _, asset := range lineInfoAssets {
		key := strings.Replace(asset, "/with_line_info/", "/stripped/", 1)
		lineInfoByKey[key] = asset
	}
	require.Equal(t, len(lineInfoAssets), len(strippedAssets), "mismatched stripped and with_line_info asset counts")

	totalOrigLines := 0
	totalFuncInfos := 0
	totalCORERelos := 0

	for _, strippedAsset := range strippedAssets {
		lineInfoAsset, ok := lineInfoByKey[strippedAsset]
		require.True(t, ok, "missing with_line_info counterpart for %s", strippedAsset)

		strippedPath, err := runfiles.Rlocation(strippedAsset)
		require.NoError(t, err)
		lineInfoPath, err := runfiles.Rlocation(lineInfoAsset)
		require.NoError(t, err)

		t.Run(strippedAsset, func(t *testing.T) {
			strippedSpec, err := ebpf.LoadCollectionSpec(strippedPath)
			require.NoError(t, err, "failed to load stripped object %s", strippedAsset)

			origSpec, err := ebpf.LoadCollectionSpec(lineInfoPath)
			require.NoError(t, err, "failed to load with_line_info object %s", lineInfoAsset)

			require.Equal(t, len(origSpec.Programs), len(strippedSpec.Programs))
			require.Equal(t, len(origSpec.Maps), len(strippedSpec.Maps))

			objOrigLines := 0
			for name, origProg := range origSpec.Programs {
				strippedProg, exists := strippedSpec.Programs[name]
				require.True(t, exists, "missing program %s in stripped object", name)
				require.Equal(t, len(origProg.Instructions), len(strippedProg.Instructions), "instruction count mismatch for %s", name)

				for i := range origProg.Instructions {
					origInsn := origProg.Instructions[i]
					strippedInsn := strippedProg.Instructions[i]

					if _, hasLine := origInsn.Source().(*btf.Line); hasLine {
						objOrigLines++
					}
					_, strippedHasLine := strippedInsn.Source().(*btf.Line)
					require.False(t, strippedHasLine, "program %s instruction %d still has BTF line info", name, i)

					origFunc := btf.FuncMetadata(&origInsn)
					strippedFunc := btf.FuncMetadata(&strippedInsn)
					if origFunc == nil {
						require.Nil(t, strippedFunc, "unexpected func_info at %s insn %d", name, i)
					} else {
						require.NotNil(t, strippedFunc, "missing func_info at %s insn %d", name, i)
						require.Equal(t, origFunc.Name, strippedFunc.Name)
						totalFuncInfos++
					}

					origRelo := btf.CORERelocationMetadata(&origInsn)
					strippedRelo := btf.CORERelocationMetadata(&strippedInsn)
					if origRelo == nil {
						require.Nil(t, strippedRelo, "unexpected core_relo at %s insn %d", name, i)
					} else {
						require.NotNil(t, strippedRelo, "missing core_relo at %s insn %d", name, i)
						require.Equal(t, fmt.Sprint(origRelo), fmt.Sprint(strippedRelo))
						totalCORERelos++
					}
				}
			}

			totalOrigLines += objOrigLines
			if objOrigLines > 0 {
				strippedInfo, err := os.Stat(strippedPath)
				require.NoError(t, err)
				origInfo, err := os.Stat(lineInfoPath)
				require.NoError(t, err)
				require.Less(t, strippedInfo.Size(), origInfo.Size(), "stripped object %s should be smaller than with_line_info", strippedAsset)
			}
		})
	}

	require.NotZero(t, totalOrigLines, "expected at least one original BTF line_info record across test assets")
	require.NotZero(t, totalFuncInfos, "expected BTF func_info records to be preserved")
	require.NotZero(t, totalCORERelos, "expected CO-RE relocation records to be preserved")
}
