// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"path/filepath"
	"testing"

	"golang.org/x/exp/maps"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// The test data is a CUDA fatbin file compiled with the Makefile present in the same directory.
func getCudaSample(t testing.TB) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sample := filepath.Join(curDir, "testdata", "sample")
	require.FileExists(t, sample)

	return sample
}

func TestParseFatbinFromPath(t *testing.T) {
	path := getCudaSample(t)
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
	path := getCudaSample(b)
	for i := 0; i < b.N; i++ {
		_, err := ParseFatbinFromELFFilePath(path)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
