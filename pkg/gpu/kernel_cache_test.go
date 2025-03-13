// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"os"
	"testing"
	"time"

	"github.com/prometheus/procfs"

	"github.com/stretchr/testify/require"

	cudatestutil "github.com/DataDog/datadog-agent/pkg/gpu/cuda/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestBuildSymbolFileIdentifier(t *testing.T) {
	// Create a file, then a symlink to it
	// and check that the identifier is the same
	// for both files.
	dir := t.TempDir()
	filePath := dir + "/file"
	copyPath := dir + "/copy"
	differentPath := dir + "/different"
	symlinkPath := dir + "/symlink"

	data := []byte("hello")
	// create the original file
	err := os.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	// create a symlink to the original file, which should have the same identifier
	err = os.Symlink(filePath, symlinkPath)
	require.NoError(t, err)

	// a copy is a different inode, so it should have a different identifier
	// even with the same size
	err = os.WriteFile(copyPath, data, 0644)
	require.NoError(t, err)

	// a different file with different content should have a different identifier
	// as it's different content and different inode
	err = os.WriteFile(differentPath, []byte("different"), 0644)
	require.NoError(t, err)

	origIdentifier, err := buildSymbolFileIdentifier(filePath)
	require.NoError(t, err)

	symlinkIdentifier, err := buildSymbolFileIdentifier(symlinkPath)
	require.NoError(t, err)

	copyIdentifier, err := buildSymbolFileIdentifier(copyPath)
	require.NoError(t, err)

	differentIdentifier, err := buildSymbolFileIdentifier(differentPath)
	require.NoError(t, err)

	require.Equal(t, origIdentifier, symlinkIdentifier)
	require.NotEqual(t, origIdentifier, copyIdentifier)
	require.NotEqual(t, origIdentifier, differentIdentifier)
	require.NotEqual(t, copyIdentifier, differentIdentifier)
}

func TestKernelCache_LoadKernelData(t *testing.T) {
	// Get sample CUDA binary path
	samplePath := cudatestutil.GetCudaSample(t, "sample")

	// sanity check that the sample has a kernel for the SM we are going to parse
	require.Contains(t, cudatestutil.SampleSMVersions, testutil.DefaultSMVersion, "default SM version should be in sample file")

	// Create system context with telemetry
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)

	// Create kernel cache
	kc := newKernelCache(sysCtx)
	kc.Start()
	defer kc.Stop()

	// Load kernel data
	key := kernelKey{
		pid:       1234,
		address:   0x1000,
		smVersion: testutil.DefaultSMVersion,
	}

	// Add memory map entry for the test
	kc.pidMaps[key.pid] = []*procfs.ProcMap{
		{
			StartAddr: 0x1000,
			EndAddr:   0x2000,
			Offset:    0,
			Pathname:  samplePath,
		},
	}

	// First call should return not processed yet
	kernel, err := kc.GetKernelData(key.pid, key.address, key.smVersion)
	require.ErrorIs(t, err, errKernelNotProcessedYet)
	require.Nil(t, kernel)

	// Wait for background processing
	time.Sleep(100 * time.Millisecond)

	// Second call should return the kernel
	kernel, err = kc.GetKernelData(key.pid, key.address, key.smVersion)
	require.NoError(t, err)
	require.NotNil(t, kernel)
	require.Equal(t, "_Z7kernel1Pfi", kernel.Name) // kernel1(float*, int)
	require.Equal(t, uint64(0), kernel.SharedMem)

	// Verify cache entry exists
	kc.cacheMutex.RLock()
	cachedData, exists := kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.True(t, exists)
	require.Equal(t, kernel, cachedData.kernel)
	require.NoError(t, cachedData.err)

	// Cleanup should remove the entry
	kc.cleanDataForPid(key.pid)
	kc.cacheMutex.RLock()
	_, exists = kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.False(t, exists)
}

func TestKernelCache_LoadKernelDataError(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)

	// Create kernel cache
	kc := newKernelCache(sysCtx)
	kc.Start()
	defer kc.Stop()

	// Load kernel data with invalid path
	key := kernelKey{
		pid:       1234,
		address:   0x1000,
		smVersion: testutil.DefaultSMVersion,
	}

	// Add memory map entry with invalid path
	kc.pidMaps[key.pid] = []*procfs.ProcMap{
		{
			StartAddr: 0x1000,
			EndAddr:   0x2000,
			Offset:    0,
			Pathname:  "/nonexistent/path",
		},
	}

	// First call should return not processed yet
	kernel, err := kc.GetKernelData(key.pid, key.address, key.smVersion)
	require.ErrorIs(t, err, errKernelNotProcessedYet)
	require.Nil(t, kernel)

	// Wait for background processing
	time.Sleep(100 * time.Millisecond)

	// Second call should return the error
	kernel, err = kc.GetKernelData(key.pid, key.address, key.smVersion)
	require.Error(t, err)
	require.Nil(t, kernel)

	// Verify cache entry exists with error
	kc.cacheMutex.RLock()
	cachedData, exists := kc.cache[key]
	kc.cacheMutex.RUnlock()
	require.True(t, exists)
	require.Nil(t, cachedData.kernel)
	require.Error(t, cachedData.err)
}
