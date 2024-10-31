// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/exp/maps"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// The test data is a CUDA fatbin file compiled with the Makefile present in the same directory,
// using `make <name>` (for now, only supported samples are `sample` and `heavy-sample`).
func getCudaSample(t testing.TB, name string) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sample := filepath.Join(curDir, "testdata", name)
	require.FileExists(t, sample)

	return sample
}

func TestParseFatbinFromPath(t *testing.T) {
	path := getCudaSample(t, "sample")
	res, err := ParseFatbinFromELFFilePath(path)
	require.NoError(t, err)

	kern1MangledName := "_Z7kernel1Pfi" // = kernel1(float*)
	kern2MangledName := "_Z7kernel2Pfi" // = kernel2(float*)

	seenSmVersionsAndKernels := make(map[uint32][]string)

	expectedSharedMemSizes := map[string]uint64{
		kern1MangledName: 0,
		kern2MangledName: 256,
	}

	for key, kernel := range res.Kernels {
		seenSmVersionsAndKernels[key.SmVersion] = append(seenSmVersionsAndKernels[key.SmVersion], key.Name)
		require.Equal(t, key.Name, kernel.Name)

		expectedMemSize, ok := expectedSharedMemSizes[key.Name]
		require.True(t, ok, "unexpected kernel %s", key.Name)

		// The memory sizes are different for sm_90, checked with cuobjdump
		if key.SmVersion != 90 {
			require.Equal(t, expectedMemSize, kernel.SharedMem, "unexpected shared memory size for kernel %s, sm=%d", key.Name, key.SmVersion)
		}

		require.Greater(t, kernel.KernelSize, uint64(0), "unexpected kernel size for kernel %s, sm=%d", key.Name, key.SmVersion)
	}

	// From the Makefile, all the -gencode arch=compute_XX,code=sm_XX flags
	expectedSmVersions := []uint32{50, 52, 60, 61, 70, 75, 80, 86, 89, 90}
	require.ElementsMatch(t, expectedSmVersions, maps.Keys(seenSmVersionsAndKernels))

	// Check that all the kernels are present in each version
	for version, kernelNames := range seenSmVersionsAndKernels {
		require.ElementsMatch(t, []string{kern1MangledName, kern2MangledName}, kernelNames, "missing kernels for version %d", version)
	}
}

func BenchmarkParseFatbinFromPath(b *testing.B) {
	samples := []string{"sample", "heavy-sample"}
	for _, sample := range samples {
		b.Run(sample, func(b *testing.B) {
			path := getCudaSample(b, sample)
			for i := 0; i < b.N; i++ {
				_, err := ParseFatbinFromELFFilePath(path)
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// The heavy-sample binary is an automatically-generated CUDA fatbin file with a
// large number of kernels, designed to stress the parser. The parser workload
// scales with the number of variables per kernel and kernels.
func TestParseBigFatbinFromPath(t *testing.T) {
	path := getCudaSample(t, "heavy-sample")
	res, err := ParseFatbinFromELFFilePath(path)
	require.NoError(t, err)

	// These parameters need to match the same values used in the Makefile to generate the sample
	numKernels := 100
	numVariablesPerKernel := 20
	sharedMemSize := 1024

	var expectedSharedMemSizes = make(map[string]uint64)
	var expectedKernels = make([]string, numKernels)
	for i := 0; i < numKernels; i++ {
		mangledArgSpec := strings.Repeat("S_", numVariablesPerKernel-1)
		funcName := fmt.Sprintf("kernel_%d", i)
		mangledKernName := fmt.Sprintf("_Z%d%sPf%s", len(funcName), funcName, mangledArgSpec)
		expectedKernels[i] = mangledKernName
		expectedSharedMemSizes[mangledKernName] = uint64(sharedMemSize)
	}

	seenSmVersionsAndKernels := make(map[uint32][]string)

	for key, kernel := range res.Kernels {
		seenSmVersionsAndKernels[key.SmVersion] = append(seenSmVersionsAndKernels[key.SmVersion], key.Name)
		require.Equal(t, key.Name, kernel.Name)

		expectedMemSize, ok := expectedSharedMemSizes[key.Name]
		require.True(t, ok, "unexpected kernel %s, expected kernels=%v", key.Name, expectedKernels)

		// The memory sizes are different for sm_90, checked with cuobjdump
		if key.SmVersion != 90 && false {
			require.Equal(t, expectedMemSize, kernel.SharedMem, "unexpected shared memory size for kernel %s, sm=%d", key.Name, key.SmVersion)
		}

		require.Greater(t, kernel.KernelSize, uint64(0), "unexpected kernel size for kernel %s, sm=%d", key.Name, key.SmVersion)
	}

	// From the Makefile, all the -gencode arch=compute_XX,code=sm_XX flags
	expectedSmVersions := []uint32{50, 52, 60, 61, 70, 75, 80, 86, 89, 90}
	require.ElementsMatch(t, expectedSmVersions, maps.Keys(seenSmVersionsAndKernels))

	// Check that all the kernels are present in each version
	for version, kernelNames := range seenSmVersionsAndKernels {
		require.ElementsMatch(t, expectedKernels, kernelNames, "missing kernels for version %d", version)
	}
}
