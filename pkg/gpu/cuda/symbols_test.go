// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cuda

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/cuda/testutil"
)

func TestGetSymbols(t *testing.T) {
	wantedSMVersions := map[uint32]struct{}{
		50: {},
		52: {},
		86: {},
	}

	// sanity check that the sample has the wanted SM versions
	for sm := range wantedSMVersions {
		require.Contains(t, testutil.SampleSMVersions, sm, "sample should have SM version %d", sm)
	}

	symbols, err := GetSymbols(testutil.GetCudaSample(t, "sample"), wantedSMVersions)
	require.NoError(t, err)
	require.NotNil(t, symbols)

	expectedKernels := []string{
		"_Z7kernel1Pfi",
		"_Z7kernel2Pfi",
	}

	for _, kernelName := range expectedKernels {
		require.True(t, symbols.Fatbin.HasKernelWithName(kernelName))

		found := false
		for addr, sym := range symbols.SymbolTable {
			if sym == kernelName {
				found = true
				require.NotZero(t, addr, "kernel %s should have an address", kernelName)
				break
			}
		}
		require.True(t, found)
	}
}
